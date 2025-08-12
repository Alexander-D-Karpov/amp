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

type AlbumsView struct {
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

	contextMenu     *widget.PopUpMenu
	parentWindow    fyne.Window
	lastTappedAlbum *types.Album

	mu             sync.RWMutex
	albums         []*types.Album
	filteredAlbums []*types.Album
	searchTimer    *time.Timer
	compactMode    bool
	loading        bool
	searchCache    map[string][]*types.Album
	currentPage    int
	hasMore        bool
	lastSearch     string

	onDownload    func(*types.Album)
	onAddPlaylist func(*types.Album)
}

func NewAlbumsView(musicService *services.MusicService, imageService *services.ImageService, handlers *handlers.UIHandlers, debug bool) *AlbumsView {
	av := &AlbumsView{
		musicService:   musicService,
		imageService:   imageService,
		handlers:       handlers,
		debug:          debug,
		albums:         make([]*types.Album, 0),
		filteredAlbums: make([]*types.Album, 0),
		searchCache:    make(map[string][]*types.Album),
		currentPage:    1,
		hasMore:        true,
	}
	av.setupWidgets()
	av.setupLayout()
	av.loadAlbums()
	return av
}

func (av *AlbumsView) SetParentWindow(w fyne.Window) { av.parentWindow = w }

func (av *AlbumsView) setupWidgets() {
	av.searchEntry = widget.NewEntry()
	av.searchEntry.SetPlaceHolder("Search albums…")
	av.searchEntry.OnChanged = av.onSearchChanged

	av.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), av.Refresh)

	av.sortSelect = widget.NewSelect([]string{"Name A-Z", "Name Z-A", "Artist A-Z", "Release Year"}, av.onSortChanged)
	av.sortSelect.SetSelected("Name A-Z")

	av.mediaGrid = components.NewMediaGrid(fyne.NewSize(200, 280), av.imageService)
	av.mediaGrid.SetItemTapCallback(av.onGridItemTapped)
	av.mediaGrid.SetItemSecondaryTapCallback(av.onGridItemSecondaryTapped)

	av.loader = widget.NewProgressBarInfinite()
	av.loader.Hide()
	av.statusLabel = widget.NewLabel("Loading albums…")
}

func (av *AlbumsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil, av.refreshBtn, av.searchEntry)
	controls := container.NewHBox(widget.NewLabel("Sort:"), av.sortSelect)
	header := container.NewVBox(searchBar, controls, av.statusLabel)

	scroll := container.NewScroll(container.NewStack(av.mediaGrid))

	av.container = container.NewBorder(header, av.loader, nil, nil, scroll)
}

func (av *AlbumsView) onGridItemTapped(index int) {
	av.mu.RLock()
	defer av.mu.RUnlock()
	if index < len(av.filteredAlbums) && av.handlers != nil {
		album := av.filteredAlbums[index]
		av.lastTappedAlbum = album
		av.handlers.HandleAlbumSelection(album) // Starts playback of album tracks as per your handler
	}
}

func (av *AlbumsView) onGridItemSecondaryTapped(index int, pos fyne.Position) {
	av.mu.RLock()
	if index >= len(av.filteredAlbums) {
		av.mu.RUnlock()
		return
	}
	album := av.filteredAlbums[index]
	av.mu.RUnlock()
	av.showContextMenu(album, pos)
}

func (av *AlbumsView) showContextMenu(album *types.Album, pos fyne.Position) {
	if album == nil || av.parentWindow == nil {
		return
	}
	if av.contextMenu != nil {
		av.contextMenu.Hide()
	}

	playItem := fyne.NewMenuItem("Play Album", func() {
		if av.handlers != nil {
			av.handlers.HandleAlbumSelection(album)
		}
	})
	playItem.Icon = theme.MediaPlayIcon()

	downloadItem := fyne.NewMenuItem("Download Album", func() {
		if av.onDownload != nil {
			av.onDownload(album)
		}
	})
	downloadItem.Icon = theme.DownloadIcon()

	playlistItem := fyne.NewMenuItem("Add to Playlist…", func() {
		if av.onAddPlaylist != nil {
			av.onAddPlaylist(album)
		}
	})
	playlistItem.Icon = theme.ContentAddIcon()

	av.contextMenu = widget.NewPopUpMenu(fyne.NewMenu("", playItem, fyne.NewMenuItemSeparator(), downloadItem, playlistItem), av.parentWindow.Canvas())
	av.contextMenu.ShowAtPosition(pos)
}

func (av *AlbumsView) onSearchChanged(q string) {
	if av.searchTimer != nil {
		av.searchTimer.Stop()
	}
	av.searchTimer = time.AfterFunc(300*time.Millisecond, func() { av.performSearch(q) })
}

func (av *AlbumsView) performSearch(q string) {
	av.mu.Lock()
	av.lastSearch = q
	av.currentPage = 1
	av.hasMore = true
	av.mu.Unlock()
	if q == "" {
		av.loadAlbums()
		return
	}
	if cached, ok := av.searchCache[q]; ok {
		av.mu.Lock()
		av.albums = cached
		av.filteredAlbums = append([]*types.Album(nil), cached...)
		av.mu.Unlock()
		av.applySortAndFilter()
		fyne.Do(func() { av.updateGridView() })
		return
	}
	av.loadAlbumsWithSearch(q)
}

func (av *AlbumsView) loadAlbumsWithSearch(q string) {
	av.mu.Lock()
	if av.loading {
		av.mu.Unlock()
		return
	}
	av.loading = true
	av.mu.Unlock()
	fyne.Do(func() { av.loader.Show(); av.statusLabel.SetText("Searching albums…") })
	go func() {
		defer func() { av.mu.Lock(); av.loading = false; av.mu.Unlock(); fyne.Do(func() { av.loader.Hide() }) }()
		ctx := context.Background()
		albums, hasMore, err := av.musicService.GetAlbums(ctx, 1, q)
		if err != nil {
			fyne.Do(func() { av.statusLabel.SetText(fmt.Sprintf("Search error: %v", err)) })
			return
		}
		av.mu.Lock()
		av.albums = albums
		av.hasMore = hasMore
		av.searchCache[q] = albums
		av.applySortAndFilter()
		av.mu.Unlock()
		fyne.Do(func() { av.updateGridView() })
	}()
}

func (av *AlbumsView) onSortChanged(_ string) {
	av.applySortAndFilter()
	fyne.Do(func() { av.updateGridView() })
}

func (av *AlbumsView) loadAlbums() {
	av.mu.Lock()
	if av.loading {
		av.mu.Unlock()
		return
	}
	av.loading = true
	q := av.lastSearch
	av.mu.Unlock()
	fyne.Do(func() { av.loader.Show(); av.statusLabel.SetText("Loading albums…") })
	go func() {
		defer func() { av.mu.Lock(); av.loading = false; av.mu.Unlock(); fyne.Do(func() { av.loader.Hide() }) }()
		ctx := context.Background()
		albums, hasMore, err := av.musicService.GetAlbums(ctx, 1, q)
		if err != nil {
			fyne.Do(func() { av.statusLabel.SetText(fmt.Sprintf("Error: %v", err)) })
			return
		}
		av.mu.Lock()
		av.albums = albums
		av.hasMore = hasMore
		av.applySortAndFilter()
		av.mu.Unlock()
		fyne.Do(func() { av.updateGridView() })
	}()
}

func (av *AlbumsView) applySortAndFilter() {
	av.filteredAlbums = append([]*types.Album(nil), av.albums...)
	if av.sortSelect == nil {
		return
	}
	var opt string
	fyne.Do(func() {
		if av.sortSelect != nil {
			opt = av.sortSelect.Selected
		}
	})
	sort.SliceStable(av.filteredAlbums, func(i, j int) bool {
		a1, a2 := av.filteredAlbums[i], av.filteredAlbums[j]
		if a1 == nil || a2 == nil {
			return false
		}
		switch opt {
		case "Name A-Z":
			return strings.ToLower(a1.Name) < strings.ToLower(a2.Name)
		case "Name Z-A":
			return strings.ToLower(a1.Name) > strings.ToLower(a2.Name)
		case "Artist A-Z":
			return firstAlbumArtist(a1) < firstAlbumArtist(a2)
		case "Release Year":
			return a1.CreatedAt.After(a2.CreatedAt)
		}
		return false
	})
}

func firstAlbumArtist(a *types.Album) string {
	if a == nil || len(a.Artists) == 0 || a.Artists[0] == nil {
		return ""
	}
	return strings.ToLower(a.Artists[0].Name)
}

func (av *AlbumsView) updateGridView() {
	if av.mediaGrid == nil {
		return
	}
	av.mu.RLock()
	albums := append([]*types.Album(nil), av.filteredAlbums...)
	av.mu.RUnlock()
	if len(albums) == 0 {
		av.statusLabel.SetText("No albums found")
		av.mediaGrid.SetItems([]components.MediaItem{})
		av.mediaGrid.Refresh()
		return
	}
	av.statusLabel.SetText(fmt.Sprintf("Showing %d albums", len(albums)))
	items := make([]components.MediaItem, 0, len(albums))
	for _, al := range albums {
		if al != nil {
			items = append(items, components.MediaItemFromAlbum(al))
		}
	}
	av.mediaGrid.SetItems(items)
	av.mediaGrid.Refresh()
	if len(albums) > 100 {
		av.mediaGrid.SetVirtualScroll(true)
	}
}

func (av *AlbumsView) SetCompactMode(compact bool) {
	av.compactMode = compact
	fyne.Do(func() { av.mediaGrid.SetCompactMode(compact); av.updateGridView() })
}

func (av *AlbumsView) Refresh() {
	av.mu.Lock()
	av.currentPage, av.hasMore = 1, true
	av.albums = nil
	av.filteredAlbums = nil
	av.searchCache = make(map[string][]*types.Album)
	av.mu.Unlock()
	if av.lastSearch != "" {
		av.performSearch(av.lastSearch)
	} else {
		av.loadAlbums()
	}
}

func (av *AlbumsView) SetCallbacks(onDownload func(*types.Album), onAddPlaylist func(*types.Album)) {
	av.onDownload, av.onAddPlaylist = onDownload, onAddPlaylist
}

func (av *AlbumsView) Container() *fyne.Container { return av.container }
