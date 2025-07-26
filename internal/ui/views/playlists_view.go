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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type PlaylistsView struct {
	api          *api.Client
	storage      *storage.Database
	parentWindow fyne.Window

	container     *fyne.Container
	scroll        *container.Scroll
	searchEntry   *widget.Entry
	playlistsGrid *fyne.Container
	playlistsList *widget.List
	createBtn     *widget.Button
	refreshBtn    *widget.Button
	viewToggleBtn *widget.Button
	sortSelect    *widget.Select
	filterSelect  *widget.Select

	playlists         []*types.Playlist
	filteredPlaylists []*types.Playlist
	isGridView        bool
	searchTimer       *time.Timer

	onPlaylistSelected func(*types.Playlist)
	onSongSelected     func(*types.Song)
}

func NewPlaylistsView(api *api.Client, storage *storage.Database) *PlaylistsView {
	pv := &PlaylistsView{
		api:     api,
		storage: storage,
	}

	pv.setupWidgets()
	pv.setupLayout()
	pv.loadPlaylists()

	return pv
}

func (pv *PlaylistsView) setupWidgets() {
	pv.searchEntry = widget.NewEntry()
	pv.searchEntry.SetPlaceHolder("Search playlists...")
	pv.searchEntry.OnChanged = pv.onSearchDebounced

	pv.createBtn = widget.NewButtonWithIcon("New Playlist", theme.ContentAddIcon(), pv.showCreatePlaylistDialog)
	pv.createBtn.Importance = widget.HighImportance

	pv.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), pv.loadPlaylists)
	pv.viewToggleBtn = widget.NewButtonWithIcon("", theme.GridIcon(), pv.toggleView)

	pv.sortSelect = widget.NewSelect([]string{
		"Name A-Z", "Name Z-A", "Recently Created", "Recently Updated", "Song Count",
	}, pv.onSortChanged)
	pv.sortSelect.SetSelected("Name A-Z")

	pv.filterSelect = widget.NewSelect([]string{
		"All Playlists", "My Playlists", "Followed", "Public", "Private",
	}, pv.onFilterChanged)
	pv.filterSelect.SetSelected("All Playlists")

	pv.playlistsGrid = container.NewGridWithColumns(3)
	pv.isGridView = true

	pv.playlistsList = widget.NewList(
		func() int {
			return len(pv.filteredPlaylists)
		},
		func() fyne.CanvasObject {
			return pv.createPlaylistListItem()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			pv.updatePlaylistListItem(id, obj)
		},
	)
	pv.playlistsList.OnSelected = pv.onPlaylistListSelected
}

func (pv *PlaylistsView) createPlaylistListItem() fyne.CanvasObject {
	cover := widget.NewIcon(theme.ListIcon())
	cover.Resize(fyne.NewSize(64, 64))

	nameLabel := widget.NewLabel("Playlist Name")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	creatorLabel := widget.NewLabel("By Creator")
	songsLabel := widget.NewLabel("0 songs")
	durationLabel := widget.NewLabel("0h 0m")

	privacyIcon := widget.NewIcon(theme.VisibilityIcon())
	privacyIcon.Resize(fyne.NewSize(16, 16))

	playlistInfo := container.NewVBox(
		nameLabel,
		creatorLabel,
		container.NewHBox(songsLabel, widget.NewLabel("•"), durationLabel),
	)

	playBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
	})
	playBtn.Importance = widget.LowImportance

	editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
	})
	editBtn.Importance = widget.LowImportance

	deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
	})
	deleteBtn.Importance = widget.LowImportance

	actions := container.NewHBox(playBtn, editBtn, deleteBtn)

	return container.NewBorder(
		nil, nil,
		container.NewHBox(cover, privacyIcon),
		actions,
		playlistInfo,
	)
}

func (pv *PlaylistsView) updatePlaylistListItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(pv.filteredPlaylists) {
		return
	}

	playlist := pv.filteredPlaylists[id]
	c := obj.(*fyne.Container)

	var leftContainer *fyne.Container
	var playlistInfo *fyne.Container

	for _, o := range c.Objects {
		if cont, ok := o.(*fyne.Container); ok {
			if len(cont.Objects) == 2 {
				leftContainer = cont
			} else if len(cont.Objects) == 3 {
				if _, okLabel := cont.Objects[0].(*widget.Label); okLabel {
					playlistInfo = cont
				}
			}
		}
	}

	if leftContainer != nil && len(leftContainer.Objects) >= 2 {
		privacyIcon := leftContainer.Objects[1].(*widget.Icon)
		if playlist.Private {
			privacyIcon.SetResource(theme.VisibilityOffIcon())
		} else {
			privacyIcon.SetResource(theme.VisibilityIcon())
		}
	}

	if playlistInfo != nil && len(playlistInfo.Objects) >= 3 {
		nameLabel := playlistInfo.Objects[0].(*widget.Label)
		creatorLabel := playlistInfo.Objects[1].(*widget.Label)
		detailsContainer := playlistInfo.Objects[2].(*fyne.Container)

		nameLabel.SetText(playlist.Name)
		creatorLabel.SetText("By You")

		if len(detailsContainer.Objects) >= 3 {
			songsLabel := detailsContainer.Objects[0].(*widget.Label)
			durationLabel := detailsContainer.Objects[2].(*widget.Label)

			songsLabel.SetText(fmt.Sprintf("%d songs", len(playlist.Songs)))

			totalDuration := time.Duration(0)
			for _, song := range playlist.Songs {
				totalDuration += time.Duration(song.Length) * time.Second
			}
			hours := int(totalDuration.Hours())
			minutes := int(totalDuration.Minutes()) % 60
			durationLabel.SetText(fmt.Sprintf("%dh %dm", hours, minutes))
		}
	}
}

func (pv *PlaylistsView) onPlaylistListSelected(id widget.ListItemID) {
	if id < len(pv.filteredPlaylists) && pv.onPlaylistSelected != nil {
		pv.onPlaylistSelected(pv.filteredPlaylists[id])
	}
}

func (pv *PlaylistsView) setupLayout() {
	searchContainer := container.NewBorder(nil, nil, pv.createBtn, container.NewHBox(pv.viewToggleBtn, pv.refreshBtn), pv.searchEntry)
	controlsContainer := container.NewHBox(widget.NewLabel("Sort:"), pv.sortSelect, widget.NewLabel("Filter:"), pv.filterSelect)
	header := container.NewVBox(searchContainer, controlsContainer)
	pv.scroll = container.NewScroll(pv.playlistsGrid)
	pv.container = container.NewBorder(header, nil, nil, nil, pv.scroll)
}

func (pv *PlaylistsView) toggleView() {
	pv.isGridView = !pv.isGridView
	if pv.isGridView {
		pv.viewToggleBtn.SetIcon(theme.GridIcon())
		pv.showGridView()
	} else {
		pv.viewToggleBtn.SetIcon(theme.ListIcon())
		pv.showListView()
	}
}

func (pv *PlaylistsView) showGridView() {
	pv.playlistsGrid.RemoveAll()
	for _, pl := range pv.filteredPlaylists {
		pv.playlistsGrid.Add(pv.createPlaylistCard(pl))
	}
	pv.scroll.Content = pv.playlistsGrid
	pv.scroll.Refresh()
}

func (pv *PlaylistsView) showListView() {
	pv.scroll.Content = pv.playlistsList
	pv.scroll.Refresh()
	pv.playlistsList.Refresh()
}

func (pv *PlaylistsView) createPlaylistCard(playlist *types.Playlist) fyne.CanvasObject {
	cover := widget.NewIcon(theme.ListIcon())
	cover.Resize(fyne.NewSize(150, 150))

	nameLabel := widget.NewLabel(playlist.Name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Alignment = fyne.TextAlignCenter
	nameLabel.Wrapping = fyne.TextWrapWord

	creatorLabel := widget.NewLabel("By You")
	creatorLabel.Alignment = fyne.TextAlignCenter

	songsCount := len(playlist.Songs)
	statsLabel := widget.NewLabel(fmt.Sprintf("%d songs", songsCount))
	statsLabel.Alignment = fyne.TextAlignCenter
	statsLabel.TextStyle = fyne.TextStyle{Italic: true}

	var privacyText string
	if playlist.Private {
		privacyText = "Private"
	} else {
		privacyText = "Public"
	}
	privacyLabel := widget.NewLabel(privacyText)
	privacyLabel.Alignment = fyne.TextAlignCenter

	playBtn := widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
		pv.playPlaylist(playlist)
	})
	playBtn.Importance = widget.LowImportance

	cardContent := container.NewVBox(
		cover,
		nameLabel,
		creatorLabel,
		statsLabel,
		privacyLabel,
		playBtn,
	)

	cardBtn := widget.NewButton("", func() {
		if pv.onPlaylistSelected != nil {
			pv.onPlaylistSelected(playlist)
		}
	})
	cardBtn.Importance = widget.LowImportance

	card := widget.NewCard("", "", cardContent)

	return container.NewStack(card, cardBtn)
}

func (pv *PlaylistsView) playPlaylist(playlist *types.Playlist) {
	if len(playlist.Songs) > 0 && pv.onSongSelected != nil {
		pv.onSongSelected(playlist.Songs[0])
	}
}

func (pv *PlaylistsView) showCreatePlaylistDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Playlist name")

	descEntry := widget.NewMultiLineEntry()
	descEntry.SetPlaceHolder("Description (optional)")
	descEntry.Resize(fyne.NewSize(300, 100))

	privateCheck := widget.NewCheck("Private playlist", nil)

	form := container.NewVBox(
		widget.NewLabel("Create new playlist"),
		widget.NewSeparator(),
		widget.NewLabel("Name:"),
		nameEntry,
		widget.NewLabel("Description:"),
		descEntry,
		privateCheck,
	)

	dialog.NewCustomConfirm("Create Playlist", "Create", "Cancel", form, func(confirmed bool) {
		if confirmed && nameEntry.Text != "" {
			pv.createPlaylist(nameEntry.Text, descEntry.Text, privateCheck.Checked)
		}
	}, pv.parentWindow).Show()
}

func (pv *PlaylistsView) createPlaylist(name, description string, private bool) {
	playlist := &types.Playlist{
		Slug:      generateSlug(name),
		Name:      name,
		Private:   private,
		LocalOnly: true,
		Songs:     make([]*types.Song, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	go func() {
		ctx := context.Background()
		if err := pv.storage.SavePlaylist(ctx, playlist); err != nil {
			log.Printf("Failed to create playlist: %v", err)
			return
		}

		pv.loadPlaylists()
	}()
}

func (pv *PlaylistsView) loadPlaylists() {
	go func() {
		ctx := context.Background()
		playlists, err := pv.storage.GetPlaylists(ctx)
		if err != nil {
			log.Printf("Failed to load playlists: %v", err)
			return
		}

		pv.playlists = playlists
		pv.filteredPlaylists = playlists
		pv.refreshView()
	}()
}

func (pv *PlaylistsView) onSearchDebounced(query string) {
	if pv.searchTimer != nil {
		pv.searchTimer.Stop()
	}

	pv.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		pv.onSearch(query)
	})
}

func (pv *PlaylistsView) onSearch(query string) {
	if query == "" {
		pv.filteredPlaylists = pv.playlists
	} else {
		filtered := make([]*types.Playlist, 0)
		queryLower := strings.ToLower(query)
		for _, playlist := range pv.playlists {
			if strings.Contains(strings.ToLower(playlist.Name), queryLower) {
				filtered = append(filtered, playlist)
			}
		}
		pv.filteredPlaylists = filtered
	}
	pv.refreshView()
}

func (pv *PlaylistsView) onSortChanged(option string) {
	switch option {
	case "Name A-Z":
		sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
			return strings.ToLower(pv.filteredPlaylists[i].Name) < strings.ToLower(pv.filteredPlaylists[j].Name)
		})
	case "Name Z-A":
		sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
			return strings.ToLower(pv.filteredPlaylists[i].Name) > strings.ToLower(pv.filteredPlaylists[j].Name)
		})
	case "Recently Created":
		sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
			return pv.filteredPlaylists[i].CreatedAt.After(pv.filteredPlaylists[j].CreatedAt)
		})
	case "Recently Updated":
		sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
			return pv.filteredPlaylists[i].UpdatedAt.After(pv.filteredPlaylists[j].UpdatedAt)
		})
	case "Song Count":
		sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
			return len(pv.filteredPlaylists[i].Songs) > len(pv.filteredPlaylists[j].Songs)
		})
	}
	pv.refreshView()
}

func (pv *PlaylistsView) onFilterChanged(filter string) {
	switch filter {
	case "All Playlists":
		pv.filteredPlaylists = pv.playlists
	case "My Playlists":
		filtered := make([]*types.Playlist, 0)
		for _, playlist := range pv.playlists {
			if playlist.LocalOnly {
				filtered = append(filtered, playlist)
			}
		}
		pv.filteredPlaylists = filtered
	case "Public":
		filtered := make([]*types.Playlist, 0)
		for _, playlist := range pv.playlists {
			if !playlist.Private {
				filtered = append(filtered, playlist)
			}
		}
		pv.filteredPlaylists = filtered
	case "Private":
		filtered := make([]*types.Playlist, 0)
		for _, playlist := range pv.playlists {
			if playlist.Private {
				filtered = append(filtered, playlist)
			}
		}
		pv.filteredPlaylists = filtered
	default:
		pv.filteredPlaylists = pv.playlists
	}
	pv.refreshView()
}

func (pv *PlaylistsView) refreshView() {
	if pv.scroll == nil {
		return
	}
	if pv.isGridView {
		pv.showGridView()
	} else {
		pv.playlistsList.Refresh()
	}
}

func (pv *PlaylistsView) SetParentWindow(window fyne.Window) {
	pv.parentWindow = window
}

func (pv *PlaylistsView) OnPlaylistSelected(callback func(*types.Playlist)) {
	pv.onPlaylistSelected = callback
}

func (pv *PlaylistsView) OnSongSelected(callback func(*types.Song)) {
	pv.onSongSelected = callback
}

func (pv *PlaylistsView) Refresh() {
	pv.loadPlaylists()
}

func (pv *PlaylistsView) Container() *fyne.Container {
	return pv.container
}

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	result := ""
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result += string(r)
		}
	}
	return result + "-" + fmt.Sprintf("%d", time.Now().Unix())
}
