package components

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"math"
)

type waveformBar struct {
	widget.BaseWidget
	data []float64 // normalized 0..1
}

func newWaveformBar() *waveformBar {
	w := &waveformBar{}
	w.ExtendBaseWidget(w)
	return w
}

func (w *waveformBar) Clear() {
	w.data = nil
	w.Refresh()
}

// SetDataInt takes the raw API ints, normalizes to 0..1 by max, and refreshes.
func (w *waveformBar) SetDataInt(vol []int) {
	if len(vol) == 0 {
		w.Clear()
		return
	}
	max := 0
	for _, v := range vol {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		w.data = make([]float64, len(vol))
		w.Refresh()
		return
	}
	out := make([]float64, len(vol))
	for i, v := range vol {
		out[i] = float64(v) / float64(max) // 0..1
	}
	w.data = out
	w.Refresh()
}

func (w *waveformBar) MinSize() fyne.Size { return fyne.NewSize(10, 14) }

type waveformRenderer struct {
	w        *waveformBar
	track    *canvas.Rectangle
	bars     []*canvas.Rectangle
	objects  []fyne.CanvasObject
	barGap   float32
	barWidth float32
}

func (w *waveformBar) CreateRenderer() fyne.WidgetRenderer {
	r := &waveformRenderer{
		w:        w,
		track:    canvas.NewRectangle(theme.ShadowColor()),
		bars:     []*canvas.Rectangle{},
		barGap:   1, // px gap between bars
		barWidth: 2, // px min bar width
	}
	r.objects = []fyne.CanvasObject{r.track}
	return r
}

func (r *waveformRenderer) MinSize() fyne.Size           { return r.w.MinSize() }
func (r *waveformRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *waveformRenderer) Destroy()                     {}

func (r *waveformRenderer) Refresh() {
	r.Layout(r.w.Size())
	canvas.Refresh(r.track)
}

func (r *waveformRenderer) Layout(size fyne.Size) {
	// Track spans full area (very subtle background)
	r.track.Resize(size)

	// Decide how many bars fit
	bw := float32(math.Max(1, float64(r.barWidth)))
	bg := r.barGap
	available := size.Width
	if available <= 0 || size.Height <= 0 || len(r.w.data) == 0 {
		// hide bars
		for _, b := range r.bars {
			b.Hide()
		}
		return
	}

	// Compute how many bars we can draw with (bar+gap)
	step := bw + bg
	count := int(available / step)
	if count < 1 {
		count = 1
	}

	// Resample by segment averaging so the waveform keeps shape
	src := r.w.data
	if count > len(src) {
		count = len(src) // no need to upsample; just limit to source length
	}
	segment := float64(len(src)) / float64(count)

	// Ensure we have enough rectangles
	if len(r.bars) < count {
		for i := len(r.bars); i < count; i++ {
			rect := canvas.NewRectangle(theme.DisabledColor())
			r.bars = append(r.bars, rect)
			r.objects = append(r.objects, rect)
		}
	}
	// Hide extra bars if we have too many
	for i := count; i < len(r.bars); i++ {
		r.bars[i].Hide()
	}

	h := size.Height
	for i := 0; i < count; i++ {
		start := int(math.Floor(float64(i) * segment))
		end := int(math.Floor(float64(i+1) * segment))
		if end <= start {
			end = start + 1
		}
		if end > len(src) {
			end = len(src)
		}
		sum := 0.0
		for j := start; j < end; j++ {
			sum += src[j]
		}
		avg := sum / float64(end-start) // 0..1
		barHeight := float32(avg) * h
		if barHeight < 1 {
			barHeight = 1 // keep tiny line visible
		}

		x := float32(i) * step
		y := h - barHeight

		bar := r.bars[i]
		bar.Show()
		bar.Resize(fyne.NewSize(bw, barHeight))
		bar.Move(fyne.NewPos(x, y))
	}
}
