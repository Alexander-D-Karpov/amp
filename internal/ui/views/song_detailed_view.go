package views

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type SongDetailView struct {
	imgSvc *services.ImageService

	root           *fyne.Container
	splitContainer *container.Split
	backBtn        *widget.Button
	playBtn        *widget.Button
	likeBtn        *widget.Button
	downloadBtn    *widget.Button
	titleLbl       *widget.Label
	artistsBox     *fyne.Container
	cover          *canvas.Image
	metaLbl        *widget.Label
	albumBtn       *widget.Button
	fileInfoLbl    *widget.Label

	song *types.Song

	onBack       func()
	onOpenAlbum  func(string)
	onOpenAuthor func(string)
	onPlay       func(*types.Song)
	onLike       func(*types.Song)
	onDownload   func(*types.Song)
}

func NewSongDetailView(img *services.ImageService) *SongDetailView {
	v := &SongDetailView{imgSvc: img}
	v.build()
	return v
}

func (v *SongDetailView) build() {
	v.backBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		if v.onBack != nil {
			v.onBack()
		}
	})

	v.playBtn = widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
		if v.onPlay != nil && v.song != nil {
			v.onPlay(v.song)
		}
	})
	v.playBtn.Importance = widget.HighImportance

	v.likeBtn = widget.NewButtonWithIcon("Like", theme.VisibilityOffIcon(), func() {
		if v.onLike != nil && v.song != nil {
			v.onLike(v.song)
		}
	})

	v.downloadBtn = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		if v.onDownload != nil && v.song != nil {
			v.onDownload(v.song)
		}
	})

	v.titleLbl = widget.NewLabel("")
	v.titleLbl.TextStyle = fyne.TextStyle{Bold: true}
	v.titleLbl.Wrapping = fyne.TextWrapWord

	v.artistsBox = container.NewHBox()
	v.cover = canvas.NewImageFromResource(theme.MediaMusicIcon())
	v.cover.FillMode = canvas.ImageFillContain
	v.metaLbl = widget.NewLabel("")
	v.albumBtn = widget.NewButton("", nil)
	v.albumBtn.Hide()
	v.fileInfoLbl = widget.NewLabel("")

	// Layout
	actionBtns := container.NewHBox(v.playBtn, v.likeBtn, v.downloadBtn)

	coverContainer := container.NewGridWrap(fyne.NewSize(300, 300), v.cover)

	infoContainer := container.NewVBox(
		container.NewHBox(v.backBtn),
		v.titleLbl,
		widget.NewLabel("by"),
		v.artistsBox,
		v.metaLbl,
		v.albumBtn,
		widget.NewSeparator(),
		actionBtns,
		widget.NewSeparator(),
		v.fileInfoLbl,
	)

	// Create the split container and set offset
	v.splitContainer = container.NewHSplit(coverContainer, infoContainer)
	v.splitContainer.Offset = 0.4 // 40% for cover, 60% for info

	// Wrap in a regular container
	v.root = container.NewBorder(nil, nil, nil, nil, v.splitContainer)
}

func (v *SongDetailView) SetSong(s *types.Song) {
	v.ShowSong(s)
}

func (v *SongDetailView) ShowSong(s *types.Song) {
	v.song = s
	if s == nil {
		return
	}

	// Title
	v.titleLbl.SetText(s.Name)

	// Artists
	v.artistsBox.Objects = nil
	if len(s.Authors) == 0 {
		v.artistsBox.Add(widget.NewLabel("Unknown Artist"))
	} else {
		for i, author := range s.Authors {
			if author == nil {
				continue
			}
			btn := widget.NewButton(author.Name, func(slug string) func() {
				return func() {
					if v.onOpenAuthor != nil {
						v.onOpenAuthor(slug)
					}
				}
			}(author.Slug))
			btn.Importance = widget.LowImportance
			v.artistsBox.Add(btn)

			if i < len(s.Authors)-1 {
				v.artistsBox.Add(widget.NewLabel("•"))
			}
		}
	}
	v.artistsBox.Refresh()

	// Meta info
	duration := formatDuration(s.Length)
	playCount := ""
	if s.Played > 0 {
		playCount = fmt.Sprintf(" • Played %d times", s.Played)
	}
	v.metaLbl.SetText(fmt.Sprintf("Duration: %s%s", duration, playCount))

	// Album
	if s.Album != nil && s.Album.Name != "" {
		v.albumBtn.SetText(fmt.Sprintf("From album: %s", s.Album.Name))
		v.albumBtn.OnTapped = func() {
			if v.onOpenAlbum != nil {
				v.onOpenAlbum(s.Album.Slug)
			}
		}
		v.albumBtn.Show()
	} else {
		v.albumBtn.Hide()
	}

	// File info
	fileInfo := []string{}
	if s.File != "" {
		// Extract filename from URL
		parts := strings.Split(s.File, "/")
		if len(parts) > 0 {
			filename := parts[len(parts)-1]
			fileInfo = append(fileInfo, fmt.Sprintf("File: %s", filename))
		}
	}
	if s.Downloaded {
		fileInfo = append(fileInfo, "✓ Downloaded")
		v.downloadBtn.SetText("Downloaded")
		v.downloadBtn.Disable()
	} else {
		v.downloadBtn.SetText("Download")
		v.downloadBtn.Enable()
	}
	v.fileInfoLbl.SetText(strings.Join(fileInfo, "\n"))

	// Like button
	v.updateLikeButton()

	// Cover image
	if v.imgSvc != nil {
		url := ""
		if s.ImageCropped != nil && *s.ImageCropped != "" {
			url = *s.ImageCropped
		} else if s.Image != nil && *s.Image != "" {
			url = *s.Image
		}

		if url != "" {
			v.imgSvc.GetImageWithSize(url, fyne.NewSize(300, 300), func(res fyne.Resource, err error) {
				if err == nil && res != nil {
					v.cover.Resource = res
					v.cover.Refresh()
				}
			})
		}
	}

	v.root.Refresh()
}

func (v *SongDetailView) updateLikeButton() {
	if v.song == nil {
		return
	}

	if v.song.Liked != nil && *v.song.Liked {
		v.likeBtn.SetText("Unlike")
		v.likeBtn.SetIcon(theme.ConfirmIcon())
		v.likeBtn.Importance = widget.MediumImportance
	} else {
		v.likeBtn.SetText("Like")
		v.likeBtn.SetIcon(theme.VisibilityOffIcon())
		v.likeBtn.Importance = widget.LowImportance
	}
	v.likeBtn.Refresh()
}

func (v *SongDetailView) SetOnBack(callback func()) {
	v.onBack = callback
}

func (v *SongDetailView) SetOnOpenAlbum(callback func(string)) {
	v.onOpenAlbum = callback
}

func (v *SongDetailView) SetOnOpenAuthor(callback func(string)) {
	v.onOpenAuthor = callback
}

func (v *SongDetailView) SetOnPlay(callback func(*types.Song)) {
	v.onPlay = callback
}

func (v *SongDetailView) SetOnLike(callback func(*types.Song)) {
	v.onLike = callback
}

func (v *SongDetailView) SetOnDownload(callback func(*types.Song)) {
	v.onDownload = callback
}

func (v *SongDetailView) Container() *fyne.Container {
	return v.root
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "0:00"
	}

	d := time.Duration(seconds) * time.Second
	minutes := int(d.Minutes())
	secs := int(d.Seconds()) % 60

	if minutes >= 60 {
		hours := minutes / 60
		minutes = minutes % 60
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}

	return fmt.Sprintf("%d:%02d", minutes, secs)
}
