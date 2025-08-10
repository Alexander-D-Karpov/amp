package services

import (
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/Alexander-D-Karpov/amp/internal/media"
)

type ImageService struct {
	loader     *media.ImageLoader
	cache      sync.Map
	loading    sync.Map
	callbacks  sync.Map
	fallback   fyne.Resource
	debug      bool
	maxRetries int
}

type CacheEntry struct {
	resource  fyne.Resource
	timestamp time.Time
	url       string
	size      int64
}

type CallbackList struct {
	callbacks []func(fyne.Resource, error)
	mutex     sync.Mutex
}

func NewImageService(loader *media.ImageLoader) *ImageService {
	return &ImageService{
		loader:     loader,
		fallback:   theme.MediaMusicIcon(),
		debug:      false, // Reduced debug logging
		maxRetries: 3,
	}
}

func (s *ImageService) GetImage(url string) fyne.Resource {
	if url == "" {
		return s.fallback
	}

	if cached, ok := s.cache.Load(url); ok {
		if entry, ok := cached.(*CacheEntry); ok {
			// Cache hit - no logging to reduce noise
			entry.timestamp = time.Now()
			s.cache.Store(url, entry)
			return entry.resource
		}
	}

	if _, loading := s.loading.LoadOrStore(url, struct{}{}); loading {
		return s.fallback
	}

	go s.loadImageAsync(url, nil)
	return s.fallback
}

func (s *ImageService) GetImageWithCallback(url string, callback func(fyne.Resource, error)) fyne.Resource {
	if url == "" {
		fyne.Do(func() {
			callback(s.fallback, nil)
		})
		return s.fallback
	}

	if cached, ok := s.cache.Load(url); ok {
		if entry, ok := cached.(*CacheEntry); ok {
			// Cache hit - no logging to reduce noise
			entry.timestamp = time.Now()
			s.cache.Store(url, entry)
			fyne.Do(func() {
				callback(entry.resource, nil)
			})
			return entry.resource
		}
	}

	s.addCallback(url, callback)

	if _, loading := s.loading.LoadOrStore(url, struct{}{}); loading {
		return s.fallback
	}

	go s.loadImageAsync(url, callback)
	return s.fallback
}

func (s *ImageService) GetImageWithSize(url string, size fyne.Size, callback func(fyne.Resource, error)) fyne.Resource {
	cacheKey := s.generateCacheKey(url, size)

	if cached, ok := s.cache.Load(cacheKey); ok {
		if entry, ok := cached.(*CacheEntry); ok {
			entry.timestamp = time.Now()
			s.cache.Store(cacheKey, entry)
			if callback != nil {
				fyne.Do(func() {
					callback(entry.resource, nil)
				})
			}
			return entry.resource
		}
	}

	if callback != nil {
		s.addCallback(cacheKey, callback)
	}

	if _, loading := s.loading.LoadOrStore(cacheKey, struct{}{}); loading {
		return s.fallback
	}

	go s.loadImageWithSizeAsync(url, size, cacheKey)
	return s.fallback
}

func (s *ImageService) generateCacheKey(url string, size fyne.Size) string {
	return url
}

func (s *ImageService) loadImageWithSizeAsync(url string, size fyne.Size, cacheKey string) {
	defer s.loading.Delete(cacheKey)

	startTime := time.Now()
	resource, err := s.loader.GetResource(url)
	loadTime := time.Since(startTime)

	if err != nil {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Failed to load sized image %s in %v: %v", url, loadTime, err)
		}
		resource = s.fallback
	} else if resource == nil {
		resource = s.fallback
	}

	entry := &CacheEntry{
		resource:  resource,
		timestamp: time.Now(),
		url:       url,
		size:      0,
	}
	s.cache.Store(cacheKey, entry)

	s.notifyCallbacks(cacheKey, resource, err)
}

func (s *ImageService) addCallback(key string, callback func(fyne.Resource, error)) {
	if callback == nil {
		return
	}

	value, _ := s.callbacks.LoadOrStore(key, &CallbackList{})
	if callbackList, ok := value.(*CallbackList); ok {
		callbackList.mutex.Lock()
		callbackList.callbacks = append(callbackList.callbacks, callback)
		callbackList.mutex.Unlock()
	}
}

func (s *ImageService) loadImageAsync(url string, priorityCallback func(fyne.Resource, error)) {
	defer s.loading.Delete(url)

	var resource fyne.Resource
	var err error

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		startTime := time.Now()
		resource, err = s.loader.GetResource(url)
		loadTime := time.Since(startTime)

		if err == nil && resource != nil {
			// Only log on first successful load and if debug enabled
			if s.debug && attempt == 0 {
				log.Printf("[IMAGE_SERVICE] Successfully loaded image: %s in %v", url, loadTime)
			}
			break
		}

		if s.debug {
			log.Printf("[IMAGE_SERVICE] Failed to load image %s in %v (attempt %d): %v",
				url, loadTime, attempt+1, err)
		}

		if attempt < s.maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	if err != nil || resource == nil {
		resource = s.fallback
	}

	entry := &CacheEntry{
		resource:  resource,
		timestamp: time.Now(),
		url:       url,
		size:      0,
	}
	s.cache.Store(url, entry)

	if priorityCallback != nil {
		fyne.Do(func() {
			priorityCallback(resource, err)
		})
	}

	s.notifyCallbacks(url, resource, err)
}

func (s *ImageService) notifyCallbacks(key string, resource fyne.Resource, err error) {
	if value, ok := s.callbacks.LoadAndDelete(key); ok {
		if callbackList, ok := value.(*CallbackList); ok {
			callbackList.mutex.Lock()
			callbacks := make([]func(fyne.Resource, error), len(callbackList.callbacks))
			copy(callbacks, callbackList.callbacks)
			callbackList.mutex.Unlock()

			for _, callback := range callbacks {
				if callback != nil {
					fyne.Do(func() {
						callback(resource, err)
					})
				}
			}
		}
	}
}

func (s *ImageService) PreloadImages(urls []string) {
	if s.debug {
		log.Printf("[IMAGE_SERVICE] Preloading %d images", len(urls))
	}

	for i, url := range urls {
		if url != "" && !s.isInCache(url) && !s.isLoading(url) {
			go s.loadImageAsync(url, nil)

			if i%5 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

func (s *ImageService) PreloadImagesWithPriority(urls []string, priority []string) {
	prioritySet := make(map[string]bool)
	for _, url := range priority {
		prioritySet[url] = true
	}

	for _, url := range priority {
		if url != "" && !s.isInCache(url) && !s.isLoading(url) {
			go s.loadImageAsync(url, nil)
		}
	}

	time.Sleep(100 * time.Millisecond)

	for _, url := range urls {
		if url != "" && !prioritySet[url] && !s.isInCache(url) && !s.isLoading(url) {
			go s.loadImageAsync(url, nil)
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *ImageService) isInCache(url string) bool {
	_, ok := s.cache.Load(url)
	return ok
}

func (s *ImageService) isLoading(url string) bool {
	_, loading := s.loading.LoadOrStore(url, struct{}{})
	if loading {
		return true
	}
	s.loading.Delete(url)
	return false
}

func (s *ImageService) ClearCache() {
	if s.debug {
		log.Printf("[IMAGE_SERVICE] Clearing cache")
	}
	s.cache.Range(func(key, value interface{}) bool {
		s.cache.Delete(key)
		return true
	})
}

func (s *ImageService) CleanupOldEntries(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	s.cache.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if entry, ok := value.(*CacheEntry); ok {
				if entry.timestamp.Before(cutoff) {
					toDelete = append(toDelete, keyStr)
				}
			}
		}
		return true
	})

	for _, key := range toDelete {
		s.cache.Delete(key)
	}

	if s.debug && len(toDelete) > 0 {
		log.Printf("[IMAGE_SERVICE] Cleaned up %d old cache entries", len(toDelete))
	}
}

func (s *ImageService) GetCacheSize() int {
	count := 0
	s.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (s *ImageService) GetCacheStats() map[string]interface{} {
	var totalSize int64
	itemCount := 0
	oldestTime := time.Now()
	newestTime := time.Time{}

	s.cache.Range(func(key, value interface{}) bool {
		itemCount++
		if entry, ok := value.(*CacheEntry); ok {
			totalSize += entry.size
			if entry.timestamp.Before(oldestTime) {
				oldestTime = entry.timestamp
			}
			if entry.timestamp.After(newestTime) {
				newestTime = entry.timestamp
			}
		}
		return true
	})

	loadingCount := 0
	s.loading.Range(func(key, value interface{}) bool {
		loadingCount++
		return true
	})

	return map[string]interface{}{
		"cached_items":  itemCount,
		"loading_items": loadingCount,
		"total_size":    totalSize,
		"oldest_cached": oldestTime,
		"newest_cached": newestTime,
	}
}

// SetDebug enables or disables debug logging
func (s *ImageService) SetDebug(debug bool) {
	s.debug = debug
}
