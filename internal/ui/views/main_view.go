package views

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/handlers"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type MainView struct {
	handlers *handlers.UIHandlers

	container *fyne.Container
	views     map[string]fyne.CanvasObject

	SongsView     *SongsView
	AlbumsView    *AlbumsView
	ArtistsView   *ArtistsView
	PlaylistsView *PlaylistsView
	DownloadsView *DownloadsView
	StatsView     *StatsView
	SettingsView  *SettingsView
}

func NewMainView(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, cfg *config.Config) *MainView {
	handlers := handlers.NewUIHandlers(musicService, imageService, downloadManager, cfg.Debug)

	mv := &MainView{
		handlers: handlers,
		views:    make(map[string]fyne.CanvasObject),
	}

	mv.setupViews(musicService, imageService, downloadManager, cfg)
	mv.container = container.NewStack(mv.views["songs"])
	mv.ShowView("songs")

	return mv
}

func (mv *MainView) setupViews(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, cfg *config.Config) {
	mv.SongsView = NewSongsView(musicService, imageService, mv.handlers)
	mv.AlbumsView = NewAlbumsView(musicService, imageService, mv.handlers, cfg.Debug)
	mv.ArtistsView = NewArtistsView(musicService, imageService, mv.handlers, cfg.Debug)
	mv.PlaylistsView = NewPlaylistsView(musicService, cfg.Debug)
	mv.DownloadsView = NewDownloadsView(downloadManager)
	mv.StatsView = NewStatsView(musicService)
	mv.SettingsView = NewSettingsView(cfg)

	mv.views["songs"] = mv.SongsView.Container()
	mv.views["albums"] = mv.AlbumsView.Container()
	mv.views["artists"] = mv.ArtistsView.Container()
	mv.views["playlists"] = mv.PlaylistsView.Container()
	mv.views["downloads"] = mv.DownloadsView.Container()
	mv.views["stats"] = mv.StatsView.Container()
	mv.views["settings"] = mv.SettingsView.Container()

	for _, view := range mv.views {
		view.Hide()
	}

	mv.PlaylistsView.OnPlaylistSelected(func(playlist *types.Playlist) {
		mv.handlers.HandlePlaylistSelection(playlist)
	})
}

func (mv *MainView) ShowView(viewName string) {
	for name, view := range mv.views {
		if name == viewName {
			view.Show()
		} else {
			view.Hide()
		}
	}
}

func (mv *MainView) OnSongSelected(callback func(*types.Song, []*types.Song)) {
	mv.handlers.SetOnSongSelected(callback)
}

func (mv *MainView) OnAlbumSelected(callback func(*types.Album)) {
	mv.handlers.SetOnAlbumSelected(callback)
}

func (mv *MainView) OnArtistSelected(callback func(*types.Author)) {
	mv.handlers.SetOnArtistSelected(callback)
}

func (mv *MainView) OnPlaylistSelected(callback func(*types.Playlist)) {
	mv.handlers.SetOnPlaylistSelected(callback)
}

func (mv *MainView) RefreshData() {
	mv.SongsView.Refresh()
	mv.AlbumsView.Refresh()
	mv.ArtistsView.Refresh()
	mv.PlaylistsView.Refresh()
}

func (mv *MainView) SetCompactMode(compact bool) {
	mv.SongsView.SetCompactMode(compact)
	mv.AlbumsView.SetCompactMode(compact)
	mv.ArtistsView.SetCompactMode(compact)
	mv.StatsView.SetCompactMode(compact)
}

func (mv *MainView) SearchInCurrentView(query string) {
	if mv.SongsView.Container().Visible() {
		mv.SongsView.searchEntry.SetText(query)
	} else if mv.AlbumsView.Container().Visible() {
		mv.AlbumsView.searchEntry.SetText(query)
	} else if mv.ArtistsView.Container().Visible() {
		mv.ArtistsView.searchEntry.SetText(query)
	} else if mv.PlaylistsView.Container().Visible() {
		mv.PlaylistsView.searchEntry.SetText(query)
	}
}

func (mv *MainView) Container() *fyne.Container {
	return mv.container
}
