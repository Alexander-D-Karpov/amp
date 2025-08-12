package views

import (
	"context"
	"fmt"
	"github.com/Alexander-D-Karpov/amp/internal/download"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/handlers"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/internal/ui/components"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type SongsView struct {
	musicService    *services.MusicService
	imageService    *services.ImageService
	downloadManager *download.Manager
	handlers        *handlers.UIHandlers

	container *fyne.Container

	mediaGrid   *components.MediaGrid
	songList    *components.SongList
	gridScroll  *container.Scroll
	listScroll  *container.Scroll
	centerStack *fyne.Container

	searchEntry   *widget.Entry
	refreshBtn    *widget.Button
	viewToggleBtn *widget.Button
	sortSelect    *widget.Select
	filterSelect  *widget.Select
	loader        *widget.ProgressBarInfinite
	statusLabel   *widget.Label

	contextMenu    *components.ContextMenu
	lastTappedSong *types.Song
	parentWindow   fyne.Window

	mu            sync.RWMutex
	songs         []*types.Song
	filteredSongs []*types.Song
	allSongs      []*types.Song
	isGridView    bool
	compactMode   bool
	searchTimer   *time.Timer
	currentPage   int
	hasMore       bool
	loading       bool
	loadingMore   bool
	lastSearch    string
	debug         bool
	searchCache   map[string][]*types.Song
	currentSort   api.SortOption

	onDownload       func(*types.Song)
	onAddPlaylist    func(*types.Song)
	openAlbumBySlug  func(string)
	openAuthorBySlug func(string)
	openSongBySlug   func(string)
}

func NewSongsView(musicService *services.MusicService, imageService *services.ImageService, handlers *handlers.UIHandlers) *SongsView {
	sv := &SongsView{
		musicService:  musicService,
		imageService:  imageService,
		handlers:      handlers,
		currentPage:   1,
		hasMore:       true,
		isGridView:    true,
		songs:         make([]*types.Song, 0),
		filteredSongs: make([]*types.Song, 0),
		allSongs:      make([]*types.Song, 0),
		debug:         true,
		searchCache:   make(map[string][]*types.Song),
		currentSort:   api.SortDefault,
	}

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadSongs()
	return sv
}

func (sv *SongsView) SetParentWindow(window fyne.Window) {
	sv.parentWindow = window
}

func (sv *SongsView) setupWidgets() {
	sv.searchEntry = widget.NewEntry()
	sv.searchEntry.SetPlaceHolder("Search songs...")
	sv.searchEntry.OnChanged = sv.onSearchChanged

	sv.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), sv.Refresh)
	sv.viewToggleBtn = widget.NewButtonWithIcon("", theme.GridIcon(), sv.toggleView)

	sv.sortSelect = widget.NewSelect([]string{
		"Date Added", "Name A-Z", "Name Z-A", "Artist A-Z", "Duration", "Most Played", "Most Liked", "Least Liked", "Longest", "Newest",
	}, sv.onSortChanged)
	sv.sortSelect.SetSelected("Date Added")

	sv.filterSelect = widget.NewSelect([]string{
		"All Songs", "Downloaded", "Liked",
	}, sv.onFilterChanged)
	sv.filterSelect.SetSelected("All Songs")

	sv.mediaGrid = components.NewMediaGrid(fyne.NewSize(200, 260), sv.imageService)
	sv.mediaGrid.SetItemTapCallback(sv.onGridItemTapped)
	sv.mediaGrid.SetItemSecondaryTapCallback(sv.onGridItemSecondaryTapped)

	sv.songList = components.NewSongList()
	sv.songList.OnPlay(func(s *types.Song, queue []*types.Song) {
		if sv.handlers != nil {
			sv.handlers.HandleSongSelection(s, queue)
			sv.recordSongPlay(s)
		}
	})
	sv.songList.OnDownload(func(s *types.Song) {
		if sv.onDownload != nil {
			sv.onDownload(s)
		}
	})
	sv.songList.OnOpenAlbum(func(slug string) {
		if sv.openAlbumBySlug != nil {
			sv.openAlbumBySlug(slug)
		}
	})
	sv.songList.OnOpenAuthor(func(slug string) {
		if sv.openAuthorBySlug != nil {
			sv.openAuthorBySlug(slug)
		}
	})
	sv.songList.OnOpenSong(func(slug string) {
		if sv.openSongBySlug != nil {
			sv.openSongBySlug(slug)
		}
	})

	sv.loader = widget.NewProgressBarInfinite()
	sv.loader.Hide()
	sv.statusLabel = widget.NewLabel("Loading songs...")
}

func (sv *SongsView) setupLayout() {
	searchBar := container.NewBorder(
		nil, nil, nil,
		container.NewHBox(sv.viewToggleBtn, sv.refreshBtn),
		sv.searchEntry,
	)
	controls := container.NewHBox(
		widget.NewLabel("Sort:"), sv.sortSelect,
		widget.NewLabel("Filter:"), sv.filterSelect,
	)
	header := container.NewVBox(searchBar, controls, sv.statusLabel)

	sv.gridScroll = container.NewScroll(sv.mediaGrid)
	sv.gridScroll.OnScrolled = sv.onScrolled

	sv.listScroll = container.NewScroll(sv.songList)
	sv.listScroll.OnScrolled = sv.onScrolled

	sv.centerStack = container.NewStack(sv.gridScroll, sv.listScroll)
	sv.listScroll.Hide()

	sv.container = container.NewBorder(header, sv.loader, nil, nil, sv.centerStack)
}

func (sv *SongsView) onScrolled(pos fyne.Position) {
	if sv.loadingMore || !sv.hasMore {
		return
	}
	var scrollSize fyne.Size
	if sv.isGridView {
		scrollSize = sv.gridScroll.Size()
	} else {
		scrollSize = sv.listScroll.Size()
	}
	contentSize := sv.mediaGrid.MinSize()

	if pos.Y >= contentSize.Height-scrollSize.Height-100 {
		if sv.debug {
			log.Printf("[SONGS_VIEW] Near bottom, loading more songs")
		}
		go sv.loadMoreSongs()
	}
}

func (sv *SongsView) onGridItemTapped(index int) {
	sv.mu.RLock()
	if index >= len(sv.filteredSongs) {
		sv.mu.RUnlock()
		return
	}
	song := sv.filteredSongs[index]
	sv.mu.RUnlock()

	sv.lastTappedSong = song

	if sv.debug {
		log.Printf("[SONGS_VIEW] Song selected: %s by %s", song.Name, getArtistNames(song.Authors))
	}

	if sv.handlers != nil {
		sv.handlers.HandleSongSelection(song, sv.filteredSongs)
		sv.recordSongPlay(song)
	}
}

func (sv *SongsView) recordSongPlay(song *types.Song) {
	if song == nil {
		return
	}

	go func() {
		ctx := context.Background()

		song.Played++
		if err := sv.musicService.GetStorage().SaveSong(ctx, song); err != nil {
			log.Printf("[SONGS_VIEW] Failed to update play count for song %s: %v", song.Name, err)
		}

		if err := sv.musicService.GetStorage().AddPlayHistory(ctx, song.Slug, nil); err != nil {
			log.Printf("[SONGS_VIEW] Failed to add play history for %s: %v", song.Slug, err)
		}

		if sv.debug {
			log.Printf("[SONGS_VIEW] Recorded play for song: %s (total plays: %d)", song.Name, song.Played)
		}
	}()
}

func (sv *SongsView) handlePlaySong(song *types.Song) {
	if sv.handlers != nil {
		sv.handlers.HandleSongSelection(song, sv.filteredSongs)
		sv.recordSongPlay(song)
	}
}

func (sv *SongsView) handleLikeSong(song *types.Song) {
	if song == nil {
		return
	}

	liked := song.Liked == nil || !*song.Liked
	song.Liked = &liked

	if sv.debug {
		log.Printf("[SONGS_VIEW] Toggled like for song: %s (liked: %v)", song.Name, liked)
	}

	go func() {
		ctx := context.Background()
		if err := sv.musicService.GetStorage().SaveSong(ctx, song); err != nil {
			log.Printf("[SONGS_VIEW] Failed to save like status: %v", err)
		}

		fyne.Do(func() {
			sv.updateGridView()
		})
	}()
}

func (sv *SongsView) handleDownloadSong(song *types.Song) {
	if song == nil {
		return
	}

	if sv.debug {
		log.Printf("[SONGS_VIEW] Download requested for song: %s", song.Name)
	}

	if sv.onDownload != nil {
		sv.onDownload(song)
		return
	}

	if sv.handlers != nil && sv.handlers.DownloadManager != nil {
		go func() {
			ctx := context.Background()
			if err := sv.handlers.DownloadManager.DownloadSong(ctx, song); err != nil {
				log.Printf("[SONGS_VIEW] Download failed for %s: %v", song.Name, err)
			} else {
				log.Printf("[SONGS_VIEW] Download started for %s", song.Name)
			}
		}()
	}
}

func (sv *SongsView) handleAddToPlaylist(song *types.Song) {
	if song == nil {
		return
	}

	if sv.debug {
		log.Printf("[SONGS_VIEW] Add to playlist requested for song: %s", song.Name)
	}

	if sv.onAddPlaylist != nil {
		sv.onAddPlaylist(song)
	} else {
		if sv.parentWindow != nil {
			log.Printf("[SONGS_VIEW] Playlist selection not implemented yet")
		}
	}
}

func (sv *SongsView) toggleView() {
	sv.isGridView = !sv.isGridView
	fyne.Do(func() {
		if sv.isGridView {
			sv.viewToggleBtn.SetIcon(theme.GridIcon())
			sv.listScroll.Hide()
			sv.gridScroll.Show()
		} else {
			sv.viewToggleBtn.SetIcon(theme.ListIcon())
			sv.gridScroll.Hide()
			sv.listScroll.Show()
		}
		sv.updateGridView()
	})
}

func (sv *SongsView) updateGridView() {
	sv.mu.RLock()
	songs := make([]*types.Song, len(sv.filteredSongs))
	copy(songs, sv.filteredSongs)
	sv.mu.RUnlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Updating view with %d songs (grid=%v)", len(songs), sv.isGridView)
	}

	fyne.Do(func() {
		if sv.statusLabel != nil {
			if len(songs) == 0 {
				sv.statusLabel.SetText("No songs found")
			} else {
				sv.statusLabel.SetText(fmt.Sprintf("Showing %d songs", len(songs)))
			}
		}
	})

	if sv.isGridView {
		items := make([]components.MediaItem, 0, len(songs))
		for _, song := range songs {
			if song != nil {
				items = append(items, components.MediaItemFromSong(song))
			}
		}
		sv.mediaGrid.SetItems(items)
		if len(songs) > 100 {
			sv.mediaGrid.SetVirtualScroll(true)
		}
	} else {
		sv.songList.SetSongs(songs)
	}
}

func (sv *SongsView) onSearchChanged(query string) {
	if sv.searchTimer != nil {
		sv.searchTimer.Stop()
	}
	sv.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		sv.performSearch(query)
	})
}

func (sv *SongsView) performSearch(query string) {
	sv.mu.Lock()
	sv.lastSearch = query
	sv.currentPage = 1
	sv.hasMore = true
	sv.songs = make([]*types.Song, 0)
	sv.allSongs = make([]*types.Song, 0)
	sv.mu.Unlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Performing search for: '%s'", query)
	}

	if query == "" {
		sv.loadSongs()
		return
	}

	cacheKey := fmt.Sprintf("%s_%s", query, sv.currentSort)
	if cached, exists := sv.searchCache[cacheKey]; exists {
		sv.mu.Lock()
		sv.songs = cached
		sv.allSongs = append([]*types.Song(nil), cached...)
		sv.filteredSongs = append([]*types.Song(nil), cached...)
		sv.mu.Unlock()

		sv.applySortAndFilter()
		fyne.Do(func() { sv.updateGridView() })
		return
	}

	sv.loadSongsWithSearch(query)
}

func (sv *SongsView) loadSongsWithSearch(query string) {
	sv.mu.Lock()
	if sv.loading {
		sv.mu.Unlock()
		return
	}
	sv.loading = true
	sv.mu.Unlock()

	fyne.Do(func() {
		if sv.loader != nil {
			sv.loader.Show()
		}
		if sv.statusLabel != nil {
			sv.statusLabel.SetText("Searching...")
		}
	})

	go func() {
		defer func() {
			sv.mu.Lock()
			sv.loading = false
			sv.mu.Unlock()
			fyne.Do(func() {
				if sv.loader != nil {
					sv.loader.Hide()
				}
			})
		}()

		ctx := context.Background()

		if sv.debug {
			log.Printf("[SONGS_VIEW] Loading songs with search - query: '%s', sort: '%s'", query, sv.currentSort)
		}

		songs, hasMore, err := sv.musicService.GetSongsWithSort(ctx, 1, query, sv.currentSort)
		if err != nil {
			if sv.debug {
				log.Printf("[SONGS_VIEW] Error searching songs: %v", err)
			}
			fyne.Do(func() {
				if sv.statusLabel != nil {
					sv.statusLabel.SetText(fmt.Sprintf("Search error: %v", err))
				}
			})
			return
		}

		if sv.debug {
			log.Printf("[SONGS_VIEW] Search returned %d songs", len(songs))
		}

		sv.mu.Lock()
		sv.songs = songs
		sv.allSongs = append([]*types.Song(nil), songs...)
		sv.filteredSongs = append([]*types.Song(nil), songs...)
		sv.hasMore = hasMore
		cacheKey := fmt.Sprintf("%s_%s", query, sv.currentSort)
		sv.searchCache[cacheKey] = songs
		sv.applySortAndFilter()
		sv.mu.Unlock()

		fyne.Do(func() { sv.updateGridView() })
	}()
}

func (sv *SongsView) onSortChanged(option string) {
	sv.mu.Lock()
	oldSort := sv.currentSort
	sv.currentSort = sv.mapSortOption(option)
	sv.mu.Unlock()

	if oldSort != sv.currentSort {
		sv.mu.Lock()
		sv.searchCache = make(map[string][]*types.Song)
		sv.currentPage = 1
		sv.hasMore = true
		sv.songs = nil
		sv.allSongs = nil
		sv.mu.Unlock()

		if sv.lastSearch != "" {
			sv.performSearch(sv.lastSearch)
		} else {
			sv.loadSongs()
		}
	}
}

func (sv *SongsView) mapSortOption(option string) api.SortOption {
	switch option {
	case "Most Played":
		return api.SortPlayed
	case "Most Liked":
		return api.SortLikes
	case "Least Liked":
		return api.SortLikesReversed
	case "Longest":
		return api.SortLength
	case "Newest":
		return api.SortUploaded
	default:
		return api.SortDefault
	}
}

func (sv *SongsView) onFilterChanged(filter string) {
	sv.applySortAndFilter()
	fyne.Do(func() { sv.updateGridView() })
}

func (sv *SongsView) loadSongs() {
	sv.mu.Lock()
	if sv.loading {
		sv.mu.Unlock()
		return
	}
	sv.loading = true
	query := sv.lastSearch
	page := sv.currentPage
	sortOption := sv.currentSort
	sv.mu.Unlock()

	fyne.Do(func() {
		if sv.loader != nil {
			sv.loader.Show()
		}
		if sv.statusLabel != nil {
			sv.statusLabel.SetText("Loading songs...")
		}
	})

	go func() {
		defer func() {
			sv.mu.Lock()
			sv.loading = false
			sv.mu.Unlock()
			fyne.Do(func() {
				if sv.loader != nil {
					sv.loader.Hide()
				}
			})
		}()

		ctx := context.Background()

		if sv.debug {
			log.Printf("[SONGS_VIEW] Loading songs - page: %d, query: '%s', sort: '%s'", page, query, sortOption)
		}

		songs, hasMore, err := sv.musicService.GetSongsWithSort(ctx, page, query, sortOption)
		if err != nil {
			if sv.debug {
				log.Printf("[SONGS_VIEW] Error loading songs: %v", err)
			}
			fyne.Do(func() {
				if sv.statusLabel != nil {
					sv.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
				}
			})
			return
		}

		if sv.debug {
			log.Printf("[SONGS_VIEW] Loaded %d songs from service", len(songs))
		}

		sv.mu.Lock()
		if page == 1 {
			sv.songs = songs
			sv.allSongs = append([]*types.Song(nil), songs...)
		} else {
			sv.songs = append(sv.songs, songs...)
			sv.allSongs = append(sv.allSongs, songs...)
		}
		sv.hasMore = hasMore
		sv.applySortAndFilter()
		sv.mu.Unlock()

		fyne.Do(func() { sv.updateGridView() })
	}()
}

func (sv *SongsView) loadMoreSongs() {
	sv.mu.Lock()
	if sv.loadingMore || !sv.hasMore || sv.loading {
		sv.mu.Unlock()
		return
	}
	sv.loadingMore = true
	sv.currentPage++
	page := sv.currentPage
	query := sv.lastSearch
	sortOption := sv.currentSort
	sv.mu.Unlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Loading more songs - page: %d", page)
	}

	go func() {
		defer func() {
			sv.mu.Lock()
			sv.loadingMore = false
			sv.mu.Unlock()
		}()

		ctx := context.Background()
		songs, hasMore, err := sv.musicService.GetSongsWithSort(ctx, page, query, sortOption)
		if err != nil {
			if sv.debug {
				log.Printf("[SONGS_VIEW] Error loading more songs: %v", err)
			}
			return
		}

		if sv.debug {
			log.Printf("[SONGS_VIEW] Loaded %d more songs", len(songs))
		}

		sv.mu.Lock()
		sv.songs = append(sv.songs, songs...)
		sv.allSongs = append(sv.allSongs, songs...)
		sv.hasMore = hasMore
		sv.applySortAndFilter()
		sv.mu.Unlock()

		fyne.Do(func() { sv.updateGridView() })
	}()
}

func (sv *SongsView) applySortAndFilter() {
	filtered := make([]*types.Song, 0, len(sv.songs))
	var filter string
	if sv.filterSelect != nil {
		filter = sv.filterSelect.Selected
	}

	for _, song := range sv.songs {
		if song == nil {
			continue
		}
		include := false
		switch filter {
		case "All Songs", "":
			include = true
		case "Downloaded":
			include = song.Downloaded
		case "Liked":
			include = song.Liked != nil && *song.Liked
		default:
			include = true
		}
		if include {
			filtered = append(filtered, song)
		}
	}

	var sortOpt string
	if sv.sortSelect != nil {
		sortOpt = sv.sortSelect.Selected
	}

	if sv.currentSort == api.SortDefault {
		sort.Slice(filtered, func(i, j int) bool {
			s1, s2 := filtered[i], filtered[j]
			if s1 == nil || s2 == nil {
				return false
			}
			switch sortOpt {
			case "Name A-Z":
				return strings.ToLower(s1.Name) < strings.ToLower(s2.Name)
			case "Name Z-A":
				return strings.ToLower(s1.Name) > strings.ToLower(s2.Name)
			case "Artist A-Z":
				return getFirstAuthor(s1) < getFirstAuthor(s2)
			case "Duration":
				return s1.Length > s2.Length
			case "Play Count":
				return s1.Played > s2.Played
			case "Date Added", "":
				return s1.CreatedAt.After(s2.CreatedAt)
			}
			return false
		})
	}

	sv.filteredSongs = filtered

	if sv.debug {
		log.Printf("[SONGS_VIEW] Applied filter '%s', result: %d songs", filter, len(sv.filteredSongs))
	}
}

func (sv *SongsView) SetCompactMode(compact bool) {
	sv.compactMode = compact
	fyne.Do(func() {
		sv.mediaGrid.SetCompactMode(compact)
		sv.updateGridView()
	})
}

func (sv *SongsView) Refresh() {
	if sv.debug {
		log.Printf("[SONGS_VIEW] Manual refresh requested")
	}
	sv.mu.Lock()
	sv.currentPage = 1
	sv.hasMore = true
	sv.songs = nil
	sv.allSongs = nil
	sv.filteredSongs = nil
	sv.searchCache = make(map[string][]*types.Song)
	sv.mu.Unlock()

	if sv.lastSearch != "" {
		sv.performSearch(sv.lastSearch)
	} else {
		sv.loadSongs()
	}
}

func (sv *SongsView) SetCallbacks(onDownload func(*types.Song), onAddPlaylist func(*types.Song)) {
	sv.onDownload = onDownload
	sv.onAddPlaylist = onAddPlaylist
}

func (sv *SongsView) SetOpenAlbumBySlug(cb func(string)) {
	sv.openAlbumBySlug = cb
	if sv.songList != nil {
		sv.songList.OnOpenAlbum(cb)
	}
}
func (sv *SongsView) SetOpenAuthorBySlug(cb func(string)) {
	sv.openAuthorBySlug = cb
	if sv.songList != nil {
		sv.songList.OnOpenAuthor(cb)
	}
}
func (sv *SongsView) SetOpenSongBySlug(cb func(string)) {
	sv.openSongBySlug = cb
	if sv.songList != nil {
		sv.songList.OnOpenSong(cb)
	}
}

func (sv *SongsView) Container() *fyne.Container {
	return sv.container
}

func (sv *SongsView) SetDownloadHandler(handler func(*types.Song)) {
	sv.onDownload = handler
}

func getArtistNames(authors []*types.Author) string {
	if len(authors) == 0 {
		return "Unknown Artist"
	}
	names := make([]string, 0, len(authors))
	for _, author := range authors {
		if author != nil && author.Name != "" {
			names = append(names, author.Name)
		}
	}
	if len(names) == 0 {
		return "Unknown Artist"
	}
	return strings.Join(names, ", ")
}

func getFirstAuthor(s *types.Song) string {
	if len(s.Authors) > 0 && s.Authors[0] != nil {
		return strings.ToLower(s.Authors[0].Name)
	}
	return ""
}

func (sv *SongsView) onGridItemSecondaryTapped(index int, pos fyne.Position) {
	sv.mu.RLock()
	if index >= len(sv.filteredSongs) {
		sv.mu.RUnlock()
		return
	}
	song := sv.filteredSongs[index]
	sv.mu.RUnlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Secondary tap on song: %s at position %v", song.Name, pos)
	}

	sv.showContextMenu(song, pos)
}

func (sv *SongsView) showContextMenu(song *types.Song, pos fyne.Position) {
	if song == nil || sv.parentWindow == nil {
		return
	}

	if sv.debug {
		log.Printf("[SONGS_VIEW] Showing context menu for song: %s at position %v", song.Name, pos)
	}

	if sv.contextMenu != nil {
		sv.contextMenu.Hide()
	}

	sv.contextMenu = components.NewContextMenu(song, sv.debug)

	sv.contextMenu.SetCallbacks(
		sv.handlePlaySong,
		sv.handleLikeSong,
		sv.handleDownloadSong,
		sv.handleAddToPlaylist,
	)

	windowSize := sv.parentWindow.Canvas().Size()
	if pos.X > windowSize.Width-200 {
		pos.X = windowSize.Width - 200
	}
	if pos.Y > windowSize.Height-150 {
		pos.Y = windowSize.Height - 150
	}
	if pos.X < 0 {
		pos.X = 10
	}
	if pos.Y < 0 {
		pos.Y = 10
	}

	sv.contextMenu.ShowAt(sv.parentWindow.Canvas(), pos)
}
