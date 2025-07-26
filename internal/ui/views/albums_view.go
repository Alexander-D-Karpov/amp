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

type AlbumsView struct {
	api     *api.Client
	storage *storage.Database
	search  *search.Engine

	container     *fyne.Container
	scroll        *container.Scroll
	searchEntry   *widget.Entry
	albumsGrid    *fyne.Container
	albumsList    *widget.List
	refreshBtn    *widget.Button
	viewToggleBtn *widget.Button
	sortSelect    *widget.Select

	albums         []*types.Album
	filteredAlbums []*types.Album
	isGridView     bool
	searchTimer    *time.Timer
	compactMode    bool
	initialized    bool

	currentPage int
	hasMore     bool
	loading     bool

	onAlbumSelected func(*types.Album)
}

func NewAlbumsView(api *api.Client, storage *storage.Database, search *search.Engine) *AlbumsView {
	av := &AlbumsView{
		api:         api,
		storage:     storage,
		search:      search,
		currentPage: 1,
		hasMore:     true,
		isGridView:  true,
	}

	av.setupWidgets()
	av.setupLayout()
	av.initialized = true
	av.loadAlbums()

	return av
}

func (av *AlbumsView) SetCompactMode(compact bool) {
	av.compactMode = compact

	if compact {
		av.albumsGrid = container.NewGridWithColumns(2)
	} else {
		av.albumsGrid = container.NewGridWithColumns(4)
	}

	if av.isGridView && av.initialized {
		av.showGridView()
	}
	if av.initialized {
		av.setupLayout()
	}
}

func (av *AlbumsView) setupWidgets() {
	av.searchEntry = widget.NewEntry()
	av.searchEntry.SetPlaceHolder("Search albums...")
	av.searchEntry.OnChanged = av.onSearchDebounced

	av.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), av.refreshData)
	av.viewToggleBtn = widget.NewButtonWithIcon("", theme.GridIcon(), av.toggleView)

	av.sortSelect = widget.NewSelect([]string{
		"Name A-Z", "Name Z-A", "Artist A-Z", "Release Date", "Songs Count",
	}, av.onSortChanged)

	av.albumsGrid = container.NewGridWithColumns(4)

	av.albumsList = widget.NewList(
		func() int {
			return len(av.filteredAlbums)
		},
		func() fyne.CanvasObject {
			return av.createAlbumListItem()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			av.updateAlbumListItem(id, obj)
		},
	)
	av.albumsList.OnSelected = av.onAlbumListSelected
}

func (av *AlbumsView) createAlbumListItem() fyne.CanvasObject {
	cover := widget.NewIcon(theme.FolderIcon())
	cover.Resize(fyne.NewSize(64, 64))

	nameLabel := widget.NewLabel("Album Name")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	artistLabel := widget.NewLabel("Artist")
	yearLabel := widget.NewLabel("2023")
	songsLabel := widget.NewLabel("12 songs")

	albumInfo := container.NewVBox(
		nameLabel,
		artistLabel,
		container.NewHBox(yearLabel, widget.NewLabel("•"), songsLabel),
	)

	return container.NewBorder(
		nil, nil,
		cover,
		nil,
		albumInfo,
	)
}

func (av *AlbumsView) updateAlbumListItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(av.filteredAlbums) {
		return
	}

	album := av.filteredAlbums[id]
	c := obj.(*fyne.Container)

	var albumInfo *fyne.Container

	for _, o := range c.Objects {
		if cont, ok := o.(*fyne.Container); ok {
			albumInfo = cont
		}
	}

	if albumInfo != nil && len(albumInfo.Objects) >= 3 {
		nameLabel := albumInfo.Objects[0].(*widget.Label)
		artistLabel := albumInfo.Objects[1].(*widget.Label)
		detailsContainer := albumInfo.Objects[2].(*fyne.Container)

		nameLabel.SetText(album.Name)
		if len(album.Artists) > 0 {
			artistLabel.SetText(album.Artists[0].Name)
		} else {
			artistLabel.SetText("Unknown Artist")
		}

		if len(detailsContainer.Objects) >= 3 {
			yearLabel := detailsContainer.Objects[0].(*widget.Label)
			songsLabel := detailsContainer.Objects[2].(*widget.Label)

			yearText := "Unknown"
			if album.Meta != nil && album.Meta.Release != nil {
				yearText = album.Meta.Release.Format("2006")
			}
			yearLabel.SetText(yearText)
			songsLabel.SetText(fmt.Sprintf("%d songs", len(album.Songs)))
		}
	}
}

func (av *AlbumsView) onAlbumListSelected(id widget.ListItemID) {
	if id < len(av.filteredAlbums) && av.onAlbumSelected != nil {
		av.onAlbumSelected(av.filteredAlbums[id])
	}
}

func (av *AlbumsView) setupLayout() {
	searchContainer := container.NewBorder(
		nil, nil,
		nil,
		container.NewHBox(av.viewToggleBtn, av.refreshBtn),
		av.searchEntry,
	)

	controlsContainer := container.NewHBox(
		widget.NewLabel("Sort:"),
		av.sortSelect,
	)

	header := container.NewVBox(
		searchContainer,
		controlsContainer,
	)

	av.scroll = container.NewScroll(av.albumsGrid)

	av.container = container.NewBorder(
		header,
		nil,
		nil,
		nil,
		av.scroll,
	)

	// Set default selection after layout is complete
	if av.sortSelect.Selected == "" {
		av.sortSelect.SetSelected("Name A-Z")
	}
}

func (av *AlbumsView) toggleView() {
	av.isGridView = !av.isGridView
	if av.isGridView {
		av.viewToggleBtn.SetIcon(theme.GridIcon())
		av.showGridView()
	} else {
		av.viewToggleBtn.SetIcon(theme.ListIcon())
		av.showListView()
	}
}

func (av *AlbumsView) showGridView() {
	if av.albumsGrid == nil || av.scroll == nil {
		return
	}

	av.albumsGrid.RemoveAll()
	for _, album := range av.filteredAlbums {
		card := av.createAlbumCard(album)
		av.albumsGrid.Add(card)
	}

	if av.hasMore && !av.loading {
		loadMoreBtn := widget.NewButton("Load More", func() {
			av.loadMoreAlbums()
		})
		av.albumsGrid.Add(loadMoreBtn)
	}

	av.scroll.Content = av.albumsGrid
	av.scroll.Refresh()
}

func (av *AlbumsView) showListView() {
	if av.albumsList == nil || av.scroll == nil {
		return
	}

	av.scroll.Content = av.albumsList
	av.scroll.Refresh()
	av.albumsList.Refresh()
}

func (av *AlbumsView) createAlbumCard(album *types.Album) fyne.CanvasObject {
	cover := widget.NewIcon(theme.FolderIcon())

	cardSize := fyne.NewSize(150, 150)
	if av.compactMode {
		cardSize = fyne.NewSize(120, 120)
	}
	cover.Resize(cardSize)

	titleLabel := widget.NewLabel(album.Name)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignCenter
	titleLabel.Wrapping = fyne.TextWrapWord

	var artistText string
	if len(album.Artists) > 0 {
		artistText = album.Artists[0].Name
	} else {
		artistText = "Unknown Artist"
	}

	artistLabel := widget.NewLabel(artistText)
	artistLabel.Alignment = fyne.TextAlignCenter

	songCountLabel := widget.NewLabel(fmt.Sprintf("%d songs", len(album.Songs)))
	songCountLabel.Alignment = fyne.TextAlignCenter
	songCountLabel.TextStyle = fyne.TextStyle{Italic: true}

	cardContent := container.NewVBox(
		cover,
		titleLabel,
		artistLabel,
		songCountLabel,
	)

	cardBtn := widget.NewButton("", func() {
		if av.onAlbumSelected != nil {
			av.onAlbumSelected(album)
		}
	})
	cardBtn.Importance = widget.LowImportance

	card := widget.NewCard("", "", cardContent)

	return container.NewStack(card, cardBtn)
}

func (av *AlbumsView) loadAlbums() {
	av.currentPage = 1
	av.hasMore = true
	go func() {
		ctx := context.Background()
		albums, err := av.storage.GetAlbums(ctx, 1000, 0)
		if err != nil {
			log.Printf("Failed to load albums from storage: %v", err)
			av.loadFromAPI()
			return
		}

		av.albums = albums
		av.filteredAlbums = albums
		av.refreshView()
	}()
}

func (av *AlbumsView) loadFromAPI() {
	if av.loading {
		return
	}

	av.loading = true
	go func() {
		defer func() { av.loading = false }()

		ctx := context.Background()
		resp, err := av.api.GetAlbums(ctx, av.currentPage, "")
		if err != nil {
			log.Printf("Failed to load albums from API: %v", err)
			return
		}

		if av.currentPage == 1 {
			av.albums = resp.Results
		} else {
			av.albums = append(av.albums, resp.Results...)
		}

		av.hasMore = resp.Next != nil
		av.filteredAlbums = av.albums
		av.refreshView()
	}()
}

func (av *AlbumsView) loadMoreAlbums() {
	if av.loading || !av.hasMore {
		return
	}

	av.currentPage++
	av.loadFromAPI()
}

func (av *AlbumsView) refreshData() {
	av.currentPage = 1
	av.hasMore = true
	av.albums = nil
	av.filteredAlbums = nil
	av.loadFromAPI()
}

func (av *AlbumsView) onSearchDebounced(query string) {
	if av.searchTimer != nil {
		av.searchTimer.Stop()
	}

	av.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		av.onSearch(query)
	})
}

func (av *AlbumsView) onSearch(query string) {
	if query == "" {
		av.filteredAlbums = av.albums
	} else {
		ctx := context.Background()
		results, err := av.search.FuzzySearch(ctx, query, 100)
		if err != nil {
			log.Printf("Search error: %v", err)
			return
		}
		av.filteredAlbums = results.Albums
	}
	av.refreshView()
}

func (av *AlbumsView) onSortChanged(option string) {
	if !av.initialized {
		return
	}

	switch option {
	case "Name A-Z":
		sort.Slice(av.filteredAlbums, func(i, j int) bool {
			return strings.ToLower(av.filteredAlbums[i].Name) < strings.ToLower(av.filteredAlbums[j].Name)
		})
	case "Name Z-A":
		sort.Slice(av.filteredAlbums, func(i, j int) bool {
			return strings.ToLower(av.filteredAlbums[i].Name) > strings.ToLower(av.filteredAlbums[j].Name)
		})
	case "Artist A-Z":
		sort.Slice(av.filteredAlbums, func(i, j int) bool {
			artistI := "Unknown"
			if len(av.filteredAlbums[i].Artists) > 0 {
				artistI = av.filteredAlbums[i].Artists[0].Name
			}
			artistJ := "Unknown"
			if len(av.filteredAlbums[j].Artists) > 0 {
				artistJ = av.filteredAlbums[j].Artists[0].Name
			}
			return strings.ToLower(artistI) < strings.ToLower(artistJ)
		})
	case "Songs Count":
		sort.Slice(av.filteredAlbums, func(i, j int) bool {
			return len(av.filteredAlbums[i].Songs) > len(av.filteredAlbums[j].Songs)
		})
	case "Release Date":
		sort.Slice(av.filteredAlbums, func(i, j int) bool {
			timeI := time.Time{}
			if av.filteredAlbums[i].Meta != nil && av.filteredAlbums[i].Meta.Release != nil {
				timeI = *av.filteredAlbums[i].Meta.Release
			}
			timeJ := time.Time{}
			if av.filteredAlbums[j].Meta != nil && av.filteredAlbums[j].Meta.Release != nil {
				timeJ = *av.filteredAlbums[j].Meta.Release
			}
			return timeI.After(timeJ)
		})
	}
	av.refreshView()
}

func (av *AlbumsView) refreshView() {
	if !av.initialized {
		return
	}

	if av.isGridView {
		av.showGridView()
	} else {
		av.showListView()
	}
}

func (av *AlbumsView) OnAlbumSelected(callback func(*types.Album)) {
	av.onAlbumSelected = callback
}

func (av *AlbumsView) Refresh() {
	av.refreshData()
}

func (av *AlbumsView) Container() *fyne.Container {
	return av.container
}
