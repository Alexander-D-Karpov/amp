package views

import (
	"context"
	"fmt"
	"log"
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

type SongsView struct {
	musicService *services.MusicService
	imageService *services.ImageService
	handlers     *handlers.UIHandlers

	container       *fyne.Container
	mediaGrid       *components.MediaGrid
	searchEntry     *widget.Entry
	refreshBtn      *widget.Button
	viewToggleBtn   *widget.Button
	sortSelect      *widget.Select
	filterSelect    *widget.Select
	loader          *widget.ProgressBarInfinite
	statusLabel     *widget.Label
	scrollContainer *container.Scroll

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
	}

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadSongs()
	return sv
}

func (sv *SongsView) setupWidgets() {
	sv.searchEntry = widget.NewEntry()
	sv.searchEntry.SetPlaceHolder("Search songs...")
	sv.searchEntry.OnChanged = sv.onSearchChanged

	sv.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), sv.Refresh)
	sv.viewToggleBtn = widget.NewButtonWithIcon("", theme.GridIcon(), sv.toggleView)

	sv.sortSelect = widget.NewSelect([]string{
		"Date Added", "Name A-Z", "Name Z-A", "Artist A-Z", "Duration", "Play Count",
	}, sv.onSortChanged)
	sv.sortSelect.SetSelected("Date Added")

	sv.filterSelect = widget.NewSelect([]string{
		"All Songs", "Downloaded", "Liked",
	}, sv.onFilterChanged)
	sv.filterSelect.SetSelected("All Songs")

	sv.mediaGrid = components.NewMediaGrid(fyne.NewSize(180, 220), sv.imageService)
	sv.mediaGrid.SetItemTapCallback(sv.onGridItemTapped)

	sv.loader = widget.NewProgressBarInfinite()
	sv.loader.Hide()

	sv.statusLabel = widget.NewLabel("Loading songs...")
}

func (sv *SongsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(sv.viewToggleBtn, sv.refreshBtn),
		sv.searchEntry)

	controls := container.NewHBox(
		widget.NewLabel("Sort:"), sv.sortSelect,
		widget.NewLabel("Filter:"), sv.filterSelect)

	header := container.NewVBox(searchBar, controls, sv.statusLabel)

	sv.scrollContainer = container.NewScroll(sv.mediaGrid)
	sv.scrollContainer.OnScrolled = sv.onScrolled

	sv.container = container.NewBorder(header, sv.loader, nil, nil, sv.scrollContainer)
}

func (sv *SongsView) onScrolled(pos fyne.Position) {
	if sv.loadingMore || !sv.hasMore {
		return
	}

	scrollSize := sv.scrollContainer.Size()
	contentSize := sv.mediaGrid.MinSize()

	if pos.Y >= contentSize.Height-scrollSize.Height-100 {
		if sv.debug {
			log.Printf("[SONGS_VIEW] Near bottom, loading more songs")
		}
		sv.loadMoreSongs()
	}
}

func (sv *SongsView) onGridItemTapped(index int) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	if index < len(sv.filteredSongs) && sv.handlers != nil {
		song := sv.filteredSongs[index]
		if sv.debug {
			log.Printf("[SONGS_VIEW] Song selected: %s by %s", song.Name, getArtistNames(song.Authors))
		}
		sv.handlers.HandleSongSelection(song, sv.filteredSongs)
	}
}

func (sv *SongsView) toggleView() {
	sv.isGridView = !sv.isGridView
	fyne.Do(func() {
		if sv.isGridView {
			sv.viewToggleBtn.SetIcon(theme.GridIcon())
		} else {
			sv.viewToggleBtn.SetIcon(theme.ListIcon())
		}
		sv.updateGridView()
	})
}

func (sv *SongsView) updateGridView() {
	if sv.mediaGrid == nil {
		return
	}

	sv.mu.RLock()
	songs := make([]*types.Song, len(sv.filteredSongs))
	copy(songs, sv.filteredSongs)
	sv.mu.RUnlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Updating grid with %d songs", len(songs))
	}

	var items []components.MediaItem
	if len(songs) == 0 {
		fyne.Do(func() {
			sv.statusLabel.SetText("No songs found")
		})
	} else {
		fyne.Do(func() {
			sv.statusLabel.SetText(fmt.Sprintf("Showing %d songs", len(songs)))
		})
		items = make([]components.MediaItem, len(songs))
		for i, song := range songs {
			if song != nil {
				items[i] = components.MediaItemFromSong(song)
				if sv.debug && i < 5 {
					log.Printf("[SONGS_VIEW] Added song to grid: %s", song.Name)
				}
			}
		}
	}

	sv.mediaGrid.SetItems(items)
	if len(songs) > 100 {
		sv.mediaGrid.SetVirtualScroll(true)
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
	sv.mu.Unlock()

	if sv.debug {
		log.Printf("[SONGS_VIEW] Performing search for: '%s'", query)
	}

	if query == "" {
		sv.mu.Lock()
		sv.filteredSongs = make([]*types.Song, len(sv.allSongs))
		copy(sv.filteredSongs, sv.allSongs)
		sv.mu.Unlock()

		sv.applySortAndFilter()
		fyne.Do(func() {
			sv.updateGridView()
		})
		return
	}

	if cached, exists := sv.searchCache[query]; exists {
		sv.mu.Lock()
		sv.songs = cached
		sv.filteredSongs = make([]*types.Song, len(cached))
		copy(sv.filteredSongs, cached)
		sv.mu.Unlock()

		sv.applySortAndFilter()
		fyne.Do(func() {
			sv.updateGridView()
		})
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
			log.Printf("[SONGS_VIEW] Loading songs with search - query: '%s'", query)
		}

		songs, hasMore, err := sv.musicService.GetSongs(ctx, 1, query)
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
		sv.filteredSongs = make([]*types.Song, len(songs))
		copy(sv.filteredSongs, songs)
		sv.hasMore = hasMore
		sv.searchCache[query] = songs
		sv.applySortAndFilter()
		sv.mu.Unlock()

		fyne.Do(func() {
			sv.updateGridView()
		})
	}()
}

func (sv *SongsView) onSortChanged(option string) {
	sv.applySortAndFilter()
	fyne.Do(func() {
		sv.updateGridView()
	})
}

func (sv *SongsView) onFilterChanged(filter string) {
	sv.applySortAndFilter()
	fyne.Do(func() {
		sv.updateGridView()
	})
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
			log.Printf("[SONGS_VIEW] Loading songs - page: %d, query: '%s'", page, query)
		}

		songs, hasMore, err := sv.musicService.GetSongs(ctx, page, query)
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
			sv.allSongs = make([]*types.Song, len(songs))
			copy(sv.allSongs, songs)
		} else {
			sv.songs = append(sv.songs, songs...)
			sv.allSongs = append(sv.allSongs, songs...)
		}
		sv.hasMore = hasMore
		sv.applySortAndFilter()
		sv.mu.Unlock()

		fyne.Do(func() {
			sv.updateGridView()
		})
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
		songs, hasMore, err := sv.musicService.GetSongs(ctx, page, query)
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

		fyne.Do(func() {
			sv.updateGridView()
		})
	}()
}

func (sv *SongsView) applySortAndFilter() {
	if sv.sortSelect == nil || sv.filterSelect == nil {
		sv.filteredSongs = sv.songs
		return
	}

	filtered := make([]*types.Song, 0)
	var filter string
	fyne.Do(func() {
		if sv.filterSelect != nil {
			filter = sv.filterSelect.Selected
		}
	})

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
	fyne.Do(func() {
		if sv.sortSelect != nil {
			sortOpt = sv.sortSelect.Selected
		}
	})

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

	sv.filteredSongs = filtered

	if sv.debug {
		log.Printf("[SONGS_VIEW] Applied filter '%s' and sort '%s', result: %d songs", filter, sortOpt, len(sv.filteredSongs))
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
	sv.songs = make([]*types.Song, 0)
	sv.allSongs = make([]*types.Song, 0)
	sv.filteredSongs = make([]*types.Song, 0)
	sv.searchCache = make(map[string][]*types.Song)
	sv.mu.Unlock()

	if sv.lastSearch != "" {
		sv.performSearch(sv.lastSearch)
	} else {
		sv.loadSongs()
	}
}

func (sv *SongsView) Container() *fyne.Container {
	return sv.container
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
