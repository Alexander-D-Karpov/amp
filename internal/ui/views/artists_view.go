package views

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/handlers"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/internal/ui/components"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type ArtistsView struct {
	musicService *services.MusicService
	imageService *services.ImageService
	handlers     *handlers.UIHandlers
	debug        bool

	container   *fyne.Container
	mediaGrid   *components.MediaGrid
	searchEntry *widget.Entry
	refreshBtn  *widget.Button
	sortSelect  *widget.Select
	loader      *widget.ProgressBarInfinite
	statusLabel *widget.Label

	contextMenu      *widget.PopUpMenu
	parentWindow     fyne.Window
	lastTappedArtist *types.Author

	mu              sync.RWMutex
	artists         []*types.Author
	filteredArtists []*types.Author
	searchTimer     *time.Timer
	compactMode     bool
	loading         bool
	searchCache     map[string][]*types.Author
	currentPage     int
	hasMore         bool
	lastSearch      string

	onDownload    func(*types.Author)
	onAddPlaylist func(*types.Author)
}

func NewArtistsView(musicService *services.MusicService, imageService *services.ImageService, handlers *handlers.UIHandlers, debug bool) *ArtistsView {
	av := &ArtistsView{
		musicService:    musicService,
		imageService:    imageService,
		handlers:        handlers,
		debug:           debug,
		artists:         make([]*types.Author, 0),
		filteredArtists: make([]*types.Author, 0),
		searchCache:     make(map[string][]*types.Author),
		currentPage:     1,
		hasMore:         true,
	}
	av.setupWidgets()
	av.setupLayout()
	av.loadArtists()
	return av
}

func (av *ArtistsView) SetParentWindow(w fyne.Window) { av.parentWindow = w }

func (av *ArtistsView) setupWidgets() {
	av.searchEntry = widget.NewEntry()
	av.searchEntry.SetPlaceHolder("Search artists…")
	av.searchEntry.OnChanged = av.onSearchChanged
	av.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), av.Refresh)
	av.sortSelect = widget.NewSelect([]string{"Name A-Z", "Name Z-A"}, av.onSortChanged)
	av.sortSelect.SetSelected("Name A-Z")
	av.mediaGrid = components.NewMediaGrid(fyne.NewSize(200, 260), av.imageService)
	av.mediaGrid.SetItemTapCallback(av.onGridItemTapped)
	av.mediaGrid.SetItemSecondaryTapCallback(av.onGridItemSecondaryTapped)
	av.loader = widget.NewProgressBarInfinite()
	av.loader.Hide()
	av.statusLabel = widget.NewLabel("Loading artists…")
}

func (av *ArtistsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil, av.refreshBtn, av.searchEntry)
	controls := container.NewHBox(widget.NewLabel("Sort:"), av.sortSelect)
	header := container.NewVBox(searchBar, controls, av.statusLabel)
	content := container.NewScroll(container.NewStack(av.mediaGrid))
	av.container = container.NewBorder(header, av.loader, nil, nil, content)
}

func (av *ArtistsView) onGridItemTapped(index int) {
	av.mu.RLock()
	defer av.mu.RUnlock()
	if index < len(av.filteredArtists) && av.handlers != nil {
		artist := av.filteredArtists[index]
		av.lastTappedArtist = artist
		av.handlers.HandleArtistSelection(artist) // Start playback or navigate per your handler
	}
}

func (av *ArtistsView) onGridItemSecondaryTapped(index int, pos fyne.Position) {
	av.mu.RLock()
	if index >= len(av.filteredArtists) {
		av.mu.RUnlock()
		return
	}
	artist := av.filteredArtists[index]
	av.mu.RUnlock()
	av.showContextMenu(artist, pos)
}

func (av *ArtistsView) showContextMenu(artist *types.Author, pos fyne.Position) {
	if artist == nil || av.parentWindow == nil {
		return
	}
	if av.contextMenu != nil {
		av.contextMenu.Hide()
	}

	playItem := fyne.NewMenuItem("Play Artist", func() {
		if av.handlers != nil {
			av.handlers.HandleArtistSelection(artist)
		}
	})
	playItem.Icon = theme.MediaPlayIcon()
	dl := fyne.NewMenuItem("Download Artist Songs", func() {
		if av.onDownload != nil {
			av.onDownload(artist)
		}
	})
	dl.Icon = theme.DownloadIcon()
	pl := fyne.NewMenuItem("Add to Playlist…", func() {
		if av.onAddPlaylist != nil {
			av.onAddPlaylist(artist)
		}
	})
	pl.Icon = theme.ContentAddIcon()
	av.contextMenu = widget.NewPopUpMenu(fyne.NewMenu("", playItem, fyne.NewMenuItemSeparator(), dl, pl), av.parentWindow.Canvas())
	av.contextMenu.ShowAtPosition(pos)
}

func (av *ArtistsView) onSearchChanged(q string) {
	if av.searchTimer != nil {
		av.searchTimer.Stop()
	}
	av.searchTimer = time.AfterFunc(300*time.Millisecond, func() { av.performSearch(q) })
}

func (av *ArtistsView) performSearch(q string) {
	av.mu.Lock()
	av.lastSearch = q
	av.currentPage = 1
	av.hasMore = true
	av.mu.Unlock()
	if q == "" {
		av.loadArtists()
		return
	}
	if cached, ok := av.searchCache[q]; ok {
		av.mu.Lock()
		av.artists = cached
		av.filteredArtists = append([]*types.Author(nil), cached...)
		av.mu.Unlock()
		av.applySortAndFilter()
		fyne.Do(func() { av.updateGridView() })
		return
	}
	av.loadArtistsWithSearch(q)
}

func (av *ArtistsView) loadArtistsWithSearch(q string) {
	av.mu.Lock()
	if av.loading {
		av.mu.Unlock()
		return
	}
	av.loading = true
	av.mu.Unlock()
	fyne.Do(func() { av.loader.Show(); av.statusLabel.SetText("Searching artists…") })
	go func() {
		defer func() { av.mu.Lock(); av.loading = false; av.mu.Unlock(); fyne.Do(func() { av.loader.Hide() }) }()
		ctx := context.Background()
		authors, hasMore, err := av.musicService.GetAuthors(ctx, 1, q)
		if err != nil {
			fyne.Do(func() { av.statusLabel.SetText(fmt.Sprintf("Search error: %v", err)) })
			return
		}
		av.mu.Lock()
		av.artists = authors
		av.hasMore = hasMore
		av.searchCache[q] = authors
		av.applySortAndFilter()
		av.mu.Unlock()
		fyne.Do(func() { av.updateGridView() })
	}()
}

func (av *ArtistsView) onSortChanged(_ string) {
	av.applySortAndFilter()
	fyne.Do(func() { av.updateGridView() })
}

func (av *ArtistsView) loadArtists() {
	av.mu.Lock()
	if av.loading {
		av.mu.Unlock()
		return
	}
	av.loading = true
	q := av.lastSearch
	av.mu.Unlock()
	fyne.Do(func() { av.loader.Show(); av.statusLabel.SetText("Loading artists…") })
	go func() {
		defer func() { av.mu.Lock(); av.loading = false; av.mu.Unlock(); fyne.Do(func() { av.loader.Hide() }) }()
		ctx := context.Background()
		artists, hasMore, err := av.musicService.GetAuthors(ctx, 1, q)
		if err != nil {
			fyne.Do(func() { av.statusLabel.SetText(fmt.Sprintf("Error: %v", err)) })
			return
		}
		av.mu.Lock()
		av.artists = artists
		av.hasMore = hasMore
		av.applySortAndFilter()
		av.mu.Unlock()
		fyne.Do(func() { av.updateGridView() })
	}()
}

func (av *ArtistsView) applySortAndFilter() {
	av.filteredArtists = append([]*types.Author(nil), av.artists...)
	if av.sortSelect == nil {
		return
	}
	var opt string
	fyne.Do(func() {
		if av.sortSelect != nil {
			opt = av.sortSelect.Selected
		}
	})
	sort.SliceStable(av.filteredArtists, func(i, j int) bool {
		a1, a2 := av.filteredArtists[i], av.filteredArtists[j]
		if a1 == nil || a2 == nil {
			return false
		}
		switch opt {
		case "Name A-Z":
			return strings.ToLower(a1.Name) < strings.ToLower(a2.Name)
		case "Name Z-A":
			return strings.ToLower(a1.Name) > strings.ToLower(a2.Name)
		}
		return false
	})
}

func (av *ArtistsView) updateGridView() {
	if av.mediaGrid == nil {
		return
	}
	av.mu.RLock()
	artists := append([]*types.Author(nil), av.filteredArtists...)
	av.mu.RUnlock()
	if len(artists) == 0 {
		av.statusLabel.SetText("No artists found")
		av.mediaGrid.SetItems([]components.MediaItem{})
		av.mediaGrid.Refresh()
		return
	}
	av.statusLabel.SetText(fmt.Sprintf("Showing %d artists", len(artists)))
	items := make([]components.MediaItem, 0, len(artists))
	for _, a := range artists {
		if a != nil {
			items = append(items, components.MediaItemFromAuthor(a))
		}
	}
	av.mediaGrid.SetItems(items)
	av.mediaGrid.Refresh()
	if len(artists) > 100 {
		av.mediaGrid.SetVirtualScroll(true)
	}
}

func (av *ArtistsView) SetCompactMode(compact bool) {
	av.compactMode = compact
	fyne.Do(func() { av.mediaGrid.SetCompactMode(compact); av.updateGridView() })
}

func (av *ArtistsView) Refresh() {
	av.mu.Lock()
	av.currentPage, av.hasMore = 1, true
	av.artists = nil
	av.filteredArtists = nil
	av.searchCache = make(map[string][]*types.Author)
	av.mu.Unlock()
	if av.lastSearch != "" {
		av.performSearch(av.lastSearch)
	} else {
		av.loadArtists()
	}
}

func (av *ArtistsView) SetCallbacks(onDownload func(*types.Author), onAddPlaylist func(*types.Author)) {
	av.onDownload, av.onAddPlaylist = onDownload, onAddPlaylist
}

func (av *ArtistsView) Container() *fyne.Container { return av.container }
