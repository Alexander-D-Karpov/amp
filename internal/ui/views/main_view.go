package views

import (
	"context"
	"log"

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

	SongDetailView   *SongDetailView
	AlbumDetailView  *AlbumDetailView
	AuthorDetailView *AuthorDetailView

	parentWindow fyne.Window

	current string
	history []string

	musicService *services.MusicService
	imageService *services.ImageService
}

const (
	viewSongs        = "songs"
	viewAlbums       = "albums"
	viewArtists      = "artists"
	viewPlaylists    = "playlists"
	viewDownloads    = "downloads"
	viewStats        = "stats"
	viewSettings     = "settings"
	viewSongDetail   = "song_detail"
	viewAlbumDetail  = "album_detail"
	viewAuthorDetail = "author_detail"
)

func NewMainView(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, playSyncService *services.PlaySyncService, cfg *config.Config) *MainView {
	handlers := handlers.NewUIHandlers(musicService, imageService, downloadManager, playSyncService, cfg.Debug)

	mv := &MainView{
		handlers:     handlers,
		views:        make(map[string]fyne.CanvasObject),
		musicService: musicService,
		imageService: imageService,
		current:      "",
		history:      make([]string, 0),
	}

	mv.setupViews(musicService, imageService, downloadManager, cfg)

	mv.container = container.NewBorder(nil, nil, nil, nil, mv.SongsView.Container())
	mv.current = viewSongs

	mv.SongsView.SetOpenAlbumBySlug(mv.OpenAlbumBySlug)
	mv.SongsView.SetOpenAuthorBySlug(mv.OpenAuthorBySlug)
	mv.SongsView.SetOpenSongBySlug(mv.OpenSongBySlug)

	return mv
}

func (mv *MainView) SetParentWindow(window fyne.Window) {
	mv.parentWindow = window
	if mv.SongsView != nil {
		mv.SongsView.SetParentWindow(window)
	}
	if mv.AlbumsView != nil {
		mv.AlbumsView.SetParentWindow(window)
	}
	if mv.ArtistsView != nil {
		mv.ArtistsView.SetParentWindow(window)
	}
	if mv.SettingsView != nil {
		mv.SettingsView.SetParentWindow(window)
	}
}

func (mv *MainView) setupViews(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, cfg *config.Config) {
	mv.SongsView = NewSongsView(musicService, imageService, mv.handlers)
	mv.AlbumsView = NewAlbumsView(musicService, imageService, mv.handlers, cfg.Debug)
	mv.ArtistsView = NewArtistsView(musicService, imageService, mv.handlers, cfg.Debug)
	mv.PlaylistsView = NewPlaylistsView(musicService, cfg.Debug)
	mv.DownloadsView = NewDownloadsView(downloadManager)
	mv.StatsView = NewStatsView(musicService)
	mv.SettingsView = NewSettingsView(cfg)

	mv.views[viewSongs] = mv.SongsView.Container()
	mv.views[viewAlbums] = mv.AlbumsView.Container()
	mv.views[viewArtists] = mv.ArtistsView.Container()
	mv.views[viewPlaylists] = mv.PlaylistsView.Container()
	mv.views[viewDownloads] = mv.DownloadsView.Container()
	mv.views[viewStats] = mv.StatsView.Container()
	mv.views[viewSettings] = mv.SettingsView.Container()

	mv.SongDetailView = NewSongDetailView(imageService)
	mv.AlbumDetailView = NewAlbumDetailView(imageService)
	mv.AuthorDetailView = NewAuthorDetailView(imageService)

	mv.SongDetailView.SetOnBack(func() {
		mv.ShowView("songs")
	})
	mv.SongDetailView.SetOnOpenAlbum(func(slug string) {
		mv.OpenAlbumBySlug(slug)
	})
	mv.SongDetailView.SetOnOpenAuthor(func(slug string) {
		mv.OpenAuthorBySlug(slug)
	})
	mv.SongsView.SetDownloadHandler(func(song *types.Song) {
		if mv.handlers != nil {
			mv.handlers.HandleDownloadSong(song)
		}
	})

	mv.AlbumDetailView.SetCallbacks(
		func() { mv.ShowView("albums") },
		func(s *types.Song) {
			if mv.handlers != nil {
				mv.handlers.HandleSongSelection(s, []*types.Song{s})
			}
		},
		func(slug string) { mv.OpenAlbumBySlug(slug) },
		func(slug string) { mv.OpenAuthorBySlug(slug) },
		func(s *types.Song) { mv.OpenSongBySlug(s.Slug) },
	)

	mv.views[viewSongDetail] = mv.SongDetailView.Container()
	mv.views[viewAlbumDetail] = mv.AlbumDetailView.Container()
	mv.views[viewAuthorDetail] = mv.AuthorDetailView.Container()

	mv.AuthorDetailView.SetCallbacks(
		func() { mv.ShowView("artists") },
		func(s *types.Song) {
			if mv.handlers != nil {
				mv.handlers.HandleSongSelection(s, []*types.Song{s})
			}
		},
		func(slug string) { mv.OpenAlbumBySlug(slug) },
		func(slug string) { mv.OpenAuthorBySlug(slug) },
	)

	mv.setupContextMenuCallbacks(downloadManager)

	mv.PlaylistsView.OnPlaylistSelected(func(playlist *types.Playlist) {
		mv.handlers.HandlePlaylistSelection(playlist)
	})
}

func (mv *MainView) setupContextMenuCallbacks(downloadManager *download.Manager) {
	// Set up SongsView callbacks
	mv.SongsView.SetCallbacks(
		func(song *types.Song) {
			// Download callback
			if song == nil {
				return
			}

			log.Printf("[MAIN_VIEW] Download requested for song: %s", song.Name)

			go func() {
				ctx := context.Background()
				if err := downloadManager.DownloadSong(ctx, song); err != nil {
					log.Printf("[MAIN_VIEW] Download failed for %s: %v", song.Name, err)
				} else {
					log.Printf("[MAIN_VIEW] Download started for %s", song.Name)

					// Update the song's downloaded status after successful start
					fyne.Do(func() {
						// Refresh the view to show updated download status
						mv.SongsView.updateGridView()
					})
				}
			}()
		},
		func(song *types.Song) {
			// Add to playlist callback
			mv.showAddToPlaylistDialog(song)
		},
	)

	// Set up AlbumsView callbacks
	mv.AlbumsView.SetCallbacks(
		func(album *types.Album) {
			// Download all songs in album
			ctx := context.Background()
			for _, song := range album.Songs {
				if song != nil {
					go func(s *types.Song) {
						if err := downloadManager.DownloadSong(ctx, s); err != nil {
							log.Printf("[MAIN_VIEW] Failed to download song %s from album %s: %v",
								s.Name, album.Name, err)
						}
					}(song)
				}
			}
		},
		func(album *types.Album) {
			mv.showAddAlbumToPlaylistDialog(album)
		},
	)

	// Set up ArtistsView callbacks
	mv.ArtistsView.SetCallbacks(
		func(artist *types.Author) {
			// Download all songs by artist
			ctx := context.Background()
			for _, song := range artist.Songs {
				if song != nil {
					go func(s *types.Song) {
						if err := downloadManager.DownloadSong(ctx, s); err != nil {
							log.Printf("[MAIN_VIEW] Failed to download song %s by artist %s: %v",
								s.Name, artist.Name, err)
						}
					}(song)
				}
			}
		},
		func(artist *types.Author) {
			mv.showAddArtistToPlaylistDialog(artist)
		},
	)
}

func (mv *MainView) showAddToPlaylistDialog(song *types.Song)      {}
func (mv *MainView) showAddAlbumToPlaylistDialog(a *types.Album)   {}
func (mv *MainView) showAddArtistToPlaylistDialog(a *types.Author) {}

func (mv *MainView) ShowView(name string) {
	if name == mv.current {
		return
	}

	targetView, exists := mv.views[name]
	if !exists {
		return
	}

	if mv.current != "" && mv.current != name {
		mv.history = append(mv.history, mv.current)
	}

	mv.container.RemoveAll()
	mv.container.Add(targetView)
	mv.current = name
	mv.container.Refresh()
}

func (mv *MainView) GoBack() {
	if len(mv.history) == 0 {
		mv.ShowView(viewSongs)
		return
	}

	last := mv.history[len(mv.history)-1]
	mv.history = mv.history[:len(mv.history)-1]

	targetView, exists := mv.views[last]
	if !exists {
		mv.ShowView(viewSongs)
		return
	}

	mv.container.RemoveAll()
	mv.container.Add(targetView)
	mv.current = last
	mv.container.Refresh()
}

func (mv *MainView) OpenSongDetail(song *types.Song) {
	if song == nil {
		return
	}
	mv.SongDetailView.SetSong(song)
	mv.ShowView(viewSongDetail)
}

func (mv *MainView) OpenAlbumDetail(album *types.Album) {
	if album == nil {
		return
	}
	mv.AlbumDetailView.SetAlbum(album)
	mv.ShowView(viewAlbumDetail)
}

func (mv *MainView) OpenAuthorDetail(author *types.Author) {
	if author == nil {
		return
	}
	mv.AuthorDetailView.SetAuthor(author)
	mv.ShowView(viewAuthorDetail)
}

func (mv *MainView) OpenSongBySlug(slug string) {
	go func() {
		ctx := context.Background()
		song, err := mv.handlers.Music().GetSong(ctx, slug)
		if err != nil || song == nil {
			return
		}
		fyne.Do(func() {
			mv.SongDetailView.ShowSong(song)
			mv.ShowView(viewSongDetail)
		})
	}()
}

func (mv *MainView) OpenAlbumBySlug(slug string) {
	go func() {
		ctx := context.Background()
		album, err := mv.handlers.Music().GetAlbum(ctx, slug)
		if err != nil || album == nil {
			return
		}
		fyne.Do(func() {
			mv.AlbumDetailView.ShowAlbum(album)
			mv.ShowView(viewAlbumDetail)
		})
	}()
}

func (mv *MainView) OpenAuthorBySlug(slug string) {
	go func() {
		ctx := context.Background()
		author, err := mv.handlers.Music().GetAuthor(ctx, slug)
		if err != nil || author == nil {
			return
		}
		fyne.Do(func() {
			mv.AuthorDetailView.ShowAuthor(author)
			mv.ShowView(viewAuthorDetail)
		})
	}()
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
	switch mv.current {
	case viewSongs:
		mv.SongsView.searchEntry.SetText(query)
	case viewAlbums:
		mv.AlbumsView.searchEntry.SetText(query)
	case viewArtists:
		mv.ArtistsView.searchEntry.SetText(query)
	case viewPlaylists:
		mv.PlaylistsView.searchEntry.SetText(query)
	}
}

func (mv *MainView) Container() *fyne.Container {
	return mv.container
}
