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

type SongsView struct {
	api     *api.Client
	storage *storage.Database
	search  *search.Engine

	container     *fyne.Container
	scroll        *container.Scroll
	searchEntry   *widget.Entry
	songsGrid     *fyne.Container
	songsList     *widget.List
	refreshBtn    *widget.Button
	viewToggleBtn *widget.Button
	sortSelect    *widget.Select
	filterSelect  *widget.Select

	songs          []*types.Song
	filteredSongs  []*types.Song
	onSongSelected func(*types.Song)
	isGridView     bool
	searchTimer    *time.Timer
	compactMode    bool

	currentPage int
	hasMore     bool
	loading     bool
}

type SortOption string

const (
	SortByName     SortOption = "Name"
	SortByArtist   SortOption = "Artist"
	SortByAlbum    SortOption = "Album"
	SortByDuration SortOption = "Duration"
	SortByPlayed   SortOption = "Play Count"
)

func NewSongsView(api *api.Client, storage *storage.Database, search *search.Engine) *SongsView {
	sv := &SongsView{
		api:         api,
		storage:     storage,
		search:      search,
		currentPage: 1,
		hasMore:     true,
	}

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadSongs()

	return sv
}

func (sv *SongsView) setupWidgets() {
	sv.searchEntry = widget.NewEntry()
	sv.searchEntry.SetPlaceHolder("Search songs...")
	sv.searchEntry.OnChanged = sv.onSearchDebounced

	sv.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), sv.refreshData)
	sv.viewToggleBtn = widget.NewButtonWithIcon("", theme.ListIcon(), sv.toggleView)

	sv.sortSelect = widget.NewSelect([]string{
		"Name A-Z", "Name Z-A", "Artist A-Z", "Album A-Z", "Duration", "Play Count",
	}, sv.onSortChanged)

	sv.filterSelect = widget.NewSelect([]string{
		"All Songs", "Downloaded", "Liked", "Recently Played",
	}, sv.onFilterChanged)

	sv.songsList = widget.NewList(
		func() int {
			return len(sv.filteredSongs)
		},
		func() fyne.CanvasObject {
			return sv.createSongListItem()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			sv.updateSongListItem(id, obj)
		},
	)
	sv.songsList.OnSelected = sv.onSongListSelected

	sv.songsGrid = container.NewGridWithColumns(4)
	sv.isGridView = false

	sv.sortSelect.SetSelected("Name A-Z")
	sv.filterSelect.SetSelected("All Songs")
}

func (sv *SongsView) SetCompactMode(compact bool) {
	sv.compactMode = compact

	if compact {
		sv.songsGrid = container.NewGridWithColumns(2)
	} else {
		sv.songsGrid = container.NewGridWithColumns(4)
	}

	if sv.isGridView {
		sv.showGridView()
	}
	sv.setupLayout()
}

func (sv *SongsView) createSongListItem() fyne.CanvasObject {
	cover := widget.NewIcon(theme.MediaMusicIcon())
	cover.Resize(fyne.NewSize(48, 48))

	titleLabel := widget.NewLabel("Song Title")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	artistLabel := widget.NewLabel("Artist")
	albumLabel := widget.NewLabel("Album")

	durationLabel := widget.NewLabel("3:45")
	statusIcon := widget.NewIcon(theme.DownloadIcon())
	statusIcon.Resize(fyne.NewSize(16, 16))
	statusIcon.Hide()

	likeBtn := widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), func() {
	})
	likeBtn.Importance = widget.LowImportance

	playBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
	})
	playBtn.Importance = widget.LowImportance

	songInfo := container.NewVBox(
		titleLabel,
		container.NewHBox(artistLabel, widget.NewLabel("•"), albumLabel),
	)

	rightSection := container.NewHBox(
		durationLabel,
		statusIcon,
		likeBtn,
		playBtn,
	)

	return container.NewBorder(
		nil, nil,
		cover,
		rightSection,
		songInfo,
	)
}

func (sv *SongsView) updateSongListItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(sv.filteredSongs) {
		return
	}

	song := sv.filteredSongs[id]
	c := obj.(*fyne.Container)

	var songInfo *fyne.Container
	var rightSection *fyne.Container

	for _, o := range c.Objects {
		if cont, ok := o.(*fyne.Container); ok {
			if len(cont.Objects) == 2 {
				songInfo = cont
			} else {
				rightSection = cont
			}
		}
	}

	if songInfo != nil && len(songInfo.Objects) >= 2 {
		titleLabel := songInfo.Objects[0].(*widget.Label)
		detailsContainer := songInfo.Objects[1].(*fyne.Container)

		titleLabel.SetText(song.Name)

		if len(detailsContainer.Objects) >= 3 {
			artistLabel := detailsContainer.Objects[0].(*widget.Label)
			albumLabel := detailsContainer.Objects[2].(*widget.Label)

			if len(song.Authors) > 0 {
				artistLabel.SetText(song.Authors[0].Name)
			} else {
				artistLabel.SetText("Unknown Artist")
			}

			if song.Album != nil {
				albumLabel.SetText(song.Album.Name)
			} else {
				albumLabel.SetText("Unknown Album")
			}
		}
	}

	if rightSection != nil && len(rightSection.Objects) >= 4 {
		durationLabel := rightSection.Objects[0].(*widget.Label)
		statusIcon := rightSection.Objects[1].(*widget.Icon)
		likeBtn := rightSection.Objects[2].(*widget.Button)

		duration := time.Duration(song.Length) * time.Second
		durationLabel.SetText(formatDuration(duration))

		if song.Downloaded {
			statusIcon.SetResource(theme.ConfirmIcon())
			statusIcon.Show()
		} else {
			statusIcon.Hide()
		}

		if song.Liked != nil && *song.Liked {
			likeBtn.SetIcon(theme.ConfirmIcon())
		} else {
			likeBtn.SetIcon(theme.VisibilityOffIcon())
		}
	}
}

func (sv *SongsView) onSongListSelected(id widget.ListItemID) {
	if id < len(sv.filteredSongs) && sv.onSongSelected != nil {
		sv.onSongSelected(sv.filteredSongs[id])
	}
}

func (sv *SongsView) createSongCard(song *types.Song) fyne.CanvasObject {
	cover := widget.NewIcon(theme.MediaMusicIcon())

	cardSize := fyne.NewSize(150, 150)
	if sv.compactMode {
		cardSize = fyne.NewSize(120, 120)
	}
	cover.Resize(cardSize)

	if song.ImageCropped != nil && *song.ImageCropped != "" {
		cover.SetResource(theme.MediaMusicIcon())
	}

	titleLabel := widget.NewLabel(song.Name)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignCenter
	titleLabel.Wrapping = fyne.TextWrapWord

	var artistText string
	if len(song.Authors) > 0 {
		artistText = song.Authors[0].Name
	} else {
		artistText = "Unknown Artist"
	}

	artistLabel := widget.NewLabel(artistText)
	artistLabel.Alignment = fyne.TextAlignCenter

	duration := time.Duration(song.Length) * time.Second
	durationLabel := widget.NewLabel(formatDuration(duration))
	durationLabel.Alignment = fyne.TextAlignCenter
	durationLabel.TextStyle = fyne.TextStyle{Italic: true}

	cardContent := container.NewVBox(
		cover,
		titleLabel,
		artistLabel,
		durationLabel,
	)

	cardBtn := widget.NewButton("", func() {
		if sv.onSongSelected != nil {
			sv.onSongSelected(song)
		}
	})
	cardBtn.Importance = widget.LowImportance

	card := widget.NewCard("", "", cardContent)

	return container.NewStack(card, cardBtn)
}

func (sv *SongsView) setupLayout() {
	searchContainer := container.NewBorder(nil, nil, nil, container.NewHBox(sv.viewToggleBtn, sv.refreshBtn), sv.searchEntry)
	controlsContainer := container.NewHBox(widget.NewLabel("Sort:"), sv.sortSelect, widget.NewLabel("Filter:"), sv.filterSelect)
	header := container.NewVBox(searchContainer, controlsContainer)

	sv.scroll = container.NewScroll(sv.songsList)
	sv.container = container.NewBorder(header, nil, nil, nil, sv.scroll)
}

func (sv *SongsView) toggleView() {
	sv.isGridView = !sv.isGridView
	if sv.isGridView {
		sv.viewToggleBtn.SetIcon(theme.GridIcon())
		sv.showGridView()
	} else {
		sv.viewToggleBtn.SetIcon(theme.ListIcon())
		sv.showListView()
	}
}

func (sv *SongsView) showGridView() {
	sv.songsGrid.RemoveAll()
	for _, song := range sv.filteredSongs {
		sv.songsGrid.Add(sv.createSongCard(song))
	}

	if sv.hasMore && !sv.loading {
		loadMoreBtn := widget.NewButton("Load More", func() {
			sv.loadMoreSongs()
		})
		sv.songsGrid.Add(loadMoreBtn)
	}

	sv.scroll.Content = sv.songsGrid
	sv.scroll.Refresh()
}

func (sv *SongsView) showListView() {
	sv.scroll.Content = sv.songsList
	sv.scroll.Refresh()
	sv.songsList.Refresh()
}

func (sv *SongsView) loadSongs() {
	sv.currentPage = 1
	sv.hasMore = true
	go func() {
		ctx := context.Background()
		songs, err := sv.storage.GetSongs(ctx, 1000, 0)
		if err != nil {
			log.Printf("Failed to load songs from storage: %v", err)
			sv.loadFromAPI()
			return
		}

		sv.songs = songs
		sv.filteredSongs = songs
		sv.refreshView()
	}()
}

func (sv *SongsView) loadFromAPI() {
	if sv.loading {
		return
	}

	sv.loading = true
	go func() {
		defer func() { sv.loading = false }()

		ctx := context.Background()
		resp, err := sv.api.GetSongs(ctx, sv.currentPage, "")
		if err != nil {
			log.Printf("Failed to load songs from API: %v", err)
			return
		}

		if sv.currentPage == 1 {
			sv.songs = resp.Results
		} else {
			sv.songs = append(sv.songs, resp.Results...)
		}

		sv.hasMore = resp.Next != nil
		sv.filteredSongs = sv.songs
		sv.refreshView()
	}()
}

func (sv *SongsView) loadMoreSongs() {
	if sv.loading || !sv.hasMore {
		return
	}

	sv.currentPage++
	sv.loadFromAPI()
}

func (sv *SongsView) refreshData() {
	sv.currentPage = 1
	sv.hasMore = true
	sv.songs = nil
	sv.filteredSongs = nil
	sv.loadFromAPI()
}

func (sv *SongsView) onSearchDebounced(query string) {
	if sv.searchTimer != nil {
		sv.searchTimer.Stop()
	}

	sv.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		sv.onSearch(query)
	})
}

func (sv *SongsView) onSearch(query string) {
	if query == "" {
		sv.filteredSongs = sv.songs
	} else {
		ctx := context.Background()
		results, err := sv.search.Search(ctx, query, 100)
		if err != nil {
			log.Printf("Search error: %v", err)
			return
		}
		sv.filteredSongs = results.Songs
	}
	sv.refreshView()
}

func (sv *SongsView) onSortChanged(option string) {
	switch option {
	case "Name A-Z":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			return strings.ToLower(sv.filteredSongs[i].Name) < strings.ToLower(sv.filteredSongs[j].Name)
		})
	case "Name Z-A":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			return strings.ToLower(sv.filteredSongs[i].Name) > strings.ToLower(sv.filteredSongs[j].Name)
		})
	case "Artist A-Z":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			artistI := "Unknown"
			if len(sv.filteredSongs[i].Authors) > 0 {
				artistI = sv.filteredSongs[i].Authors[0].Name
			}
			artistJ := "Unknown"
			if len(sv.filteredSongs[j].Authors) > 0 {
				artistJ = sv.filteredSongs[j].Authors[0].Name
			}
			return strings.ToLower(artistI) < strings.ToLower(artistJ)
		})
	case "Album A-Z":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			albumI := "Unknown"
			if sv.filteredSongs[i].Album != nil {
				albumI = sv.filteredSongs[i].Album.Name
			}
			albumJ := "Unknown"
			if sv.filteredSongs[j].Album != nil {
				albumJ = sv.filteredSongs[j].Album.Name
			}
			return strings.ToLower(albumI) < strings.ToLower(albumJ)
		})
	case "Duration":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			return sv.filteredSongs[i].Length > sv.filteredSongs[j].Length
		})
	case "Play Count":
		sort.Slice(sv.filteredSongs, func(i, j int) bool {
			return sv.filteredSongs[i].Played > sv.filteredSongs[j].Played
		})
	}
	sv.refreshView()
}

func (sv *SongsView) onFilterChanged(filter string) {
	filtered := make([]*types.Song, 0)

	for _, song := range sv.songs {
		include := false
		switch filter {
		case "All Songs":
			include = true
		case "Downloaded":
			include = song.Downloaded
		case "Liked":
			include = song.Liked != nil && *song.Liked
		case "Recently Played":
			include = song.Played > 0
		}

		if include {
			filtered = append(filtered, song)
		}
	}

	sv.filteredSongs = filtered
	sv.refreshView()
}

func (sv *SongsView) refreshView() {
	if sv.scroll == nil {
		return
	}
	if sv.isGridView {
		sv.showGridView()
	} else {
		sv.songsList.Refresh()
	}
}

func (sv *SongsView) OnSongSelected(callback func(*types.Song)) {
	sv.onSongSelected = callback
}

func (sv *SongsView) Refresh() {
	sv.refreshData()
}

func (sv *SongsView) Container() *fyne.Container {
	return sv.container
}

func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
