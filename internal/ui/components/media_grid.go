package components

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

const UnknownArtist = "Unknown Artist"

type MediaGrid struct {
	widget.BaseWidget
	items              []MediaItem
	itemSize           fyne.Size
	columns            int
	onItemTap          func(int)
	onItemSecondaryTap func(int, fyne.Position)
	imageService       *services.ImageService
	compactMode        bool
	virtualScroll      bool
	debug              bool
	maxItems           int
	initialized        bool
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
		items:        make([]MediaItem, 0),
		initialized:  false,
	}
	grid.ExtendBaseWidget(grid)
	return grid
}

func (mg *MediaGrid) SetItems(items []MediaItem) {
	if mg == nil {
		return
	}

	if items == nil {
		items = make([]MediaItem, 0)
	}

	mg.items = items

	if mg.initialized {
		mg.Refresh()
	}
}

func (mg *MediaGrid) GetItems() []MediaItem {
	if mg == nil {
		return make([]MediaItem, 0)
	}
	return mg.items
}

func (mg *MediaGrid) SetItemTapCallback(callback func(int)) {
	if mg == nil {
		return
	}
	mg.onItemTap = callback
}

func (mg *MediaGrid) SetItemSecondaryTapCallback(callback func(int, fyne.Position)) {
	if mg == nil {
		return
	}
	mg.onItemSecondaryTap = callback
}

func (mg *MediaGrid) SetVirtualScroll(enabled bool) {
	if mg == nil {
		return
	}
	mg.virtualScroll = enabled
	if mg.initialized {
		mg.Refresh()
	}
}

func (mg *MediaGrid) SetCompactMode(compact bool) {
	if mg == nil {
		return
	}
	mg.compactMode = compact
	if compact {
		mg.columns = 2
		mg.itemSize = fyne.NewSize(160, 220)
	} else {
		mg.columns = 4
		mg.itemSize = fyne.NewSize(200, 280)
	}
	if mg.initialized {
		mg.Refresh()
	}
}

type mediaGridRenderer struct {
	grid      *MediaGrid
	container *fyne.Container
}

func (mg *MediaGrid) CreateRenderer() fyne.WidgetRenderer {
	if mg == nil {
		return &mediaGridRenderer{
			container: container.NewGridWithColumns(1),
		}
	}

	renderer := &mediaGridRenderer{
		grid:      mg,
		container: container.NewGridWithColumns(mg.columns),
	}

	mg.initialized = true
	renderer.Refresh()
	return renderer
}

func (r *mediaGridRenderer) MinSize() fyne.Size {
	if r.grid == nil || len(r.grid.items) == 0 {
		return fyne.NewSize(200, 200)
	}
	pad := theme.Padding()
	cols := maxInt(1, r.grid.columns)

	minW := r.grid.itemSize.Width + 2*pad

	rows := (len(r.grid.items) + cols - 1) / cols
	if r.grid.virtualScroll && rows > 10 {
		rows = 10
	}
	minH := float32(rows)*r.grid.itemSize.Height + float32(maxInt(0, rows-1))*pad

	return fyne.NewSize(minW, minH)
}

func (r *mediaGridRenderer) Refresh() {
	if r.container == nil {
		r.container = container.NewGridWithColumns(1)
		return
	}

	if r.grid == nil {
		r.container.Objects = []fyne.CanvasObject{}
		r.container.Refresh()
		return
	}

	r.calculateColumns(r.grid.Size())

	itemsToShow := r.grid.items
	if r.grid.virtualScroll && len(itemsToShow) > r.grid.maxItems {
		itemsToShow = itemsToShow[:r.grid.maxItems]
	}

	objs := make([]fyne.CanvasObject, 0, len(itemsToShow))
	for i, item := range itemsToShow {
		card := NewMediaCardWithContext(item, r.grid.itemSize, r.grid.imageService, r.grid.debug, i)

		// Set up tap callbacks properly
		if r.grid.onItemTap != nil {
			idx := i // Capture the index
			card.SetTapCallback(func() {
				if r.grid.debug {
					log.Printf("[MEDIA_GRID] Primary tap on item %d: %s", idx, item.Title)
				}
				r.grid.onItemTap(idx)
			})
		}

		if r.grid.onItemSecondaryTap != nil {
			idx := i // Capture the index
			card.SetSecondaryTapCallback(func(relativePos fyne.Position) {
				if r.grid.debug {
					log.Printf("[MEDIA_GRID] Secondary tap on item %d: %s at relative pos %v", idx, item.Title, relativePos)
				}

				// Calculate absolute position
				absolutePos := r.calculateAbsolutePosition(card, relativePos)

				if r.grid.debug {
					log.Printf("[MEDIA_GRID] Calculated absolute position: %v", absolutePos)
				}

				r.grid.onItemSecondaryTap(idx, absolutePos)
			})
		}

		objs = append(objs, card)
	}
	r.container.Objects = objs
	r.container.Refresh()
}

func (r *mediaGridRenderer) calculateAbsolutePosition(card *MediaCard, relativePos fyne.Position) fyne.Position {
	// Get the card's position within the grid container
	cardPos := card.Position()

	// Get the grid container's position
	gridPos := r.container.Position()

	// Calculate absolute position by adding all offsets
	absoluteX := gridPos.X + cardPos.X + relativePos.X
	absoluteY := gridPos.Y + cardPos.Y + relativePos.Y

	// Add some offset to prevent menu from appearing exactly at click point
	absoluteX += 5
	absoluteY += 5

	return fyne.NewPos(absoluteX, absoluteY)
}

func (r *mediaGridRenderer) Objects() []fyne.CanvasObject {
	if r.container == nil {
		return []fyne.CanvasObject{}
	}
	return []fyne.CanvasObject{r.container}
}

func (r *mediaGridRenderer) Destroy() {}

type MediaCard struct {
	widget.BaseWidget
	item           MediaItem
	size           fyne.Size
	imageService   *services.ImageService
	onTap          func()
	onSecondaryTap func(fyne.Position)
	debug          bool
	index          int

	image     *canvas.Image
	title     *widget.Label
	subtitle  *widget.Label
	overlay   *canvas.Rectangle
	hovered   bool
	container *fyne.Container

	lastTapTime    time.Time
	tapCount       int
	longPressTimer *time.Timer
	longPressPos   fyne.Position
}

func NewMediaCardWithContext(item MediaItem, size fyne.Size, imageService *services.ImageService, debug bool, index int) *MediaCard {
	card := &MediaCard{
		item:         item,
		size:         size,
		imageService: imageService,
		debug:        debug,
		index:        index,
		tapCount:     0,
	}

	const padding = float32(8)
	const textBlock = float32(56)
	imageHeight := size.Height - textBlock
	if imageHeight < 40 {
		imageHeight = 40
	}
	imageSize := fyne.NewSize(size.Width-2*padding, imageHeight)

	card.image = canvas.NewImageFromResource(theme.MediaMusicIcon())
	card.image.FillMode = canvas.ImageFillContain
	card.image.ScaleMode = canvas.ImageScaleSmooth
	card.image.SetMinSize(imageSize)
	card.image.Resize(imageSize)

	card.title = widget.NewLabel(item.Title)
	card.title.Alignment = fyne.TextAlignCenter
	card.title.TextStyle = fyne.TextStyle{Bold: true}
	card.title.Truncation = fyne.TextTruncateEllipsis
	card.title.Wrapping = fyne.TextWrapOff

	card.subtitle = widget.NewLabel(item.Subtitle)
	card.subtitle.Alignment = fyne.TextAlignCenter
	card.subtitle.Truncation = fyne.TextTruncateEllipsis
	card.subtitle.Wrapping = fyne.TextWrapOff

	card.overlay = canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 30})
	card.overlay.Hide()

	card.ExtendBaseWidget(card)

	if debug {
		log.Printf("[MEDIA_CARD] Created card for: %s (index: %d)", item.Title, index)
	}

	// Load image if available
	if item.ImageURL != "" && imageService != nil {
		imageService.GetImageWithSize(item.ImageURL, fyne.NewSize(size.Width-16, size.Height-60), func(res fyne.Resource, err error) {
			if err == nil && res != nil {
				card.image.Resource = res
				card.image.Refresh()
			}
		})
	}

	return card
}

func (mc *MediaCard) Tapped(event *fyne.PointEvent) {
	if mc.debug {
		log.Printf("[MEDIA_CARD] Primary tap on: %s", mc.item.Title)
	}

	now := time.Now()
	if now.Sub(mc.lastTapTime) < 500*time.Millisecond {
		mc.tapCount++
	} else {
		mc.tapCount = 1
	}
	mc.lastTapTime = now

	if mc.longPressTimer != nil {
		mc.longPressTimer.Stop()
		mc.longPressTimer = nil
	}

	// Handle double-tap as secondary on mobile
	if fyne.CurrentDevice().IsMobile() && mc.tapCount >= 2 {
		if mc.onSecondaryTap != nil {
			mc.onSecondaryTap(event.Position)
		}
		mc.tapCount = 0
		return
	}

	if mc.onTap != nil {
		mc.onTap()
	}
}

func (mc *MediaCard) TappedSecondary(event *fyne.PointEvent) {
	if mc.debug {
		log.Printf("[MEDIA_CARD] Secondary tap on card: %s at relative pos %v", mc.item.Title, event.Position)
	}

	if mc.onSecondaryTap != nil {
		mc.onSecondaryTap(event.Position)
	}
}

func (mc *MediaCard) TouchDown(event *mobile.TouchEvent) {
	mc.longPressPos = event.Position

	if fyne.CurrentDevice().IsMobile() {
		mc.longPressTimer = time.AfterFunc(800*time.Millisecond, func() {
			if mc.onSecondaryTap != nil {
				fyne.Do(func() {
					mc.onSecondaryTap(mc.longPressPos)
				})
			}
		})
	}
}

func (mc *MediaCard) TouchUp(event *mobile.TouchEvent) {
	if mc.longPressTimer != nil {
		mc.longPressTimer.Stop()
		mc.longPressTimer = nil
	}
}

func (mc *MediaCard) TouchCancel(event *mobile.TouchEvent) {
	if mc.longPressTimer != nil {
		mc.longPressTimer.Stop()
		mc.longPressTimer = nil
	}
}

func (mc *MediaCard) MouseIn(event *desktop.MouseEvent) {
	mc.hovered = true
	mc.overlay.FillColor = color.NRGBA{R: 255, G: 255, B: 255, A: 20}
	mc.overlay.Show()
	mc.overlay.Refresh()
}

func (mc *MediaCard) MouseMoved(event *desktop.MouseEvent) {}

func (mc *MediaCard) MouseOut() {
	mc.hovered = false
	mc.overlay.Hide()
	mc.overlay.Refresh()
}

func (mc *MediaCard) SetTapCallback(callback func()) {
	mc.onTap = callback
}

func (mc *MediaCard) SetSecondaryTapCallback(callback func(fyne.Position)) {
	mc.onSecondaryTap = callback
}

func (r *mediaGridRenderer) Layout(size fyne.Size) {
	if r.container == nil {
		return
	}
	r.calculateColumns(size)
	r.container.Resize(size)
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

	// Show actual song count if available
	if len(album.Songs) > 0 {
		if len(album.Artists) > 0 {
			subtitle = fmt.Sprintf("%s • %d songs", subtitle, len(album.Songs))
		} else {
			subtitle = fmt.Sprintf("%d songs", len(album.Songs))
		}
	} else if len(album.Artists) == 0 {
		subtitle = "Album"
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
	var subtitle string

	songCount := len(author.Songs)
	albumCount := len(author.Albums)

	if songCount > 0 && albumCount > 0 {
		subtitle = fmt.Sprintf("%d songs • %d albums", songCount, albumCount)
	} else if songCount > 0 {
		subtitle = fmt.Sprintf("%d songs", songCount)
	} else if albumCount > 0 {
		subtitle = fmt.Sprintf("%d albums", albumCount)
	} else {
		subtitle = "Artist"
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

func (r *mediaGridRenderer) setColumns(cols int) {
	if r.grid == nil || cols == r.grid.columns {
		return
	}
	r.grid.columns = cols
	if r.container != nil {
		old := r.container.Objects
		r.container = container.NewGridWithColumns(cols)
		r.container.Objects = old
	}
}

func (r *mediaGridRenderer) calculateColumns(size fyne.Size) {
	if r.grid == nil || r.grid.itemSize.Width <= 0 || size.Width <= 0 {
		return
	}

	pad := theme.Padding()

	cell := r.grid.itemSize.Width + pad
	avail := size.Width - 2*pad
	if avail < r.grid.itemSize.Width {
		avail = r.grid.itemSize.Width
	}

	ideal := int((avail + pad) / cell)
	if ideal < 1 {
		ideal = 1
	}
	if ideal > 8 {
		ideal = 8
	}

	r.setColumns(ideal)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (mc *MediaCard) CreateRenderer() fyne.WidgetRenderer {
	return &mediaCardRenderer{card: mc}
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
	if r.card == nil {
		return fyne.NewSize(200, 280)
	}
	return r.card.size
}

func (r *mediaCardRenderer) Refresh() {
	if r.card == nil {
		return
	}

	imageContainer := container.NewStack(r.card.image, r.card.overlay)

	textHeight := float32(60)
	textContainer := container.NewVBox(r.card.title, r.card.subtitle)
	textContainer.Resize(fyne.NewSize(r.card.size.Width, textHeight))

	r.card.container = container.NewBorder(nil, textContainer, nil, nil, imageContainer)
}

func (r *mediaCardRenderer) Objects() []fyne.CanvasObject {
	if r.card == nil || r.card.container == nil {
		return []fyne.CanvasObject{}
	}
	return []fyne.CanvasObject{r.card.container}
}

func (r *mediaCardRenderer) Destroy() {}
