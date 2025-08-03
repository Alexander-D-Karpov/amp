package components

import (
	"fmt"
	"image/color"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

const UnknownArtist = "Unknown Artist"

type MediaGrid struct {
	widget.BaseWidget
	items         []MediaItem
	itemSize      fyne.Size
	columns       int
	onItemTap     func(int)
	imageService  *services.ImageService
	compactMode   bool
	virtualScroll bool
	debug         bool
}

type MediaItem struct {
	Title    string
	Subtitle string
	ImageURL string
	Data     interface{}
}

func NewMediaGrid(itemSize fyne.Size, imageService *services.ImageService) *MediaGrid {
	grid := &MediaGrid{
		itemSize:     itemSize,
		imageService: imageService,
		columns:      4,
		debug:        true,
	}
	grid.ExtendBaseWidget(grid)
	return grid
}

func (mg *MediaGrid) SetItems(items []MediaItem) {
	mg.items = items
	if mg.debug {
		log.Printf("[MEDIA_GRID] Setting %d items", len(items))
	}
	mg.Refresh()
}

func (mg *MediaGrid) SetItemTapCallback(callback func(int)) {
	mg.onItemTap = callback
}

func (mg *MediaGrid) SetVirtualScroll(enabled bool) {
	mg.virtualScroll = enabled
	mg.Refresh()
}

func (mg *MediaGrid) SetCompactMode(compact bool) {
	mg.compactMode = compact
	if compact {
		mg.columns = 2
		mg.itemSize = fyne.NewSize(140, 200)
	} else {
		mg.columns = 4
		mg.itemSize = fyne.NewSize(180, 240)
	}
	if mg.debug {
		log.Printf("[MEDIA_GRID] Compact mode: %v, columns: %d, item size: %v", compact, mg.columns, mg.itemSize)
	}
	mg.Refresh()
}

func (mg *MediaGrid) CreateRenderer() fyne.WidgetRenderer {
	return &mediaGridRenderer{
		grid:      mg,
		container: container.NewGridWithColumns(mg.columns),
	}
}

type mediaGridRenderer struct {
	grid      *MediaGrid
	container *fyne.Container
}

func (r *mediaGridRenderer) Layout(size fyne.Size) {
	r.calculateColumns(size)
	r.container.Resize(size)
}

func (r *mediaGridRenderer) MinSize() fyne.Size {
	if len(r.grid.items) == 0 {
		return fyne.NewSize(200, 200)
	}

	cols := r.grid.columns
	rows := (len(r.grid.items) + cols - 1) / cols

	width := float32(cols) * r.grid.itemSize.Width
	height := float32(rows) * r.grid.itemSize.Height

	return fyne.NewSize(width, height)
}

func (r *mediaGridRenderer) Refresh() {
	r.calculateColumns(r.grid.Size())

	r.container.Objects = make([]fyne.CanvasObject, 0, len(r.grid.items))

	if r.grid.debug {
		log.Printf("[MEDIA_GRID] Refreshing with %d items, %d columns", len(r.grid.items), r.grid.columns)
	}

	for i, item := range r.grid.items {
		card := NewMediaCard(item, r.grid.itemSize, r.grid.imageService, r.grid.debug)
		if r.grid.onItemTap != nil {
			index := i
			card.SetTapCallback(func() {
				if r.grid.debug {
					log.Printf("[MEDIA_GRID] Item %d tapped: %s", index, item.Title)
				}
				r.grid.onItemTap(index)
			})
		}
		r.container.Objects = append(r.container.Objects, card)
	}

	r.container.Refresh()

	if r.grid.debug {
		log.Printf("[MEDIA_GRID] Refresh completed, container has %d objects", len(r.container.Objects))
	}
}

func (r *mediaGridRenderer) calculateColumns(size fyne.Size) {
	if r.grid.itemSize.Width > 0 && size.Width > 0 {
		cols := int(size.Width / r.grid.itemSize.Width)
		if cols < 1 {
			cols = 1
		}
		if cols != r.grid.columns {
			r.grid.columns = cols
			r.container = container.NewGridWithColumns(cols)
			if r.grid.debug {
				log.Printf("[MEDIA_GRID] Recalculated columns: %d for width: %.0f", cols, size.Width)
			}
		}
	}
}

func (r *mediaGridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.container}
}

func (r *mediaGridRenderer) Destroy() {}

type MediaCard struct {
	widget.BaseWidget
	item         MediaItem
	size         fyne.Size
	imageService *services.ImageService
	onTap        func()
	debug        bool

	image    *canvas.Image
	title    *widget.Label
	subtitle *widget.Label
	button   *widget.Button
}

func NewMediaCard(item MediaItem, size fyne.Size, imageService *services.ImageService, debug bool) *MediaCard {
	card := &MediaCard{
		item:         item,
		size:         size,
		imageService: imageService,
		debug:        debug,
	}

	card.image = canvas.NewImageFromResource(theme.MediaMusicIcon())
	card.image.FillMode = canvas.ImageFillContain
	card.image.ScaleMode = canvas.ImageScaleSmooth

	card.title = widget.NewLabel(item.Title)
	card.title.Alignment = fyne.TextAlignCenter
	card.title.TextStyle = fyne.TextStyle{Bold: true}
	card.title.Wrapping = fyne.TextWrapWord

	card.subtitle = widget.NewLabel(item.Subtitle)
	card.subtitle.Alignment = fyne.TextAlignCenter
	card.subtitle.Wrapping = fyne.TextWrapWord

	card.button = widget.NewButton("", func() {
		if card.onTap != nil {
			card.onTap()
		}
	})
	card.button.Importance = widget.LowImportance

	if len(card.title.Text) > 25 {
		card.title.SetText(card.title.Text[:25] + "...")
	}

	if len(card.subtitle.Text) > 30 {
		card.subtitle.SetText(card.subtitle.Text[:30] + "...")
	}

	card.ExtendBaseWidget(card)

	if debug {
		log.Printf("[MEDIA_CARD] Creating card for: %s, image URL: %s", item.Title, item.ImageURL)
	}

	if item.ImageURL != "" && imageService != nil {
		if debug {
			log.Printf("[MEDIA_CARD] Loading image for: %s from: %s", item.Title, item.ImageURL)
		}

		imageService.GetImageWithCallback(item.ImageURL, func(resource fyne.Resource, err error) {
			if err != nil {
				if debug {
					log.Printf("[MEDIA_CARD] Failed to load image for %s: %v", item.Title, err)
				}
				return
			}

			if resource != nil {
				if debug {
					log.Printf("[MEDIA_CARD] Successfully loaded image for: %s", item.Title)
				}
				card.image.Resource = resource
				card.image.Refresh()
			}
		})
	}

	return card
}

func (mc *MediaCard) CreateRenderer() fyne.WidgetRenderer {
	return &mediaCardRenderer{card: mc}
}

type mediaCardRenderer struct {
	card      *MediaCard
	container fyne.CanvasObject
}

func (r *mediaCardRenderer) Layout(size fyne.Size) {
	if r.container != nil {
		r.container.Resize(size)
	}
}

func (r *mediaCardRenderer) MinSize() fyne.Size {
	return r.card.size
}

func (r *mediaCardRenderer) Refresh() {
	imageHeight := r.card.size.Width - 20

	imageSizer := canvas.NewRectangle(color.Transparent)
	imageSizer.SetMinSize(fyne.NewSize(imageHeight, imageHeight))

	r.card.image.Resize(fyne.NewSize(imageHeight, imageHeight))

	imageContainer := container.NewStack(
		imageSizer,
		container.NewCenter(r.card.image),
	)

	textContainer := container.NewVBox(
		r.card.title,
		r.card.subtitle,
	)

	content := container.NewBorder(
		imageContainer,
		textContainer,
		nil,
		nil,
		nil,
	)

	cardWidget := widget.NewCard("", "", content)

	r.container = container.NewStack(
		cardWidget,
		r.card.button,
	)
}

func (r *mediaCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.container}
}

func (r *mediaCardRenderer) Destroy() {}

func (mc *MediaCard) SetTapCallback(callback func()) {
	mc.onTap = callback
}

func (mc *MediaCard) MinSize() fyne.Size {
	return mc.size
}

func MediaItemFromSong(song *types.Song) MediaItem {
	subtitle := UnknownArtist
	if len(song.Authors) > 0 && song.Authors[0] != nil {
		subtitle = song.Authors[0].Name
	}
	imageURL := ""
	if song.ImageCropped != nil && *song.ImageCropped != "" {
		imageURL = *song.ImageCropped
	} else if song.Image != nil && *song.Image != "" {
		imageURL = *song.Image
	}
	return MediaItem{Title: song.Name, Subtitle: subtitle, ImageURL: imageURL, Data: song}
}

func MediaItemFromAlbum(album *types.Album) MediaItem {
	subtitle := UnknownArtist
	if len(album.Artists) > 0 && album.Artists[0] != nil {
		subtitle = album.Artists[0].Name
	}
	imageURL := ""
	if album.ImageCropped != nil && *album.ImageCropped != "" {
		imageURL = *album.ImageCropped
	} else if album.Image != nil && *album.Image != "" {
		imageURL = *album.Image
	}
	return MediaItem{Title: album.Name, Subtitle: subtitle, ImageURL: imageURL, Data: album}
}

func MediaItemFromAuthor(author *types.Author) MediaItem {
	imageURL := ""
	if author.ImageCropped != nil && *author.ImageCropped != "" {
		imageURL = *author.ImageCropped
	} else if author.Image != nil && *author.Image != "" {
		imageURL = *author.Image
	}
	return MediaItem{Title: author.Name, Subtitle: fmt.Sprintf("%d songs", len(author.Songs)), ImageURL: imageURL, Data: author}
}

func MediaItemFromPlaylist(playlist *types.Playlist) MediaItem {
	imageURL := ""
	if len(playlist.Images) > 0 {
		imageURL = playlist.Images[0]
	}
	return MediaItem{Title: playlist.Name, Subtitle: fmt.Sprintf("%d songs", len(playlist.Songs)), ImageURL: imageURL, Data: playlist}
}

func GetArtistNames(authors []*types.Author) string {
	if len(authors) == 0 {
		return UnknownArtist
	}
	var names []string
	for _, author := range authors {
		if author != nil {
			names = append(names, author.Name)
		}
	}
	return strings.Join(names, ", ")
}
