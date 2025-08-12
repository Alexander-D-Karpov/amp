package components

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type SongList struct {
	widget.BaseWidget

	songs []*types.Song

	// callbacks
	onPlay       func(*types.Song, []*types.Song)
	onDownload   func(*types.Song)
	onOpenAlbum  func(slug string)
	onOpenAuthor func(slug string)
	onOpenSong   func(slug string) // optional: open detailed song view

	root *fyne.Container
}

func NewSongList() *SongList {
	sl := &SongList{}
	sl.ExtendBaseWidget(sl)
	return sl
}

func (sl *SongList) CreateRenderer() fyne.WidgetRenderer {
	if sl.root == nil {
		sl.root = container.NewVBox()
	}
	return &songListRenderer{sl: sl}
}

func (sl *SongList) SetSongs(songs []*types.Song) {
	sl.songs = songs
	sl.Refresh()
}

func (sl *SongList) OnPlay(cb func(*types.Song, []*types.Song)) { sl.onPlay = cb }
func (sl *SongList) OnDownload(cb func(*types.Song))            { sl.onDownload = cb }
func (sl *SongList) OnOpenAlbum(cb func(slug string))           { sl.onOpenAlbum = cb }
func (sl *SongList) OnOpenAuthor(cb func(slug string))          { sl.onOpenAuthor = cb }
func (sl *SongList) OnOpenSong(cb func(slug string))            { sl.onOpenSong = cb }

type songListRenderer struct {
	sl *SongList
}

func (r *songListRenderer) Layout(size fyne.Size) { r.sl.root.Resize(size) }
func (r *songListRenderer) MinSize() fyne.Size    { return r.sl.root.MinSize() }
func (r *songListRenderer) Destroy()              {}
func (r *songListRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.sl.root}
}

func (r *songListRenderer) Refresh() {
	r.sl.root.Objects = nil

	if len(r.sl.songs) == 0 {
		empty := widget.NewLabel("No songs")
		r.sl.root.Add(empty)
		r.sl.root.Refresh()
		return
	}

	// header
	header := container.NewHBox(
		widget.NewLabelWithStyle(" ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Title", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Artist(s)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Len", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	r.sl.root.Add(header)

	for _, s := range r.sl.songs {
		row := r.makeRow(s)
		r.sl.root.Add(row)
	}

	r.sl.root.Refresh()
}

func (r *songListRenderer) makeRow(s *types.Song) fyne.CanvasObject {
	// play / pause button
	playBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if r.sl.onPlay != nil {
			r.sl.onPlay(s, r.sl.songs)
		}
	})

	// title “link”
	title := s.Name
	if title == "" {
		title = "Untitled"
	}
	titleBtn := widget.NewButton(title, func() {
		// open song detail (preferred) if set, fallback to album
		if r.sl.onOpenSong != nil && s.Slug != "" {
			r.sl.onOpenSong(s.Slug)
			return
		}
		if r.sl.onOpenAlbum != nil && s.Album != nil && s.Album.Slug != "" {
			r.sl.onOpenAlbum(s.Album.Slug)
		}
	})
	titleBtn.Importance = widget.MediumImportance
	titleBtn.Alignment = widget.ButtonAlignLeading

	// authors “chips”
	authorsBox := container.NewHBox()
	if len(s.Authors) == 0 {
		authorsBox.Add(widget.NewLabel("Unknown Artist"))
	} else {
		for i, a := range s.Authors {
			if a == nil {
				continue
			}
			txt := a.Name
			if txt == "" {
				txt = "Unknown"
			}
			btn := widget.NewButtonWithIcon(txt, theme.AccountIcon(), func(slug string) func() {
				return func() {
					if r.sl.onOpenAuthor != nil && slug != "" {
						r.sl.onOpenAuthor(slug)
					}
				}
			}(a.Slug))
			btn.Importance = widget.LowImportance
			btn.Alignment = widget.ButtonAlignLeading
			authorsBox.Add(btn)

			if i < len(s.Authors)-1 {
				authorsBox.Add(widget.NewLabel(", "))
			}
		}
	}

	// duration
	dur := "-"
	if s.Length > 0 {
		dur = fmtDuration(s.Length)
	}
	durLbl := widget.NewLabel(dur)

	// download (optional)
	downloadBtn := widget.NewButtonWithIcon("", theme.DownloadIcon(), func() {
		if r.sl.onDownload != nil {
			r.sl.onDownload(s)
		}
	})

	row := container.NewBorder(
		nil, nil,
		container.NewHBox(playBtn),
		container.NewHBox(downloadBtn),
		container.NewGridWithColumns(3, titleBtn, authorsBox, durLbl),
	)
	return row
}

func fmtDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}

func joinNonEmpty(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}
