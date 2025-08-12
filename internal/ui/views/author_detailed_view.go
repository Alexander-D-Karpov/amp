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

type AuthorDetailView struct {
	imgSvc   *services.ImageService
	songList *components.SongList
	albums   *components.MediaGrid

	root           *fyne.Container
	splitContainer *container.Split
	backBtn        *widget.Button
	nameLbl        *widget.Label
	avatar         *canvas.Image
	metaLbl        *widget.Label

	author *types.Author

	onBack       func()
	onPlaySong   func(*types.Song)
	onOpenAlbum  func(string)
	onOpenAuthor func(string)
}

func NewAuthorDetailView(img *services.ImageService) *AuthorDetailView {
	v := &AuthorDetailView{imgSvc: img}
	v.build()
	return v
}

func (v *AuthorDetailView) build() {
	v.backBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		if v.onBack != nil {
			v.onBack()
		}
	})
	v.nameLbl = widget.NewLabel("")
	v.nameLbl.TextStyle = fyne.TextStyle{Bold: true}
	v.avatar = canvas.NewImageFromResource(theme.AccountIcon())
	v.avatar.FillMode = canvas.ImageFillContain
	v.metaLbl = widget.NewLabel("")

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

	v.albums = components.NewMediaGrid(fyne.NewSize(200, 260), v.imgSvc)
	v.albums.SetItemTapCallback(func(i int) {
		if v.author != nil && i >= 0 && i < len(v.author.Albums) {
			a := v.author.Albums[i]
			if a != nil && v.onOpenAlbum != nil {
				v.onOpenAlbum(a.Slug)
			}
		}
	})

	left := container.NewGridWrap(fyne.NewSize(200, 200), v.avatar)
	head := container.NewVBox(container.NewHBox(v.backBtn), v.nameLbl, v.metaLbl, widget.NewSeparator(), widget.NewLabel("Albums"))
	albumsScroll := container.NewVScroll(container.NewStack(v.albums))

	// Create the split container and set offset
	v.splitContainer = container.NewHSplit(albumsScroll, v.songList)
	v.splitContainer.Offset = 0.45

	// Use border container with the split in the center
	v.root = container.NewBorder(head, nil, left, nil, v.splitContainer)
}

func (v *AuthorDetailView) SetCallbacks(onBack func(), onPlaySong func(*types.Song), onOpenAlbum func(string), onOpenAuthor func(string)) {
	v.onBack, v.onPlaySong, v.onOpenAlbum, v.onOpenAuthor = onBack, onPlaySong, onOpenAlbum, onOpenAuthor
}

func (v *AuthorDetailView) ShowAuthor(a *types.Author) {
	v.author = a
	if a == nil {
		return
	}
	v.nameLbl.SetText(a.Name)
	v.metaLbl.SetText(fmt.Sprintf("%d songs • %d albums", len(a.Songs), len(a.Albums)))

	// avatar
	if v.imgSvc != nil {
		url := ""
		if a.ImageCropped != nil && *a.ImageCropped != "" {
			url = *a.ImageCropped
		} else if a.Image != nil {
			url = *a.Image
		}
		if url != "" {
			v.imgSvc.GetImageWithSize(url, fyne.NewSize(200, 200), func(res fyne.Resource, err error) {
				if err == nil && res != nil {
					v.avatar.Resource = res
					v.avatar.Refresh()
				}
			})
		}
	}

	// songs
	v.songList.SetSongs(a.Songs)

	// albums grid items
	items := make([]components.MediaItem, 0, len(a.Albums))
	for _, al := range a.Albums {
		if al != nil {
			items = append(items, components.MediaItemFromAlbum(al))
		}
	}
	v.albums.SetItems(items)
	v.albums.Refresh()
	v.root.Refresh()
}

func (v *AuthorDetailView) Container() *fyne.Container { return v.root }

func (v *AuthorDetailView) SetAuthor(a *types.Author) {
	v.ShowAuthor(a)
}
