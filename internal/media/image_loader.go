package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	db "github.com/Alexander-D-Karpov/amp/internal/storage"
)

type ImageLoader struct {
	storage      *db.Database
	httpClient   *http.Client
	memCache     sync.Map
	downloadMu   sync.Map
	mediaBase    string
	debug        bool
	cacheDir     string
	maxCacheSize int64
}

type CachedResource struct {
	resource   fyne.Resource
	lastAccess time.Time
	size       int64
}

func NewImageLoader(cfg *config.Config, db *db.Database) (*ImageLoader, error) {
	parsedURL, err := url.Parse(cfg.API.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL for image loader: %w", err)
	}
	mediaBase := parsedURL.Scheme + "://" + parsedURL.Host

	cacheDir := filepath.Join(cfg.Storage.CacheDir, "images")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &ImageLoader{
		storage:      db,
		httpClient:   &http.Client{Timeout: time.Duration(cfg.API.Timeout) * time.Second},
		mediaBase:    mediaBase,
		debug:        cfg.Debug,
		cacheDir:     cacheDir,
		maxCacheSize: cfg.Storage.MaxCacheSize / 4,
	}, nil
}

func (l *ImageLoader) GetResource(imageURL string) (fyne.Resource, error) {
	if imageURL == "" {
		return theme.MediaMusicIcon(), nil
	}

	fullURL := l.buildFullURL(imageURL)
	cacheKey := l.generateCacheKey(fullURL)

	if cached, ok := l.memCache.Load(cacheKey); ok {
		if cachedRes, ok := cached.(*CachedResource); ok {
			cachedRes.lastAccess = time.Now()
			l.memCache.Store(cacheKey, cachedRes)
			return cachedRes.resource, nil
		}
	}

	if _, loaded := l.downloadMu.LoadOrStore(fullURL, struct{}{}); loaded {
		for i := 0; i < 50; i++ {
			time.Sleep(20 * time.Millisecond)
			if cached, ok := l.memCache.Load(cacheKey); ok {
				if cachedRes, ok := cached.(*CachedResource); ok {
					return cachedRes.resource, nil
				}
			}
		}
		return theme.MediaMusicIcon(), fmt.Errorf("timeout waiting for download")
	}
	defer l.downloadMu.Delete(fullURL)

	localPath := filepath.Join(l.cacheDir, cacheKey)
	if data, err := l.loadFromDisk(localPath); err == nil && len(data) > 0 {
		res := fyne.NewStaticResource(generateResourceName(fullURL), data)
		l.storeInMemCache(cacheKey, res, int64(len(data)))
		return res, nil
	}

	ctx := context.Background()
	path, err := l.storage.GetCachedFile(ctx, fullURL)
	if err == nil && path != "" {
		data, err := l.loadFromDisk(path)
		if err == nil && len(data) > 0 {
			res := fyne.NewStaticResource(generateResourceName(fullURL), data)
			l.storeInMemCache(cacheKey, res, int64(len(data)))
			return res, nil
		}
	}

	downloadCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	data, err := l.downloadImage(downloadCtx, fullURL)
	if err != nil {
		return theme.MediaMusicIcon(), err
	}

	if len(data) == 0 {
		return theme.MediaMusicIcon(), fmt.Errorf("empty image data")
	}

	l.saveToDisk(localPath, data)

	go func() {
		_, saveErr := l.storage.SaveCachedFile(ctx, fullURL, bytes.NewReader(data))
		if saveErr != nil && l.debug {
		}
	}()

	res := fyne.NewStaticResource(generateResourceName(fullURL), data)
	l.storeInMemCache(cacheKey, res, int64(len(data)))

	return res, nil
}

func (l *ImageLoader) GetResourceAsync(imageURL string, callback func(fyne.Resource, error)) {
	go func() {
		resource, err := l.GetResource(imageURL)
		fyne.Do(func() {
			callback(resource, err)
		})
	}()
}

func (l *ImageLoader) downloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return data, nil
}

func (l *ImageLoader) loadFromDisk(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (l *ImageLoader) saveToDisk(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (l *ImageLoader) storeInMemCache(key string, resource fyne.Resource, size int64) {
	l.evictIfNecessary(size)

	cached := &CachedResource{
		resource:   resource,
		lastAccess: time.Now(),
		size:       size,
	}
	l.memCache.Store(key, cached)
}

func (l *ImageLoader) evictIfNecessary(newSize int64) {
	var totalSize int64
	var items []string

	l.memCache.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if cached, ok := value.(*CachedResource); ok {
				totalSize += cached.size
				items = append(items, keyStr)
			}
		}
		return true
	})

	if totalSize+newSize <= l.maxCacheSize {
		return
	}

	type cacheItem struct {
		key        string
		lastAccess time.Time
		size       int64
	}

	var sortedItems []cacheItem
	for _, key := range items {
		if cached, ok := l.memCache.Load(key); ok {
			if cachedRes, ok := cached.(*CachedResource); ok {
				sortedItems = append(sortedItems, cacheItem{
					key:        key,
					lastAccess: cachedRes.lastAccess,
					size:       cachedRes.size,
				})
			}
		}
	}

	for i := 0; i < len(sortedItems)-1; i++ {
		for j := 0; j < len(sortedItems)-i-1; j++ {
			if sortedItems[j].lastAccess.After(sortedItems[j+1].lastAccess) {
				sortedItems[j], sortedItems[j+1] = sortedItems[j+1], sortedItems[j]
			}
		}
	}

	for _, item := range sortedItems {
		l.memCache.Delete(item.key)
		totalSize -= item.size
		if totalSize+newSize <= l.maxCacheSize {
			break
		}
	}
}

func (l *ImageLoader) buildFullURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return l.mediaBase + path
	}
	return l.mediaBase + "/" + path
}

func (l *ImageLoader) generateCacheKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", hash)
}

func generateResourceName(url string) string {
	if len(url) > 50 {
		return url[len(url)-50:]
	}
	return url
}

func (l *ImageLoader) ClearMemoryCache() {
	l.memCache.Range(func(key, value interface{}) bool {
		l.memCache.Delete(key)
		return true
	})
}

func (l *ImageLoader) GetCacheStats() (itemCount int, totalSize int64) {
	l.memCache.Range(func(key, value interface{}) bool {
		itemCount++
		if cached, ok := value.(*CachedResource); ok {
			totalSize += cached.size
		}
		return true
	})
	return
}
