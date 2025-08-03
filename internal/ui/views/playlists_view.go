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

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type PlaylistsView struct {
	musicService *services.MusicService
	debug        bool

	container     *fyne.Container
	playlistsGrid *fyne.Container
	searchEntry   *widget.Entry
	refreshBtn    *widget.Button
	sortSelect    *widget.Select

	mu                sync.RWMutex
	playlists         []*types.Playlist
	filteredPlaylists []*types.Playlist
	searchTimer       *time.Timer
	loading           bool

	onPlaylistSelected func(*types.Playlist)
}

func NewPlaylistsView(musicService *services.MusicService, debug bool) *PlaylistsView {
	pv := &PlaylistsView{
		musicService:      musicService,
		debug:             debug,
		playlists:         make([]*types.Playlist, 0),
		filteredPlaylists: make([]*types.Playlist, 0),
	}

	pv.setupWidgets()
	pv.setupLayout()
	pv.loadPlaylists()
	return pv
}

func (pv *PlaylistsView) setupWidgets() {
	pv.searchEntry = widget.NewEntry()
	pv.searchEntry.SetPlaceHolder("Search playlists...")
	pv.searchEntry.OnChanged = pv.onSearchChanged

	pv.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), pv.loadPlaylists)

	pv.sortSelect = widget.NewSelect([]string{
		"Name A-Z", "Name Z-A", "Recently Created", "Song Count",
	}, pv.onSortChanged)

	pv.playlistsGrid = container.NewGridWithColumns(3)
}

func (pv *PlaylistsView) setupLayout() {
	searchBar := container.NewBorder(nil, nil, nil, pv.refreshBtn, pv.searchEntry)
	controls := container.NewHBox(widget.NewLabel("Sort:"), pv.sortSelect)
	header := container.NewVBox(searchBar, controls)
	content := container.NewScroll(pv.playlistsGrid)
	pv.container = container.NewBorder(header, nil, nil, nil, content)

	pv.sortSelect.SetSelected("Name A-Z")
}

func (pv *PlaylistsView) onSearchChanged(query string) {
	if pv.searchTimer != nil {
		pv.searchTimer.Stop()
	}
	pv.searchTimer = time.AfterFunc(300*time.Millisecond, func() {
		pv.applyFilter(query)
	})
}

func (pv *PlaylistsView) onSortChanged(option string) {
	pv.applySortAndFilter()
	fyne.Do(func() {
		pv.refreshView()
	})
}

func (pv *PlaylistsView) loadPlaylists() {
	pv.mu.Lock()
	if pv.loading {
		pv.mu.Unlock()
		return
	}
	pv.loading = true
	pv.mu.Unlock()

	go func() {
		defer func() {
			pv.mu.Lock()
			pv.loading = false
			pv.mu.Unlock()
		}()

		ctx := context.Background()
		playlists, err := pv.musicService.GetPlaylists(ctx)
		if err != nil {
			return
		}

		pv.mu.Lock()
		pv.playlists = playlists
		pv.filteredPlaylists = playlists
		pv.mu.Unlock()

		fyne.Do(func() {
			pv.refreshView()
		})
	}()
}

func (pv *PlaylistsView) applyFilter(query string) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

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

	fyne.Do(func() {
		pv.refreshView()
	})
}

func (pv *PlaylistsView) applySortAndFilter() {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	if pv.sortSelect == nil {
		return
	}

	pv.filteredPlaylists = pv.playlists

	sortOpt := pv.sortSelect.Selected
	sort.Slice(pv.filteredPlaylists, func(i, j int) bool {
		p1, p2 := pv.filteredPlaylists[i], pv.filteredPlaylists[j]
		switch sortOpt {
		case "Name A-Z":
			return strings.ToLower(p1.Name) < strings.ToLower(p2.Name)
		case "Name Z-A":
			return strings.ToLower(p1.Name) > strings.ToLower(p2.Name)
		case "Recently Created":
			return p1.CreatedAt.After(p2.CreatedAt)
		case "Song Count":
			return len(p1.Songs) > len(p2.Songs)
		}
		return false
	})
}

func (pv *PlaylistsView) refreshView() {
	if pv.playlistsGrid == nil {
		return
	}

	pv.playlistsGrid.RemoveAll()

	pv.mu.RLock()
	playlists := make([]*types.Playlist, len(pv.filteredPlaylists))
	copy(playlists, pv.filteredPlaylists)
	pv.mu.RUnlock()

	for _, playlist := range playlists {
		pv.playlistsGrid.Add(pv.createPlaylistCard(playlist))
	}
	pv.playlistsGrid.Refresh()
}

func (pv *PlaylistsView) createPlaylistCard(playlist *types.Playlist) fyne.CanvasObject {
	cover := widget.NewIcon(theme.ListIcon())
	cover.Resize(fyne.NewSize(120, 120))

	name := widget.NewLabel(playlist.Name)
	name.Alignment = fyne.TextAlignCenter
	name.TextStyle = fyne.TextStyle{Bold: true}
	name.Wrapping = fyne.TextWrapWord

	songsCount := len(playlist.Songs)
	stats := widget.NewLabel(fmt.Sprintf("%d songs", songsCount))
	stats.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(cover, name, stats)

	btn := widget.NewButton("", func() {
		if pv.onPlaylistSelected != nil {
			pv.onPlaylistSelected(playlist)
		}
	})

	return container.NewStack(content, btn)
}

func (pv *PlaylistsView) OnPlaylistSelected(callback func(*types.Playlist)) {
	pv.onPlaylistSelected = callback
}

func (pv *PlaylistsView) Refresh() {
	pv.loadPlaylists()
}

func (pv *PlaylistsView) Container() *fyne.Container {
	return pv.container
}
