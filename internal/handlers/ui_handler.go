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
		debug:           false, // Reduced debug logging
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

	// Check if song is already available locally
	if h.isLocallyAvailable(song) {
		if h.debug {
			log.Printf("[UI_HANDLERS] Song '%s' is available locally", song.Name)
		}
		return
	}

	// Start background download for streaming songs
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

func (h *UIHandlers) isLocallyAvailable(song *types.Song) bool {
	// Check if already marked as downloaded
	if song.Downloaded {
		return true
	}

	// Check if local path exists and file is accessible
	if song.LocalPath != nil && *song.LocalPath != "" {
		if stat, err := os.Stat(*song.LocalPath); err == nil && stat.Size() > 0 {
			// Update the song record to mark as downloaded
			song.Downloaded = true
			return true
		}
	}

	return false
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

// SetDebug enables or disables debug logging
func (h *UIHandlers) SetDebug(debug bool) {
	h.debug = debug
}
