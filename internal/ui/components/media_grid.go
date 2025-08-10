package components

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
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
	maxItems      int
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
		debug:        false,
		maxItems:     1000,
	}
	grid.ExtendBaseWidget(grid)
	return grid
}

func (mg *MediaGrid) SetItems(items []MediaItem) {
	mg.items = items
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
		mg.itemSize = fyne.NewSize(140, 180)
	} else {
		mg.columns = 4
		mg.itemSize = fyne.NewSize(180, 220)
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

	if r.grid.virtualScroll && rows > 10 {
		rows = 10
	}

	width := float32(cols) * r.grid.itemSize.Width
	height := float32(rows) * r.grid.itemSize.Height

	return fyne.NewSize(width, height)
}

func (r *mediaGridRenderer) Refresh() {
	r.calculateColumns(r.grid.Size())

	itemsToShow := r.grid.items
	if r.grid.virtualScroll && len(itemsToShow) > r.grid.maxItems {
		itemsToShow = itemsToShow[:r.grid.maxItems]
	}

	r.container.Objects = make([]fyne.CanvasObject, 0, len(itemsToShow))

	for i, item := range itemsToShow {
		card := NewMediaCard(item, r.grid.itemSize, r.grid.imageService, false)
		if r.grid.onItemTap != nil {
			index := i
			card.SetTapCallback(func() {
				r.grid.onItemTap(index)
			})
		}
		r.container.Objects = append(r.container.Objects, card)
	}

	r.container.Refresh()
}

func (r *mediaGridRenderer) calculateColumns(size fyne.Size) {
	if r.grid.itemSize.Width > 0 && size.Width > 0 {
		minCols := 1
		maxCols := 8
		idealCols := int(size.Width / r.grid.itemSize.Width)

		if idealCols < minCols {
			idealCols = minCols
		}
		if idealCols > maxCols {
			idealCols = maxCols
		}

		if idealCols != r.grid.columns {
			r.grid.columns = idealCols
			r.container = container.NewGridWithColumns(idealCols)
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

	image     *canvas.Image
	title     *widget.Label
	subtitle  *widget.Label
	overlay   *canvas.Rectangle
	hovered   bool
	container *fyne.Container
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

	imageSize := fyne.NewSize(size.Width-20, size.Width-20)
	card.image.SetMinSize(imageSize)
	card.image.Resize(imageSize)

	card.title = widget.NewLabel(truncateText(item.Title, 25))
	card.title.Alignment = fyne.TextAlignCenter
	card.title.TextStyle = fyne.TextStyle{Bold: true}
	card.title.Wrapping = fyne.TextWrapWord

	card.subtitle = widget.NewLabel(truncateText(item.Subtitle, 30))
	card.subtitle.Alignment = fyne.TextAlignCenter
	card.subtitle.TextStyle = fyne.TextStyle{}

	card.overlay = canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 0})
	card.overlay.Hide()

	card.ExtendBaseWidget(card)

	if item.ImageURL != "" && imageService != nil {
		imageService.GetImageWithCallback(item.ImageURL, func(resource fyne.Resource, err error) {
			if err == nil && resource != nil {
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

func (mc *MediaCard) Tapped(*fyne.PointEvent) {
	if mc.onTap != nil {
		mc.onTap()
	}
}

func (mc *MediaCard) TappedSecondary(*fyne.PointEvent) {}

func (mc *MediaCard) MouseIn(*desktop.MouseEvent) {
	mc.hovered = true
	mc.overlay.FillColor = color.NRGBA{R: 255, G: 255, B: 255, A: 20}
	mc.overlay.Show()
	mc.overlay.Refresh()
}

func (mc *MediaCard) MouseMoved(*desktop.MouseEvent) {}

func (mc *MediaCard) MouseOut() {
	mc.hovered = false
	mc.overlay.Hide()
	mc.overlay.Refresh()
}

func (mc *MediaCard) SetTapCallback(callback func()) {
	mc.onTap = callback
}

type mediaCardRenderer struct {
	card *MediaCard
}

func (r *mediaCardRenderer) Layout(size fyne.Size) {
	if r.card.container != nil {
		r.card.container.Resize(size)
	}
}

func (r *mediaCardRenderer) MinSize() fyne.Size {
	return r.card.size
}

func (r *mediaCardRenderer) Refresh() {
	imageContainer := container.NewMax(
		r.card.image,
		r.card.overlay,
	)

	textContainer := container.NewVBox(
		r.card.title,
		r.card.subtitle,
	)

	cardContent := container.New(
		layout.NewBorderLayout(nil, textContainer, nil, nil),
		textContainer,
		imageContainer,
	)

	r.card.container = cardContent
}

func (r *mediaCardRenderer) Objects() []fyne.CanvasObject {
	if r.card.container == nil {
		r.Refresh()
	}
	return []fyne.CanvasObject{r.card.container}
}

func (r *mediaCardRenderer) Destroy() {}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

func MediaItemFromSong(song *types.Song) MediaItem {
	subtitle := getArtistNamesForSong(song.Authors)
	imageURL := ""
	if song.ImageCropped != nil && *song.ImageCropped != "" {
		imageURL = *song.ImageCropped
	} else if song.Image != nil && *song.Image != "" {
		imageURL = *song.Image
	}
	return MediaItem{Title: song.Name, Subtitle: subtitle, ImageURL: imageURL, Data: song}
}

func MediaItemFromAlbum(album *types.Album) MediaItem {
	subtitle := getArtistNamesForAlbum(album.Artists)
	imageURL := ""
	if album.ImageCropped != nil && *album.ImageCropped != "" {
		imageURL = *album.ImageCropped
	} else if album.Image != nil && *album.Image != "" {
		imageURL = *album.Image
	}
	return MediaItem{Title: album.Name, Subtitle: subtitle, ImageURL: imageURL, Data: album}
}

func MediaItemFromAuthor(author *types.Author) MediaItem {
	subtitle := fmt.Sprintf("%d songs", len(author.Songs))
	if len(author.Albums) > 0 {
		subtitle = fmt.Sprintf("%d albums", len(author.Albums))
	}
	imageURL := ""
	if author.ImageCropped != nil && *author.ImageCropped != "" {
		imageURL = *author.ImageCropped
	} else if author.Image != nil && *author.Image != "" {
		imageURL = *author.Image
	}
	return MediaItem{Title: author.Name, Subtitle: subtitle, ImageURL: imageURL, Data: author}
}

func getArtistNamesForSong(authors []*types.Author) string {
	if len(authors) == 0 {
		return UnknownArtist
	}

	names := make([]string, 0, len(authors))
	for _, author := range authors {
		if author != nil && author.Name != "" {
			names = append(names, author.Name)
		}
	}

	if len(names) == 0 {
		return UnknownArtist
	}

	if len(names) == 1 {
		return names[0]
	}

	if len(names) == 2 {
		return names[0] + " & " + names[1]
	}

	return strings.Join(names[:2], ", ") + fmt.Sprintf(" +%d", len(names)-2)
}

func getArtistNamesForAlbum(artists []*types.Author) string {
	if len(artists) == 0 {
		return UnknownArtist
	}

	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		if artist != nil && artist.Name != "" {
			names = append(names, artist.Name)
		}
	}

	if len(names) == 0 {
		return UnknownArtist
	}

	return strings.Join(names, ", ")
}
