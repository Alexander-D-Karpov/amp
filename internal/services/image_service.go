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
	loader    *media.ImageLoader
	cache     sync.Map
	loading   sync.Map
	fallback  fyne.Resource
	callbacks sync.Map
	debug     bool
}

type CacheEntry struct {
	resource  fyne.Resource
	timestamp time.Time
}

func NewImageService(loader *media.ImageLoader) *ImageService {
	return &ImageService{
		loader:   loader,
		fallback: theme.MediaMusicIcon(),
		debug:    true,
	}
}

func (s *ImageService) GetImage(url string) fyne.Resource {
	if url == "" {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Empty URL provided")
		}
		return s.fallback
	}

	if cached, ok := s.cache.Load(url); ok {
		if entry, ok := cached.(*CacheEntry); ok {
			if s.debug {
				log.Printf("[IMAGE_SERVICE] Cache hit for: %s", url)
			}
			return entry.resource
		}
	}

	if _, loading := s.loading.LoadOrStore(url, struct{}{}); loading {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Already loading: %s", url)
		}
		return s.fallback
	}

	if s.debug {
		log.Printf("[IMAGE_SERVICE] Starting async load for: %s", url)
	}
	go s.loadImageAsync(url)
	return s.fallback
}

func (s *ImageService) GetImageWithCallback(url string, callback func(fyne.Resource, error)) fyne.Resource {
	if url == "" {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Empty URL provided to GetImageWithCallback")
		}
		fyne.Do(func() {
			callback(s.fallback, nil)
		})
		return s.fallback
	}

	if cached, ok := s.cache.Load(url); ok {
		if entry, ok := cached.(*CacheEntry); ok {
			if s.debug {
				log.Printf("[IMAGE_SERVICE] Cache hit for callback: %s", url)
			}
			fyne.Do(func() {
				callback(entry.resource, nil)
			})
			return entry.resource
		}
	}

	s.addCallback(url, callback)

	if _, loading := s.loading.LoadOrStore(url, struct{}{}); loading {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Already loading (with callback): %s", url)
		}
		return s.fallback
	}

	if s.debug {
		log.Printf("[IMAGE_SERVICE] Starting async load with callback for: %s", url)
	}
	go s.loadImageAsync(url)
	return s.fallback
}

func (s *ImageService) addCallback(url string, callback func(fyne.Resource, error)) {
	callbacks, _ := s.callbacks.LoadOrStore(url, make([]func(fyne.Resource, error), 0))
	if callbackList, ok := callbacks.([]func(fyne.Resource, error)); ok {
		callbackList = append(callbackList, callback)
		s.callbacks.Store(url, callbackList)
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Added callback for: %s (total callbacks: %d)", url, len(callbackList))
		}
	}
}

func (s *ImageService) loadImageAsync(url string) {
	defer s.loading.Delete(url)

	if s.debug {
		log.Printf("[IMAGE_SERVICE] Loading image: %s", url)
	}

	startTime := time.Now()
	resource, err := s.loader.GetResource(url)
	loadTime := time.Since(startTime)

	if err != nil {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Failed to load image %s in %v: %v", url, loadTime, err)
		}
		resource = s.fallback
	} else if resource == nil {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Got nil resource for: %s in %v", url, loadTime)
		}
		resource = s.fallback
	} else {
		if s.debug {
			log.Printf("[IMAGE_SERVICE] Successfully loaded image: %s in %v", url, loadTime)
		}
	}

	entry := &CacheEntry{
		resource:  resource,
		timestamp: time.Now(),
	}
	s.cache.Store(url, entry)

	s.notifyCallbacks(url, resource, err)
}

func (s *ImageService) notifyCallbacks(url string, resource fyne.Resource, err error) {
	if callbacks, ok := s.callbacks.LoadAndDelete(url); ok {
		if callbackList, ok := callbacks.([]func(fyne.Resource, error)); ok {
			if s.debug {
				log.Printf("[IMAGE_SERVICE] Notifying %d callbacks for: %s", len(callbackList), url)
			}
			for i, callback := range callbackList {
				if callback != nil {
					fyne.Do(func() {
						if s.debug {
							log.Printf("[IMAGE_SERVICE] Executing callback %d for: %s", i+1, url)
						}
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
			if s.debug {
				log.Printf("[IMAGE_SERVICE] Preloading image %d/%d: %s", i+1, len(urls), url)
			}
			go s.loadImageAsync(url)
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
