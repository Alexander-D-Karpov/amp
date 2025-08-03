package views

import (
	"context"
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
	artistsGrid *fyne.Container
	searchEntry *widget.Entry
	refreshBtn  *widget.Button
	sortSelect  *widget.Select
	loader      *widget.ProgressBarInfinite

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
		debug:           debug,
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

	av.artistsGrid = container.NewGridWithColumns(4)
	av.loader = widget.NewProgressBarInfinite()
	av.loader.Hide()
}

func (av *ArtistsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil, av.refreshBtn, av.searchEntry)
	controls := container.NewHBox(widget.NewLabel("Sort:"), av.sortSelect)
	header := container.NewVBox(searchBar, controls)
	content := container.NewScroll(av.artistsGrid)
	av.container = container.NewBorder(header, av.loader, nil, nil, content)
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
		av.refreshView()
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
			return
		}

		av.mu.Lock()
		av.artists = artists
		av.applySortAndFilter()
		av.mu.Unlock()

		fyne.Do(func() {
			av.refreshView()
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

func (av *ArtistsView) refreshView() {
	if av.artistsGrid == nil {
		return
	}

	av.artistsGrid.RemoveAll()

	av.mu.RLock()
	artists := make([]*types.Author, len(av.filteredArtists))
	copy(artists, av.filteredArtists)
	av.mu.RUnlock()

	if len(artists) == 0 {
		emptyLabel := widget.NewLabel("No artists found")
		emptyLabel.Alignment = fyne.TextAlignCenter
		av.artistsGrid.Add(emptyLabel)
	} else {
		for _, artist := range artists {
			if artist != nil {
				av.artistsGrid.Add(av.createArtistCard(artist))
			}
		}
	}

	av.artistsGrid.Refresh()
}

func (av *ArtistsView) createArtistCard(artist *types.Author) fyne.CanvasObject {
	avatar := widget.NewIcon(theme.AccountIcon())
	avatar.Resize(fyne.NewSize(120, 120))

	name := widget.NewLabel(artist.Name)
	name.Alignment = fyne.TextAlignCenter
	name.TextStyle = fyne.TextStyle{Bold: true}
	name.Wrapping = fyne.TextWrapWord
	if len(name.Text) > 30 {
		name.SetText(name.Text[:30] + "...")
	}

	if artist.ImageCropped != nil && *artist.ImageCropped != "" {
		go func(imageURL string, avatarWidget *widget.Icon) {
			imageRes := av.imageService.GetImage(imageURL)
			fyne.Do(func() {
				avatarWidget.SetResource(imageRes)
			})
		}(*artist.ImageCropped, avatar)
	}

	content := container.NewVBox(avatar, name)

	btn := widget.NewButton("", func() {
		av.handlers.HandleArtistSelection(artist)
	})

	return container.NewStack(content, btn)
}

func (av *ArtistsView) SetCompactMode(compact bool) {
	av.compactMode = compact
	cols := 4
	if compact {
		cols = 2
	}
	fyne.Do(func() {
		av.artistsGrid = container.NewGridWithColumns(cols)
		av.refreshView()
	})
}

func (av *ArtistsView) Refresh() {
	av.loadArtists()
}

func (av *ArtistsView) Container() *fyne.Container {
	return av.container
}
