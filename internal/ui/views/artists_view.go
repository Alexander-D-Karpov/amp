package views

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/search"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type ArtistsView struct {
	api     *api.Client
	storage *storage.Database
	search  *search.Engine

	container     *fyne.Container
	scroll        *container.Scroll
	searchEntry   *widget.Entry
	artistsGrid   *fyne.Container
	artistsList   *widget.List
	refreshBtn    *widget.Button
	viewToggleBtn *widget.Button
	sortSelect    *widget.Select
	genreFilter   *widget.Select

	artists         []*types.Author
	filteredArtists []*types.Author
	isGridView      bool
	searchTimer     *time.Timer
	compactMode     bool

	currentPage int
	hasMore     bool
	loading     bool

	onArtistSelected func(*types.Author)
}

func NewArtistsView(api *api.Client, storage *storage.Database, search *search.Engine) *ArtistsView {
	av := &ArtistsView{
		api:         api,
		storage:     storage,
		search:      search,
		currentPage: 1,
		hasMore:     true,
	}

	av.setupWidgets()
	av.setupLayout()
	av.loadArtists()

	return av
}

func (av *ArtistsView) SetCompactMode(compact bool) {
	av.compactMode = compact

	if compact {
		av.artistsGrid = container.NewGridWithColumns(2)
	} else {
		av.artistsGrid = container.NewGridWithColumns(4)
	}

	if av.isGridView {
		av.showGridView()
	}
	av.setupLayout()
}

func (av *ArtistsView) setupWidgets() {
	av.searchEntry = widget.NewEntry()
	av.searchEntry.SetPlaceHolder("Search artists...")
	av.searchEntry.OnChanged = av.onSearchDebounced

	av.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), av.refreshData)
	av.viewToggleBtn = widget.NewButtonWithIcon("", theme.GridIcon(), av.toggleView)

	av.sortSelect = widget.NewSelect([]string{
		"Name A-Z", "Name Z-A", "Songs Count", "Albums Count", "Most Played",
	}, av.onSortChanged)
	av.sortSelect.SetSelected("Name A-Z")

	av.genreFilter = widget.NewSelect([]string{
		"All Genres", "Pop", "Rock", "Hip-Hop", "Electronic", "Classical", "Jazz",
	}, av.onFilterChanged)
	av.genreFilter.SetSelected("All Genres")

	av.artistsGrid = container.NewGridWithColumns(4)
	av.isGridView = true

	av.artistsList = widget.NewList(
		func() int {
			return len(av.filteredArtists)
		},
		func() fyne.CanvasObject {
			return av.createArtistListItem()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			av.updateArtistListItem(id, obj)
		},
	)
	av.artistsList.OnSelected = av.onArtistListSelected
}

func (av *ArtistsView) createArtistListItem() fyne.CanvasObject {
	avatar := widget.NewIcon(theme.AccountIcon())
	avatar.Resize(fyne.NewSize(64, 64))

	nameLabel := widget.NewLabel("Artist Name")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	songsLabel := widget.NewLabel("0 songs")
	albumsLabel := widget.NewLabel("0 albums")

	statsLabel := widget.NewLabel("0 plays")
	statsLabel.TextStyle = fyne.TextStyle{Italic: true}

	artistInfo := container.NewVBox(
		nameLabel,
		container.NewHBox(songsLabel, widget.NewLabel("•"), albumsLabel),
		statsLabel,
	)

	playBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
	})
	playBtn.Importance = widget.LowImportance

	followBtn := widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
	})
	followBtn.Importance = widget.LowImportance

	actions := container.NewHBox(playBtn, followBtn)

	return container.NewBorder(
		nil, nil,
		avatar,
		actions,
		artistInfo,
	)
}

func (av *ArtistsView) updateArtistListItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(av.filteredArtists) {
		return
	}

	artist := av.filteredArtists[id]
	c := obj.(*fyne.Container)

	var artistInfo *fyne.Container

	for _, o := range c.Objects {
		if cont, ok := o.(*fyne.Container); ok {
			if len(cont.Objects) >= 3 {
				artistInfo = cont
			}
		}
	}

	if artistInfo != nil && len(artistInfo.Objects) >= 3 {
		nameLabel := artistInfo.Objects[0].(*widget.Label)
		statsContainer := artistInfo.Objects[1].(*fyne.Container)
		playsLabel := artistInfo.Objects[2].(*widget.Label)

		nameLabel.SetText(artist.Name)
		playsLabel.SetText("0 plays")

		if len(statsContainer.Objects) >= 3 {
			songsLabel := statsContainer.Objects[0].(*widget.Label)
			albumsLabel := statsContainer.Objects[2].(*widget.Label)

			songsLabel.SetText(fmt.Sprintf("%d songs", len(artist.Songs)))
			albumsLabel.SetText(fmt.Sprintf("%d albums", len(artist.Albums)))
		}
	}
}

func (av *ArtistsView) onArtistListSelected(id widget.ListItemID) {
	if id < len(av.filteredArtists) && av.onArtistSelected != nil {
		av.onArtistSelected(av.filteredArtists[id])
	}
}

func (av *ArtistsView) setupLayout() {
	searchContainer := container.NewBorder(nil, nil, nil, container.NewHBox(av.viewToggleBtn, av.refreshBtn), av.searchEntry)
	controlsContainer := container.NewHBox(widget.NewLabel("Sort:"), av.sortSelect, widget.NewLabel("Genre:"), av.genreFilter)
	header := container.NewVBox(searchContainer, controlsContainer)
	av.scroll = container.NewScroll(av.artistsGrid)
	av.container = container.NewBorder(header, nil, nil, nil, av.scroll)
}

func (av *ArtistsView) toggleView() {
	av.isGridView = !av.isGridView
	if av.isGridView {
		av.viewToggleBtn.SetIcon(theme.GridIcon())
		av.showGridView()
	} else {
		av.viewToggleBtn.SetIcon(theme.ListIcon())
		av.showListView()
	}
}

func (av *ArtistsView) showGridView() {
	av.artistsGrid.RemoveAll()
	for _, artist := range av.filteredArtists {
		av.artistsGrid.Add(av.createArtistCard(artist))
	}

	if av.hasMore && !av.loading {
		loadMoreBtn := widget.NewButton("Load More", func() {
			av.loadMoreArtists()
		})
		av.artistsGrid.Add(loadMoreBtn)
	}

	av.scroll.Content = av.artistsGrid
	av.scroll.Refresh()
}

func (av *ArtistsView) showListView() {
	av.scroll.Content = av.artistsList
	av.scroll.Refresh()
	av.artistsList.Refresh()
}

func (av *ArtistsView) createArtistCard(artist *types.Author) fyne.CanvasObject {
	avatar := widget.NewIcon(theme.AccountIcon())

	cardSize := fyne.NewSize(120, 120)
	if av.compactMode {
		cardSize = fyne.NewSize(80, 80)
	}
	avatar.Resize(cardSize)

	nameLabel := widget.NewLabel(artist.Name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Alignment = fyne.TextAlignCenter
	nameLabel.Wrapping = fyne.TextWrapWord

	songsCount := len(artist.Songs)
	albumsCount := len(artist.Albums)

	statsLabel := widget.NewLabel(fmt.Sprintf("%d songs • %d albums", songsCount, albumsCount))
	statsLabel.Alignment = fyne.TextAlignCenter
	statsLabel.TextStyle = fyne.TextStyle{Italic: true}

	playBtn := widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
	})
	playBtn.Importance = widget.LowImportance

	cardContent := container.NewVBox(
		avatar,
		nameLabel,
		statsLabel,
		playBtn,
	)

	cardBtn := widget.NewButton("", func() {
		if av.onArtistSelected != nil {
			av.onArtistSelected(artist)
		}
	})
	cardBtn.Importance = widget.LowImportance

	card := widget.NewCard("", "", cardContent)

	return container.NewStack(card, cardBtn)
}

func (av *ArtistsView) loadArtists() {
	av.currentPage = 1
	av.hasMore = true
	go func() {
		ctx := context.Background()
		artists, err := av.storage.GetAuthors(ctx, 1000, 0)
		if err != nil {
			log.Printf("Failed to load artists from storage: %v", err)
			av.loadFromAPI()
			return
		}

		av.artists = artists
		av.filteredArtists = artists
		av.refreshView()
	}()
}

func (av *ArtistsView) loadFromAPI() {
	if av.loading {
		return
	}

	av.loading = true
	go func() {
		defer func() { av.loading = false }()

		ctx := context.Background()
		resp, err := av.api.GetAuthors(ctx, av.currentPage, "")
		if err != nil {
			log.Printf("Failed to load artists from API: %v", err)
			return
		}

		if av.currentPage == 1 {
			av.artists = resp.Results
		} else {
			av.artists = append(av.artists, resp.Results...)
		}

		av.hasMore = resp.Next != nil
		av.filteredArtists = av.artists
		av.refreshView()
	}()
}

func (av *ArtistsView) loadMoreArtists() {
	if av.loading || !av.hasMore {
		return
	}

	av.currentPage++
	av.loadFromAPI()
}

func (av *ArtistsView) refreshData() {
	av.currentPage = 1
	av.hasMore = true
	av.artists = nil
	av.filteredArtists = nil
	av.loadFromAPI()
}

func (av *ArtistsView) onSearchDebounced(query string) {
	if av.searchTimer != nil {
		av.searchTimer.Stop()
	}

	av.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		av.onSearch(query)
	})
}

func (av *ArtistsView) onSearch(query string) {
	if query == "" {
		av.filteredArtists = av.artists
	} else {
		ctx := context.Background()
		results, err := av.search.FuzzySearch(ctx, query, 100)
		if err != nil {
			log.Printf("Search error: %v", err)
			return
		}
		av.filteredArtists = results.Authors
	}
	av.refreshView()
}

func (av *ArtistsView) onSortChanged(option string) {
	if av.scroll == nil {
		return
	}
	switch option {
	case "Name A-Z":
		sort.Slice(av.filteredArtists, func(i, j int) bool {
			return strings.ToLower(av.filteredArtists[i].Name) < strings.ToLower(av.filteredArtists[j].Name)
		})
	case "Name Z-A":
		sort.Slice(av.filteredArtists, func(i, j int) bool {
			return strings.ToLower(av.filteredArtists[i].Name) > strings.ToLower(av.filteredArtists[j].Name)
		})
	case "Songs Count":
		sort.Slice(av.filteredArtists, func(i, j int) bool { return len(av.filteredArtists[i].Songs) > len(av.filteredArtists[j].Songs) })
	case "Albums Count":
		sort.Slice(av.filteredArtists, func(i, j int) bool { return len(av.filteredArtists[i].Albums) > len(av.filteredArtists[j].Albums) })
	case "Most Played":
		sort.Slice(av.filteredArtists, func(i, j int) bool { return len(av.filteredArtists[i].Songs) > len(av.filteredArtists[j].Songs) })
	}
	av.refreshView()
}

func (av *ArtistsView) onFilterChanged(genre string) {
	av.refreshView()
}

func (av *ArtistsView) refreshView() {
	if av.scroll == nil {
		return
	}
	if av.isGridView {
		av.showGridView()
	} else {
		av.artistsList.Refresh()
	}
}

func (av *ArtistsView) OnArtistSelected(callback func(*types.Author)) {
	av.onArtistSelected = callback
}

func (av *ArtistsView) Refresh() {
	av.refreshData()
}

func (av *ArtistsView) Container() *fyne.Container {
	return av.container
}
