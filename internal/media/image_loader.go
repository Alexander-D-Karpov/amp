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
	maxMemItems  int
	loadQueue    chan *loadRequest
	workers      int
}

type CachedResource struct {
	resource   fyne.Resource
	lastAccess time.Time
	size       int64
	url        string
}

type loadRequest struct {
	url      string
	callback func(fyne.Resource, error)
	priority int
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

	loader := &ImageLoader{
		storage:      db,
		httpClient:   &http.Client{Timeout: time.Duration(cfg.API.Timeout) * time.Second},
		mediaBase:    mediaBase,
		debug:        cfg.Debug,
		cacheDir:     cacheDir,
		maxCacheSize: cfg.Storage.MaxCacheSize / 4,
		maxMemItems:  500,
		loadQueue:    make(chan *loadRequest, 1000),
		workers:      4,
	}

	for i := 0; i < loader.workers; i++ {
		go loader.worker()
	}

	go loader.cleanupWorker()

	return loader, nil
}

func (l *ImageLoader) worker() {
	for req := range l.loadQueue {
		resource, err := l.loadResourceSync(req.url)
		if req.callback != nil {
			req.callback(resource, err)
		}
	}
}

func (l *ImageLoader) cleanupWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.cleanupMemoryCache()
	}
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
		for i := 0; i < 100; i++ {
			time.Sleep(10 * time.Millisecond)
			if cached, ok := l.memCache.Load(cacheKey); ok {
				if cachedRes, ok := cached.(*CachedResource); ok {
					return cachedRes.resource, nil
				}
			}
		}
		return theme.MediaMusicIcon(), fmt.Errorf("timeout waiting for download")
	}
	defer l.downloadMu.Delete(fullURL)

	return l.loadResourceSync(fullURL)
}

func (l *ImageLoader) loadResourceSync(fullURL string) (fyne.Resource, error) {
	cacheKey := l.generateCacheKey(fullURL)

	localPath := filepath.Join(l.cacheDir, cacheKey)
	if data, err := l.loadFromDisk(localPath); err == nil && len(data) > 0 {
		if l.isValidImageData(data) {
			res := fyne.NewStaticResource(l.generateResourceName(fullURL), data)
			l.storeInMemCache(cacheKey, res, int64(len(data)), fullURL)
			return res, nil
		}
	}

	ctx := context.Background()
	path, err := l.storage.GetCachedFile(ctx, fullURL)
	if err == nil && path != "" {
		data, err := l.loadFromDisk(path)
		if err == nil && len(data) > 0 && l.isValidImageData(data) {
			res := fyne.NewStaticResource(l.generateResourceName(fullURL), data)
			l.storeInMemCache(cacheKey, res, int64(len(data)), fullURL)
			return res, nil
		}
	}

	downloadCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	data, err := l.downloadImage(downloadCtx, fullURL)
	if err != nil {
		return theme.MediaMusicIcon(), err
	}

	if len(data) == 0 || !l.isValidImageData(data) {
		return theme.MediaMusicIcon(), fmt.Errorf("invalid image data")
	}

	l.saveToDisk(localPath, data)

	go func() {
		_, saveErr := l.storage.SaveCachedFile(ctx, fullURL, bytes.NewReader(data))
		if saveErr != nil && l.debug {
		}
	}()

	res := fyne.NewStaticResource(l.generateResourceName(fullURL), data)
	l.storeInMemCache(cacheKey, res, int64(len(data)), fullURL)

	return res, nil
}

func (l *ImageLoader) GetResourceAsync(imageURL string, callback func(fyne.Resource, error)) {
	if imageURL == "" {
		fyne.Do(func() {
			callback(theme.MediaMusicIcon(), nil)
		})
		return
	}

	fullURL := l.buildFullURL(imageURL)
	cacheKey := l.generateCacheKey(fullURL)

	if cached, ok := l.memCache.Load(cacheKey); ok {
		if cachedRes, ok := cached.(*CachedResource); ok {
			cachedRes.lastAccess = time.Now()
			l.memCache.Store(cacheKey, cachedRes)
			fyne.Do(func() {
				callback(cachedRes.resource, nil)
			})
			return
		}
	}

	req := &loadRequest{
		url: fullURL,
		callback: func(resource fyne.Resource, err error) {
			fyne.Do(func() {
				callback(resource, err)
			})
		},
		priority: 1,
	}

	select {
	case l.loadQueue <- req:
	default:
		go func() {
			resource, err := l.loadResourceSync(fullURL)
			fyne.Do(func() {
				callback(resource, err)
			})
		}()
	}
}

func (l *ImageLoader) isValidImageData(data []byte) bool {
	if len(data) < 10 {
		return false
	}

	jpegHeader := []byte{0xFF, 0xD8, 0xFF}
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47}
	gifHeader := []byte{0x47, 0x49, 0x46}
	webpHeader := []byte{0x52, 0x49, 0x46, 0x46}

	if bytes.HasPrefix(data, jpegHeader) ||
		bytes.HasPrefix(data, pngHeader) ||
		bytes.HasPrefix(data, gifHeader) ||
		bytes.HasPrefix(data, webpHeader) {
		return true
	}

	return false
}

func (l *ImageLoader) downloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "AMP/1.0.0")
	req.Header.Set("Accept", "image/*")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") && contentType != "" {
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
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

func (l *ImageLoader) storeInMemCache(key string, resource fyne.Resource, size int64, url string) {
	l.evictIfNecessary(size)

	cached := &CachedResource{
		resource:   resource,
		lastAccess: time.Now(),
		size:       size,
		url:        url,
	}
	l.memCache.Store(key, cached)
}

func (l *ImageLoader) evictIfNecessary(newSize int64) {
	var totalSize int64
	var itemCount int
	var items []string

	l.memCache.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if cached, ok := value.(*CachedResource); ok {
				totalSize += cached.size
				itemCount++
				items = append(items, keyStr)
			}
		}
		return true
	})

	if totalSize+newSize <= l.maxCacheSize && itemCount < l.maxMemItems {
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

	evictCount := len(sortedItems) / 4
	if evictCount < 10 {
		evictCount = 10
	}

	for i := 0; i < evictCount && i < len(sortedItems); i++ {
		item := sortedItems[i]
		l.memCache.Delete(item.key)
		totalSize -= item.size
		if totalSize+newSize <= l.maxCacheSize {
			break
		}
	}
}

func (l *ImageLoader) cleanupMemoryCache() {
	cutoff := time.Now().Add(-30 * time.Minute)
	var toDelete []string

	l.memCache.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if cached, ok := value.(*CachedResource); ok {
				if cached.lastAccess.Before(cutoff) {
					toDelete = append(toDelete, keyStr)
				}
			}
		}
		return true
	})

	for _, key := range toDelete {
		l.memCache.Delete(key)
	}

	if l.debug && len(toDelete) > 0 {
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

func (l *ImageLoader) generateResourceName(url string) string {
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

func (l *ImageLoader) PreloadImages(urls []string) {
	for _, url := range urls {
		if url != "" {
			fullURL := l.buildFullURL(url)
			cacheKey := l.generateCacheKey(fullURL)

			if _, ok := l.memCache.Load(cacheKey); !ok {
				req := &loadRequest{
					url:      fullURL,
					callback: nil,
					priority: 0,
				}

				select {
				case l.loadQueue <- req:
				default:
				}
			}
		}
	}
}
