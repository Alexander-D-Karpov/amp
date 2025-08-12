package views

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/internal/ui/components"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type AlbumDetailView struct {
	imgSvc   *services.ImageService
	songList *components.SongList

	root     *fyne.Container
	backBtn  *widget.Button
	titleLbl *widget.Label
	cover    *canvas.Image
	authors  *fyne.Container
	metaLbl  *widget.Label

	album *types.Album

	onBack       func()
	onPlaySong   func(*types.Song)
	onOpenAlbum  func(string)
	onOpenAuthor func(string)
	onOpenSong   func(*types.Song)
}

func NewAlbumDetailView(img *services.ImageService) *AlbumDetailView {
	v := &AlbumDetailView{imgSvc: img}
	v.build()
	return v
}

func (v *AlbumDetailView) build() {
	v.backBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		if v.onBack != nil {
			v.onBack()
		}
	})
	v.titleLbl = widget.NewLabel("")
	v.titleLbl.TextStyle = fyne.TextStyle{Bold: true}
	v.cover = canvas.NewImageFromResource(theme.FolderIcon())
	v.cover.FillMode = canvas.ImageFillContain
	v.metaLbl = widget.NewLabel("")
	v.authors = container.NewHBox()

	v.songList = components.NewSongList()
	v.songList.OnPlay(func(s *types.Song, _ []*types.Song) {
		if v.onPlaySong != nil {
			v.onPlaySong(s)
		}
	})
	v.songList.OnOpenAlbum(func(slug string) {
		if v.onOpenAlbum != nil {
			v.onOpenAlbum(slug)
		}
	})
	v.songList.OnOpenAuthor(func(slug string) {
		if v.onOpenAuthor != nil {
			v.onOpenAuthor(slug)
		}
	})

	left := container.NewGridWrap(fyne.NewSize(280, 280), v.cover)
	head := container.NewVBox(container.NewHBox(v.backBtn), v.titleLbl, v.authors, v.metaLbl)

	// Use container.NewBorder instead of trying to create an HSplit
	v.root = container.NewBorder(head, nil, left, nil, v.songList)
}

func (v *AlbumDetailView) SetCallbacks(onBack func(), onPlaySong func(*types.Song), onOpenAlbum func(string), onOpenAuthor func(string), onOpenSong func(*types.Song)) {
	v.onBack, v.onPlaySong, v.onOpenAlbum, v.onOpenAuthor, v.onOpenSong = onBack, onPlaySong, onOpenAlbum, onOpenAuthor, onOpenSong
}

func (v *AlbumDetailView) ShowAlbum(a *types.Album) {
	v.album = a
	if a == nil {
		return
	}
	v.titleLbl.SetText(a.Name)
	v.metaLbl.SetText(fmt.Sprintf("%d tracks", len(a.Songs)))

	v.authors.Objects = nil
	for _, ar := range a.Artists {
		if ar == nil {
			continue
		}
		btn := widget.NewButton(ar.Name, func(slug string) func() {
			return func() {
				if v.onOpenAuthor != nil {
					v.onOpenAuthor(slug)
				}
			}
		}(ar.Slug))
		btn.Importance = widget.LowImportance
		v.authors.Add(btn)
	}
	v.authors.Refresh()

	if v.imgSvc != nil {
		url := ""
		if a.ImageCropped != nil && *a.ImageCropped != "" {
			url = *a.ImageCropped
		} else if a.Image != nil {
			url = *a.Image
		}
		if url != "" {
			v.imgSvc.GetImageWithSize(url, fyne.NewSize(280, 280), func(res fyne.Resource, err error) {
				if err == nil && res != nil {
					v.cover.Resource = res
					v.cover.Refresh()
				}
			})
		}
	}

	v.songList.SetSongs(a.Songs)
	v.root.Refresh()
}

func (v *AlbumDetailView) Container() *fyne.Container { return v.root }

func (v *AlbumDetailView) SetAlbum(a *types.Album) {
	v.ShowAlbum(a)
}
