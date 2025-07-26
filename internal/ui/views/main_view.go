package views

import (
	"context"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/search"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type MainView struct {
	api             *api.Client
	storage         *storage.Database
	search          *search.Engine
	downloadManager *download.Manager
	cfg             *config.Config

	container   *fyne.Container
	currentView string
	compactMode bool

	SongsView     *SongsView
	AlbumsView    *AlbumsView
	ArtistsView   *ArtistsView
	PlaylistsView *PlaylistsView
	DownloadsView *DownloadsView
	StatsView     *StatsView
	SettingsView  *SettingsView

	onSongSelected     func(*types.Song)
	onAlbumSelected    func(*types.Album)
	onArtistSelected   func(*types.Author)
	onPlaylistSelected func(*types.Playlist)
}

func NewMainView(api *api.Client, storage *storage.Database, search *search.Engine, downloadManager *download.Manager, cfg *config.Config) *MainView {
	mv := &MainView{
		api:             api,
		storage:         storage,
		search:          search,
		downloadManager: downloadManager,
		cfg:             cfg,
	}

	mv.setupViews()
	mv.setupLayout()
	mv.ShowView("songs")

	return mv
}

func (mv *MainView) setupViews() {
	mv.SongsView = NewSongsView(mv.api, mv.storage, mv.search)
	mv.AlbumsView = NewAlbumsView(mv.api, mv.storage, mv.search)
	mv.ArtistsView = NewArtistsView(mv.api, mv.storage, mv.search)
	mv.PlaylistsView = NewPlaylistsView(mv.api, mv.storage)
	mv.DownloadsView = NewDownloadsView(mv.downloadManager)
	mv.StatsView = NewStatsView(mv.storage)
	mv.SettingsView = NewSettingsView(mv.cfg)

	mv.setupCallbacks()
}

func (mv *MainView) setupCallbacks() {
	mv.SongsView.OnSongSelected(func(song *types.Song) {
		if mv.onSongSelected != nil {
			mv.onSongSelected(song)
		}

		if mv.cfg.Download.AutoDownload && !song.Downloaded {
			go func() {
				ctx := context.Background()
				if err := mv.downloadManager.DownloadSong(ctx, song); err != nil {
					log.Printf("Failed to auto-download song %s: %v", song.Name, err)
				}
			}()
		}
	})

	mv.AlbumsView.OnAlbumSelected(func(album *types.Album) {
		if mv.onAlbumSelected != nil {
			mv.onAlbumSelected(album)
		}

		if len(album.Songs) > 0 && mv.onSongSelected != nil {
			mv.onSongSelected(album.Songs[0])
		}
	})

	mv.ArtistsView.OnArtistSelected(func(artist *types.Author) {
		if mv.onArtistSelected != nil {
			mv.onArtistSelected(artist)
		}

		if len(artist.Songs) > 0 && mv.onSongSelected != nil {
			mv.onSongSelected(artist.Songs[0])
		}
	})

	mv.PlaylistsView.OnPlaylistSelected(func(playlist *types.Playlist) {
		if mv.onPlaylistSelected != nil {
			mv.onPlaylistSelected(playlist)
		}
	})

	mv.PlaylistsView.OnSongSelected(func(song *types.Song) {
		if mv.onSongSelected != nil {
			mv.onSongSelected(song)
		}
	})
}

func (mv *MainView) setupLayout() {
	mv.container = container.NewStack()
}

func (mv *MainView) ShowView(viewName string) {
	mv.currentView = viewName
	mv.container.RemoveAll()

	var content fyne.CanvasObject

	switch viewName {
	case "songs":
		content = mv.SongsView.Container()
	case "albums":
		content = mv.AlbumsView.Container()
	case "artists":
		content = mv.ArtistsView.Container()
	case "playlists":
		content = mv.PlaylistsView.Container()
	case "downloads":
		content = mv.DownloadsView.Container()
	case "stats":
		content = mv.StatsView.Container()
	case "settings":
		content = mv.SettingsView.Container()
	default:
		content = widget.NewLabel("View not implemented: " + viewName)
	}

	if mv.compactMode {
		content = container.NewScroll(content)
	}

	mv.container.Add(content)
	mv.container.Refresh()
}

func (mv *MainView) OnSongSelected(callback func(*types.Song)) {
	mv.onSongSelected = callback
}

func (mv *MainView) OnAlbumSelected(callback func(*types.Album)) {
	mv.onAlbumSelected = callback
}

func (mv *MainView) OnArtistSelected(callback func(*types.Author)) {
	mv.onArtistSelected = callback
}

func (mv *MainView) OnPlaylistSelected(callback func(*types.Playlist)) {
	mv.onPlaylistSelected = callback
}

func (mv *MainView) RefreshData() {
	switch mv.currentView {
	case "songs":
		mv.SongsView.Refresh()
	case "albums":
		mv.AlbumsView.Refresh()
	case "artists":
		mv.ArtistsView.Refresh()
	case "playlists":
		mv.PlaylistsView.Refresh()
	case "downloads":
		mv.DownloadsView.Refresh()
	case "stats":
		mv.StatsView.Refresh()
	}
}

func (mv *MainView) SetCompactMode(compact bool) {
	mv.compactMode = compact

	if mv.SongsView != nil {
		mv.SongsView.SetCompactMode(compact)
	}
	if mv.AlbumsView != nil {
		mv.AlbumsView.SetCompactMode(compact)
	}
	if mv.ArtistsView != nil {
		mv.ArtistsView.SetCompactMode(compact)
	}
	if mv.StatsView != nil {
		mv.StatsView.SetCompactMode(compact)
	}
}

func (mv *MainView) RefreshLayout() {
	mv.ShowView(mv.currentView)
}

func (mv *MainView) GetCurrentView() string {
	return mv.currentView
}

func (mv *MainView) SearchInCurrentView(query string) {
	switch mv.currentView {
	case "songs":
		mv.SongsView.searchEntry.SetText(query)
	case "albums":
		mv.AlbumsView.searchEntry.SetText(query)
	case "artists":
		mv.ArtistsView.searchEntry.SetText(query)
	case "playlists":
		mv.PlaylistsView.searchEntry.SetText(query)
	}
}

func (mv *MainView) Container() *fyne.Container {
	return mv.container
}
