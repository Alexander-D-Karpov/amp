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

const (
	sortNameAZ = "Name A-Z"
	sortNameZA = "Name Z-A"
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

	mu              sync.RWMutex
	artists         []*types.Author
	filteredArtists []*types.Author
	searchTimer     *time.Timer
	compactMode     bool
	loading         bool
}

func NewArtistsView(musicService *services.MusicService, imageService *services.ImageService, handlers *handlers.UIHandlers, debug bool) *ArtistsView {
	av := &ArtistsView{
		musicService:    musicService,
		imageService:    imageService,
		handlers:        handlers,
		debug:           false, // Reduced debug logging
		artists:         make([]*types.Author, 0),
		filteredArtists: make([]*types.Author, 0),
	}

	av.setupWidgets()
	av.setupLayout()
	av.loadArtists()
	return av
}

func (av *ArtistsView) setupWidgets() {
	av.searchEntry = widget.NewEntry()
	av.searchEntry.SetPlaceHolder("Search artists...")
	av.searchEntry.OnChanged = av.onSearchChanged

	av.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), av.Refresh)

	av.sortSelect = widget.NewSelect([]string{
		sortNameAZ, sortNameZA,
	}, av.onSortChanged)
	av.sortSelect.SetSelected(sortNameAZ)

	av.mediaGrid = components.NewMediaGrid(fyne.NewSize(160, 200), av.imageService)
	av.mediaGrid.SetItemTapCallback(av.onGridItemTapped)

	av.loader = widget.NewProgressBarInfinite()
	av.loader.Hide()

	av.statusLabel = widget.NewLabel("Loading artists...")
}

func (av *ArtistsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil, av.refreshBtn, av.searchEntry)
	controls := container.NewHBox(widget.NewLabel("Sort:"), av.sortSelect)
	header := container.NewVBox(searchBar, controls, av.statusLabel)
	content := container.NewScroll(av.mediaGrid)
	av.container = container.NewBorder(header, av.loader, nil, nil, content)
}

func (av *ArtistsView) onGridItemTapped(index int) {
	av.mu.RLock()
	defer av.mu.RUnlock()

	if index < len(av.filteredArtists) && av.handlers != nil {
		artist := av.filteredArtists[index]
		if av.debug {
			fmt.Printf("[ARTIST_VIEW] Artist selected: %s\n", artist.Name)
		}
		av.handlers.HandleArtistSelection(artist)
	}
}

func (av *ArtistsView) onSearchChanged(query string) {
	if av.searchTimer != nil {
		av.searchTimer.Stop()
	}
	av.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		av.loadArtists()
	})
}

func (av *ArtistsView) onSortChanged(option string) {
	av.applySortAndFilter()
	fyne.Do(func() {
		av.updateGridView()
	})
}

func (av *ArtistsView) loadArtists() {
	av.mu.Lock()
	if av.loading {
		av.mu.Unlock()
		return
	}
	av.loading = true
	av.mu.Unlock()

	fyne.Do(func() {
		av.loader.Show()
		av.statusLabel.SetText("Loading artists...")
	})

	go func() {
		defer func() {
			av.mu.Lock()
			av.loading = false
			av.mu.Unlock()
			fyne.Do(func() {
				av.loader.Hide()
			})
		}()

		ctx := context.Background()
		query := ""
		fyne.Do(func() {
			if av.searchEntry != nil {
				query = av.searchEntry.Text
			}
		})

		artists, _, err := av.musicService.GetAuthors(ctx, 1, query)
		if err != nil {
			fyne.Do(func() {
				av.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		av.mu.Lock()
		av.artists = artists
		av.applySortAndFilter()
		av.mu.Unlock()

		fyne.Do(func() {
			av.updateGridView()
		})
	}()
}

func (av *ArtistsView) applySortAndFilter() {
	av.filteredArtists = av.artists

	if av.sortSelect == nil {
		return
	}

	var sortOpt string
	fyne.Do(func() {
		sortOpt = av.sortSelect.Selected
	})

	sort.Slice(av.filteredArtists, func(i, j int) bool {
		a1, a2 := av.filteredArtists[i], av.filteredArtists[j]
		if a1 == nil || a2 == nil {
			return false
		}
		switch sortOpt {
		case sortNameAZ:
			return strings.ToLower(a1.Name) < strings.ToLower(a2.Name)
		case sortNameZA:
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
	artists := make([]*types.Author, len(av.filteredArtists))
	copy(artists, av.filteredArtists)
	av.mu.RUnlock()

	var items []components.MediaItem
	if len(artists) == 0 {
		fyne.Do(func() {
			av.statusLabel.SetText("No artists found")
		})
	} else {
		fyne.Do(func() {
			av.statusLabel.SetText(fmt.Sprintf("Showing %d artists", len(artists)))
		})
		items = make([]components.MediaItem, len(artists))
		for i, artist := range artists {
			if artist != nil {
				items[i] = components.MediaItemFromAuthor(artist)
			}
		}
	}

	av.mediaGrid.SetItems(items)
	if len(artists) > 100 {
		av.mediaGrid.SetVirtualScroll(true)
	}
}

func (av *ArtistsView) SetCompactMode(compact bool) {
	av.compactMode = compact
	fyne.Do(func() {
		av.mediaGrid.SetCompactMode(compact)
		av.updateGridView()
	})
}

func (av *ArtistsView) Refresh() {
	av.mu.Lock()
	av.artists = make([]*types.Author, 0)
	av.filteredArtists = make([]*types.Author, 0)
	av.mu.Unlock()
	av.loadArtists()
}

func (av *ArtistsView) Container() *fyne.Container {
	return av.container
}
