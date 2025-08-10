package services

import (
	"context"
	"fmt"
	"log"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/search"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type MusicService struct {
	api     *api.Client
	storage *storage.Database
	search  *search.SearchEngine
	debug   bool
}

func NewMusicService(api *api.Client, storage *storage.Database, search *search.SearchEngine) *MusicService {
	return &MusicService{
		api:     api,
		storage: storage,
		search:  search,
		debug:   false, // Reduced debug logging
	}
}

func (s *MusicService) GetSongs(ctx context.Context, page int, searchQuery string) ([]*types.Song, bool, error) {
	if searchQuery != "" {
		// Try API search first
		resp, err := s.api.GetSongs(ctx, page, searchQuery)
		if err != nil {
			// Fallback to local search without flooding logs
			results, searchErr := s.search.Search(ctx, searchQuery, 100)
			if searchErr != nil {
				return nil, false, fmt.Errorf("search failed: %w", searchErr)
			}
			return results.Songs, false, nil
		}

		// Cache songs in background to avoid blocking UI
		go s.cacheSongsInBackground(ctx, resp.Results)
		return resp.Results, resp.Next != nil, nil
	}

	// Get songs without search
	resp, err := s.api.GetSongs(ctx, page, "")
	if err != nil {
		// Fallback to storage
		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		songs, dbErr := s.storage.GetSongs(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		return songs, len(songs) == limit, nil
	}

	// Cache songs in background
	go s.cacheSongsInBackground(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) cacheSongsInBackground(ctx context.Context, songs []*types.Song) {
	for _, song := range songs {
		if song == nil {
			continue
		}

		// Cache album if present
		if song.Album != nil {
			if err := s.storage.SaveAlbum(ctx, song.Album); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", song.Album.Name, err)
			}
		}

		// Cache authors
		for _, author := range song.Authors {
			if author != nil {
				if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
				}
			}
		}

		// Cache song
		if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
		}
	}
}

func (s *MusicService) GetAlbums(ctx context.Context, page int, searchQuery string) ([]*types.Album, bool, error) {
	resp, err := s.api.GetAlbums(ctx, page, searchQuery)
	if err != nil {
		// Fallback to storage
		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		albums, dbErr := s.storage.GetAlbums(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		return albums, len(albums) == limit, nil
	}

	// Cache albums in background
	go s.cacheAlbumsInBackground(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) cacheAlbumsInBackground(ctx context.Context, albums []*types.Album) {
	for _, album := range albums {
		if album == nil {
			continue
		}

		// Cache artists
		for _, artist := range album.Artists {
			if artist != nil {
				if err := s.storage.SaveAuthor(ctx, artist); err != nil && s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to cache artist %s: %v", artist.Name, err)
				}
			}
		}

		// Cache album
		if err := s.storage.SaveAlbum(ctx, album); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", album.Name, err)
		}
	}
}

func (s *MusicService) GetAuthors(ctx context.Context, page int, searchQuery string) ([]*types.Author, bool, error) {
	resp, err := s.api.GetAuthors(ctx, page, searchQuery)
	if err != nil {
		// Fallback to storage
		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		authors, dbErr := s.storage.GetAuthors(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		return authors, len(authors) == limit, nil
	}

	// Cache authors in background
	go s.cacheAuthorsInBackground(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) cacheAuthorsInBackground(ctx context.Context, authors []*types.Author) {
	for _, author := range authors {
		if author != nil {
			if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
			}
		}
	}
}

func (s *MusicService) GetPlaylists(ctx context.Context) ([]*types.Playlist, error) {
	playlists, err := s.api.GetPlaylists(ctx)
	if err != nil {
		// Fallback to storage
		dbPlaylists, dbErr := s.storage.GetPlaylists(ctx)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		return dbPlaylists, nil
	}

	// Cache playlists in background
	go s.cachePlaylistsInBackground(ctx, playlists)
	return playlists, nil
}

func (s *MusicService) cachePlaylistsInBackground(ctx context.Context, playlists []*types.Playlist) {
	for _, playlist := range playlists {
		if playlist == nil {
			continue
		}

		// Cache songs in playlist
		for _, song := range playlist.Songs {
			if song != nil {
				// Cache album and authors for each song
				if song.Album != nil {
					if err := s.storage.SaveAlbum(ctx, song.Album); err != nil && s.debug {
						log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", song.Album.Name, err)
					}
				}

				for _, author := range song.Authors {
					if author != nil {
						if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
							log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
						}
					}
				}

				if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
				}
			}
		}

		// Cache playlist
		if err := s.storage.SavePlaylist(ctx, playlist); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to cache playlist %s: %v", playlist.Name, err)
		}
	}
}

func (s *MusicService) GetPlaylist(ctx context.Context, slug string) (*types.Playlist, error) {
	playlist, err := s.api.GetPlaylist(ctx, slug)
	if err != nil {
		// Fallback to storage
		dbPlaylist, dbErr := s.storage.GetPlaylist(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		return dbPlaylist, nil
	}

	// Cache playlist in background
	go func() {
		for _, song := range playlist.Songs {
			if song != nil {
				if song.Album != nil {
					s.storage.SaveAlbum(ctx, song.Album)
				}
				for _, author := range song.Authors {
					if author != nil {
						s.storage.SaveAuthor(ctx, author)
					}
				}
				s.storage.SaveSong(ctx, song)
			}
		}
		s.storage.SavePlaylist(ctx, playlist)
	}()

	return playlist, nil
}

func (s *MusicService) SearchAll(ctx context.Context, query string) (*types.SearchResponse, error) {
	result, err := s.api.SearchAll(ctx, query)
	if err != nil {
		// Fallback to local search
		localResults, localErr := s.search.Search(ctx, query, 100)
		if localErr != nil {
			return nil, fmt.Errorf("both API and local search failed: api=%w, local=%w", err, localErr)
		}

		return &types.SearchResponse{
			Songs:   localResults.Songs,
			Albums:  localResults.Albums,
			Authors: localResults.Authors,
		}, nil
	}

	return result, nil
}

// SetDebug enables or disables debug logging
func (s *MusicService) SetDebug(debug bool) {
	s.debug = debug
}
