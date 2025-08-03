package components

import (
	"sync"

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
}

func NewURLImage(loader *media.ImageLoader, placeholder fyne.Resource, debug bool) *URLImage {
	img := &URLImage{
		loader:      loader,
		placeholder: placeholder,
		debug:       debug,
		loading:     false,
	}

	img.image = canvas.NewImageFromResource(placeholder)
	img.image.FillMode = canvas.ImageFillContain
	img.image.ScaleMode = canvas.ImageScaleSmooth
	img.ExtendBaseWidget(img)
	return img
}

func (i *URLImage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(i.image)
}

func (i *URLImage) SetURL(url string) {
	i.mu.Lock()
	if i.url == url || i.loading {
		i.mu.Unlock()
		return
	}

	i.url = url
	i.loading = true
	i.mu.Unlock()

	if url == "" {
		fyne.Do(func() {
			i.image.Resource = i.placeholder
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

	res, err := i.loader.GetResource(url)
	if err != nil || res == nil {
		fyne.Do(func() {
			i.image.Resource = i.placeholder
			i.image.Refresh()
		})
		return
	}

	fyne.Do(func() {
		i.mu.RLock()
		currentURL := i.url
		i.mu.RUnlock()

		if currentURL == url && res != nil {
			i.image.Resource = res
			i.image.Refresh()
		}
	})
}

func (i *URLImage) SetFillMode(mode canvas.ImageFill) {
	fyne.Do(func() {
		i.image.FillMode = mode
		i.Refresh()
	})
}

func (i *URLImage) SetScaleMode(mode canvas.ImageScale) {
	fyne.Do(func() {
		i.image.ScaleMode = mode
		i.Refresh()
	})
}

func (i *URLImage) Resize(size fyne.Size) {
	i.BaseWidget.Resize(size)
	if i.image != nil {
		fyne.Do(func() {
			i.image.Resize(size)
		})
	}
}
