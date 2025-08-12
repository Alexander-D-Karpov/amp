package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type bufferBar struct {
	widget.BaseWidget
	value float64 // 0..1
}

func newBufferBar() *bufferBar {
	b := &bufferBar{}
	b.ExtendBaseWidget(b)
	return b
}

func (b *bufferBar) SetValue(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	b.value = v
	b.Refresh()
}

func (b *bufferBar) MinSize() fyne.Size { return fyne.NewSize(10, 4) }

type bufferBarRenderer struct {
	bar     *bufferBar
	track   *canvas.Rectangle
	fill    *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (b *bufferBar) CreateRenderer() fyne.WidgetRenderer {
	r := &bufferBarRenderer{
		bar:   b,
		track: canvas.NewRectangle(theme.ShadowColor()),   // dark track
		fill:  canvas.NewRectangle(theme.DisabledColor()), // GREY fill
	}
	r.objects = []fyne.CanvasObject{r.track, r.fill}
	return r
}

func (r *bufferBarRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)
	r.fill.Resize(fyne.NewSize(size.Width*float32(r.bar.value), r.track.Size().Height))
}
func (r *bufferBarRenderer) MinSize() fyne.Size           { return r.bar.MinSize() }
func (r *bufferBarRenderer) Refresh()                     { r.Layout(r.bar.Size()); canvas.Refresh(r.track) }
func (r *bufferBarRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *bufferBarRenderer) Destroy()                     {}
