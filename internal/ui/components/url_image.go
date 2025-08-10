package components

import (
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/media"
)

type URLImage struct {
	widget.BaseWidget

	loader      *media.ImageLoader
	image       *canvas.Image
	url         string
	placeholder fyne.Resource
	debug       bool
	mu          sync.RWMutex
	loading     bool
	defaultSize fyne.Size
	fillMode    canvas.ImageFill
	scaleMode   canvas.ImageScale
}

func NewURLImage(loader *media.ImageLoader, placeholder fyne.Resource, debug bool) *URLImage {
	img := &URLImage{
		loader:      loader,
		placeholder: placeholder,
		debug:       debug,
		loading:     false,
		defaultSize: fyne.NewSize(150, 150),
		fillMode:    canvas.ImageFillContain,
		scaleMode:   canvas.ImageScaleSmooth,
	}

	img.image = canvas.NewImageFromResource(placeholder)
	img.image.FillMode = img.fillMode
	img.image.ScaleMode = img.scaleMode
	img.image.SetMinSize(img.defaultSize)

	img.ExtendBaseWidget(img)

	if debug {
		log.Printf("[URL_IMAGE] Created with default size: %v, fill mode: %v",
			img.defaultSize, img.fillMode)
	}

	return img
}

func NewURLImageWithSize(loader *media.ImageLoader, placeholder fyne.Resource, size fyne.Size, debug bool) *URLImage {
	img := NewURLImage(loader, placeholder, debug)
	img.SetSize(size)
	return img
}

func (i *URLImage) CreateRenderer() fyne.WidgetRenderer {
	return &urlImageRenderer{image: i}
}

type urlImageRenderer struct {
	image *URLImage
}

func (r *urlImageRenderer) Layout(size fyne.Size) {
	if r.image.image != nil {
		r.image.image.Resize(size)
	}
}

func (r *urlImageRenderer) MinSize() fyne.Size {
	return r.image.defaultSize
}

func (r *urlImageRenderer) Refresh() {
	if r.image.image != nil {
		r.image.image.Refresh()
	}
}

func (r *urlImageRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.image.image}
}

func (r *urlImageRenderer) Destroy() {}

func (i *URLImage) SetURL(url string) {
	i.mu.Lock()
	if i.url == url || i.loading {
		i.mu.Unlock()
		return
	}

	i.url = url
	i.loading = true
	i.mu.Unlock()

	if i.debug {
		log.Printf("[URL_IMAGE] Setting URL: %s", url)
	}

	if url == "" {
		fyne.Do(func() {
			i.image.Resource = i.placeholder
			i.image.SetMinSize(i.defaultSize)
			i.image.Refresh()
		})
		i.mu.Lock()
		i.loading = false
		i.mu.Unlock()
		return
	}

	go i.loadImage(url)
}

func (i *URLImage) loadImage(url string) {
	defer func() {
		i.mu.Lock()
		i.loading = false
		i.mu.Unlock()
	}()

	if i.debug {
		log.Printf("[URL_IMAGE] Loading image: %s", url)
	}

	startTime := time.Now()
	res, err := i.loader.GetResource(url)
	loadTime := time.Since(startTime)

	if err != nil || res == nil {
		if i.debug {
			log.Printf("[URL_IMAGE] Failed to load image %s in %v: %v", url, loadTime, err)
		}

		fyne.Do(func() {
			i.image.Resource = i.placeholder
			i.image.SetMinSize(i.defaultSize)
			i.image.Refresh()
		})
		return
	}

	if i.debug {
		log.Printf("[URL_IMAGE] Successfully loaded image: %s in %v", url, loadTime)
	}

	fyne.Do(func() {
		i.mu.RLock()
		currentURL := i.url
		i.mu.RUnlock()

		if currentURL == url && res != nil {
			i.image.Resource = res
			i.image.SetMinSize(i.defaultSize)
			i.image.FillMode = i.fillMode
			i.image.ScaleMode = i.scaleMode
			i.image.Refresh()

			if i.debug {
				log.Printf("[URL_IMAGE] Applied image resource for: %s", url)
			}
		}
	})
}

func (i *URLImage) SetSize(size fyne.Size) {
	i.mu.Lock()
	i.defaultSize = size
	i.mu.Unlock()

	fyne.Do(func() {
		if i.image != nil {
			i.image.SetMinSize(size)
			i.image.Resize(size)
			i.Refresh()
		}
	})

	if i.debug {
		log.Printf("[URL_IMAGE] Size set to: %v", size)
	}
}

func (i *URLImage) SetFillMode(mode canvas.ImageFill) {
	i.mu.Lock()
	i.fillMode = mode
	i.mu.Unlock()

	fyne.Do(func() {
		if i.image != nil {
			i.image.FillMode = mode
			i.image.Refresh()
		}
	})

	if i.debug {
		log.Printf("[URL_IMAGE] Fill mode set to: %v", mode)
	}
}

func (i *URLImage) SetScaleMode(mode canvas.ImageScale) {
	i.mu.Lock()
	i.scaleMode = mode
	i.mu.Unlock()

	fyne.Do(func() {
		if i.image != nil {
			i.image.ScaleMode = mode
			i.image.Refresh()
		}
	})

	if i.debug {
		log.Printf("[URL_IMAGE] Scale mode set to: %v", mode)
	}
}

func (i *URLImage) Resize(size fyne.Size) {
	i.BaseWidget.Resize(size)
	if i.image != nil {
		fyne.Do(func() {
			i.image.Resize(size)
		})
	}
}

func (i *URLImage) GetURL() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.url
}

func (i *URLImage) IsLoading() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.loading
}

func (i *URLImage) GetSize() fyne.Size {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.defaultSize
}

func (i *URLImage) GetFillMode() canvas.ImageFill {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.fillMode
}

func (i *URLImage) GetScaleMode() canvas.ImageScale {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.scaleMode
}

func (i *URLImage) Reset() {
	i.mu.Lock()
	i.url = ""
	i.loading = false
	i.mu.Unlock()

	fyne.Do(func() {
		if i.image != nil {
			i.image.Resource = i.placeholder
			i.image.SetMinSize(i.defaultSize)
			i.image.Refresh()
		}
	})

	if i.debug {
		log.Printf("[URL_IMAGE] Reset to placeholder")
	}
}
