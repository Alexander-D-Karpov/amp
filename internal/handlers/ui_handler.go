package handlers

import (
	"context"
	"log"
	"os"

	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type UIHandlers struct {
	musicService    *services.MusicService
	imageService    *services.ImageService
	downloadManager *download.Manager
	debug           bool

	onSongSelected     func(*types.Song, []*types.Song)
	onAlbumSelected    func(*types.Album)
	onArtistSelected   func(*types.Author)
	onPlaylistSelected func(*types.Playlist)
}

func NewUIHandlers(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, debug bool) *UIHandlers {
	return &UIHandlers{
		musicService:    musicService,
		imageService:    imageService,
		downloadManager: downloadManager,
		debug:           debug,
	}
}

func (h *UIHandlers) SetOnSongSelected(callback func(*types.Song, []*types.Song)) {
	h.onSongSelected = callback
}

func (h *UIHandlers) SetOnAlbumSelected(callback func(*types.Album)) {
	h.onAlbumSelected = callback
}

func (h *UIHandlers) SetOnArtistSelected(callback func(*types.Author)) {
	h.onArtistSelected = callback
}

func (h *UIHandlers) SetOnPlaylistSelected(callback func(*types.Playlist)) {
	h.onPlaylistSelected = callback
}

func (h *UIHandlers) HandleSongSelection(song *types.Song, playlist []*types.Song) {
	if h.onSongSelected != nil {
		h.onSongSelected(song, playlist)
	}

	isDownloaded := song.Downloaded
	if song.LocalPath != nil && *song.LocalPath != "" {
		if _, err := os.Stat(*song.LocalPath); err == nil {
			isDownloaded = true
		}
	}

	if isDownloaded {
		if h.debug {
			log.Printf("[UI_HANDLERS] Song '%s' already exists locally.", song.Name)
		}
		return
	}

	go func() {
		if h.debug {
			log.Printf("[UI_HANDLERS] Starting background download for: %s", song.Name)
		}
		if err := h.downloadManager.DownloadSong(context.Background(), song); err != nil {
			if h.debug {
				log.Printf("[UI_HANDLERS] Background download failed for %s: %v", song.Name, err)
			}
		}
	}()
}

func (h *UIHandlers) HandleAlbumSelection(album *types.Album) {
	if h.onAlbumSelected != nil {
		h.onAlbumSelected(album)
	}
	if len(album.Songs) > 0 && h.onSongSelected != nil {
		h.onSongSelected(album.Songs[0], album.Songs)
	}
}

func (h *UIHandlers) HandleArtistSelection(artist *types.Author) {
	if h.onArtistSelected != nil {
		h.onArtistSelected(artist)
	}
	if len(artist.Songs) > 0 && h.onSongSelected != nil {
		h.onSongSelected(artist.Songs[0], artist.Songs)
	}
}

func (h *UIHandlers) HandlePlaylistSelection(playlist *types.Playlist) {
	if h.onPlaylistSelected != nil {
		h.onPlaylistSelected(playlist)
	}
	if len(playlist.Songs) > 0 && h.onSongSelected != nil {
		h.onSongSelected(playlist.Songs[0], playlist.Songs)
	}
}
