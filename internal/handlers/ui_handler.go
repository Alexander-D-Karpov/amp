package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type UIHandlers struct {
	musicService    *services.MusicService
	imageService    *services.ImageService
	DownloadManager *download.Manager
	playSyncService *services.PlaySyncService
	debug           bool

	onSongSelected     func(*types.Song, []*types.Song)
	onAlbumSelected    func(*types.Album)
	onArtistSelected   func(*types.Author)
	onPlaylistSelected func(*types.Playlist)
}

func NewUIHandlers(musicService *services.MusicService, imageService *services.ImageService, downloadManager *download.Manager, playSyncService *services.PlaySyncService, debug bool) *UIHandlers {
	return &UIHandlers{
		musicService:    musicService,
		imageService:    imageService,
		DownloadManager: downloadManager,
		playSyncService: playSyncService,
		debug:           debug,
	}
}

// Add getter methods for access from views
func (h *UIHandlers) Music() *services.MusicService {
	return h.musicService
}

func (h *UIHandlers) Download() *download.Manager {
	return h.DownloadManager
}

func (h *UIHandlers) PlaySync() *services.PlaySyncService {
	return h.playSyncService
}

func (h *UIHandlers) Images() *services.ImageService {
	return h.imageService
}

// Add a method specifically for downloading songs
func (h *UIHandlers) HandleDownloadSong(song *types.Song) error {
	if song == nil {
		return fmt.Errorf("song is nil")
	}

	if h.debug {
		log.Printf("[UI_HANDLERS] Download requested for: %s", song.Name)
	}

	go func() {
		ctx := context.Background()
		if err := h.DownloadManager.DownloadSong(ctx, song); err != nil {
			log.Printf("[UI_HANDLERS] Download failed for %s: %v", song.Name, err)
		} else {
			log.Printf("[UI_HANDLERS] Download started successfully for: %s", song.Name)
		}
	}()

	return nil
}

// Existing methods...
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
	if h.debug {
		log.Printf("[UI_HANDLERS] Song selected for playback: %s", song.Name)
	}

	if h.onSongSelected != nil {
		h.onSongSelected(song, playlist)
	}

	go func() {
		ctx := context.Background()

		if h.playSyncService != nil {
			if err := h.playSyncService.RecordAndSendListen(ctx, song); err != nil {
				if h.debug {
					log.Printf("[UI_HANDLERS] Failed to record/send listen for %s: %v", song.Name, err)
				}
			}
		}

		if h.isLocallyAvailable(song) {
			if h.debug {
				log.Printf("[UI_HANDLERS] Song '%s' is available locally", song.Name)
			}
			return
		}

		if h.debug {
			log.Printf("[UI_HANDLERS] Starting background download for: %s", song.Name)
		}
		if err := h.DownloadManager.DownloadSong(ctx, song); err != nil {
			if h.debug {
				log.Printf("[UI_HANDLERS] Background download failed for %s: %v", song.Name, err)
			}
		}
	}()
}

func (h *UIHandlers) isLocallyAvailable(song *types.Song) bool {
	if song == nil {
		return false
	}

	// Check multiple possible locations for the file
	locations := []string{}

	// 1. Check explicit LocalPath
	if song.LocalPath != nil && *song.LocalPath != "" {
		locations = append(locations, *song.LocalPath)
	}

	// 2. Check standard cache location
	if song.Slug != "" {
		filename := song.Slug + ".mp3"
		cachePath := filepath.Join("cache", "songs", filename)
		locations = append(locations, cachePath)
	}

	// 3. Check alternative cache location with safe filename
	if song.Name != "" {
		safeFilename := h.generateSafeFilename(song.Name) + ".mp3"
		cachePath := filepath.Join("cache", "songs", safeFilename)
		locations = append(locations, cachePath)
	}

	// Check each location
	for _, path := range locations {
		if stat, err := os.Stat(path); err == nil && stat.Size() > 1024 { // At least 1KB
			if h.debug {
				log.Printf("[UI_HANDLERS] Found local file for '%s' at: %s (%d bytes)",
					song.Name, path, stat.Size())
			}

			// Update song metadata
			song.LocalPath = &path
			song.Downloaded = true

			// Save updated metadata
			go func() {
				ctx := context.Background()
				if err := h.musicService.GetStorage().SaveSong(ctx, song); err != nil {
					log.Printf("[UI_HANDLERS] Failed to update song metadata: %v", err)
				}
			}()

			return true
		}
	}

	// Mark as not downloaded if we can't find it
	if song.Downloaded {
		song.Downloaded = false
		go func() {
			ctx := context.Background()
			if err := h.musicService.GetStorage().SaveSong(ctx, song); err != nil {
				log.Printf("[UI_HANDLERS] Failed to update song metadata: %v", err)
			}
		}()
	}

	return false
}

func (h *UIHandlers) generateSafeFilename(name string) string {
	safe := strings.ReplaceAll(name, "/", "-")
	safe = strings.ReplaceAll(safe, "\\", "-")
	safe = strings.ReplaceAll(safe, ":", "-")
	safe = strings.ReplaceAll(safe, "*", "-")
	safe = strings.ReplaceAll(safe, "?", "-")
	safe = strings.ReplaceAll(safe, "\"", "-")
	safe = strings.ReplaceAll(safe, "<", "-")
	safe = strings.ReplaceAll(safe, ">", "-")
	safe = strings.ReplaceAll(safe, "|", "-")

	if len(safe) > 100 {
		safe = safe[:100]
	}

	return safe
}

func (h *UIHandlers) HandleDownloadCompletion(song *types.Song, localPath string) {
	if song == nil || localPath == "" {
		return
	}

	// Verify file exists and has reasonable size
	if stat, err := os.Stat(localPath); err == nil && stat.Size() > 1024 {
		song.LocalPath = &localPath
		song.Downloaded = true

		if h.debug {
			log.Printf("[UI_HANDLERS] Download completed for '%s': %s (%d bytes)",
				song.Name, localPath, stat.Size())
		}

		// Update database
		go func() {
			ctx := context.Background()
			if err := h.musicService.GetStorage().SaveSong(ctx, song); err != nil {
				log.Printf("[UI_HANDLERS] Failed to save download completion: %v", err)
			}
		}()
	}
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

func (h *UIHandlers) SetDebug(debug bool) {
	h.debug = debug
}
