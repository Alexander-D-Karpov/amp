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
		debug:   true,
	}
}

func (s *MusicService) GetSongs(ctx context.Context, page int, searchQuery string) ([]*types.Song, bool, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] GetSongs - page: %d, search: '%s'", page, searchQuery)
	}

	if searchQuery != "" {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] Using API search for query: '%s'", searchQuery)
		}

		resp, err := s.api.GetSongs(ctx, page, searchQuery)
		if err != nil {
			if s.debug {
				log.Printf("[MUSIC_SERVICE] API search failed, falling back to local search: %v", err)
			}

			results, searchErr := s.search.Search(ctx, searchQuery, 100)
			if searchErr != nil {
				return nil, false, fmt.Errorf("both API and local search failed: api=%w, local=%w", err, searchErr)
			}
			return results.Songs, false, nil
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] API search returned %d songs", len(resp.Results))
		}

		for _, song := range resp.Results {
			go func(song *types.Song) {
				if err := s.cacheSongSafely(ctx, song); err != nil && s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
				}
			}(song)
		}

		return resp.Results, resp.Next != nil, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] Getting songs from API without search")
	}

	resp, err := s.api.GetSongs(ctx, page, "")
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API failed, falling back to storage: %v", err)
		}

		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		songs, dbErr := s.storage.GetSongs(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Returned %d songs from storage", len(songs))
		}
		return songs, len(songs) == limit, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API returned %d songs", len(resp.Results))
	}

	for _, song := range resp.Results {
		go func(song *types.Song) {
			if err := s.cacheSongSafely(ctx, song); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
			}
		}(song)
	}

	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) cacheSongSafely(ctx context.Context, song *types.Song) error {
	if song.Album != nil {
		if err := s.storage.SaveAlbum(ctx, song.Album); err != nil {
			if s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to save album %s: %v", song.Album.Name, err)
			}
		}
	}

	for _, author := range song.Authors {
		if author != nil {
			if err := s.storage.SaveAuthor(ctx, author); err != nil {
				if s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to save author %s: %v", author.Name, err)
				}
			}
		}
	}

	return s.storage.SaveSong(ctx, song)
}

func (s *MusicService) GetAlbums(ctx context.Context, page int, searchQuery string) ([]*types.Album, bool, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] GetAlbums - page: %d, search: '%s'", page, searchQuery)
	}

	resp, err := s.api.GetAlbums(ctx, page, searchQuery)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API failed, falling back to storage: %v", err)
		}

		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		albums, dbErr := s.storage.GetAlbums(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Returned %d albums from storage", len(albums))
		}
		return albums, len(albums) == limit, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API returned %d albums", len(resp.Results))
	}

	for _, album := range resp.Results {
		go func(album *types.Album) {
			for _, artist := range album.Artists {
				if artist != nil {
					if err := s.storage.SaveAuthor(ctx, artist); err != nil && s.debug {
						log.Printf("[MUSIC_SERVICE] Failed to cache artist %s: %v", artist.Name, err)
					}
				}
			}

			if err := s.storage.SaveAlbum(ctx, album); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", album.Name, err)
			}
		}(album)
	}

	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) GetAuthors(ctx context.Context, page int, searchQuery string) ([]*types.Author, bool, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] GetAuthors - page: %d, search: '%s'", page, searchQuery)
	}

	resp, err := s.api.GetAuthors(ctx, page, searchQuery)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API failed, falling back to storage: %v", err)
		}

		limit := 50
		offset := (page - 1) * limit
		if offset < 0 {
			offset = 0
		}

		authors, dbErr := s.storage.GetAuthors(ctx, limit, offset)
		if dbErr != nil {
			return nil, false, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Returned %d authors from storage", len(authors))
		}
		return authors, len(authors) == limit, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API returned %d authors", len(resp.Results))
	}

	for _, author := range resp.Results {
		go func(author *types.Author) {
			if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
			}
		}(author)
	}

	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) GetPlaylists(ctx context.Context) ([]*types.Playlist, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] GetPlaylists")
	}

	playlists, err := s.api.GetPlaylists(ctx)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API failed, falling back to storage: %v", err)
		}

		dbPlaylists, dbErr := s.storage.GetPlaylists(ctx)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Returned %d playlists from storage", len(dbPlaylists))
		}
		return dbPlaylists, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API returned %d playlists", len(playlists))
	}

	for _, playlist := range playlists {
		go func(playlist *types.Playlist) {
			for _, song := range playlist.Songs {
				if err := s.cacheSongSafely(ctx, song); err != nil && s.debug {
					log.Printf("[MUSIC_SERVICE] Failed to cache playlist song %s: %v", song.Name, err)
				}
			}

			if err := s.storage.SavePlaylist(ctx, playlist); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache playlist %s: %v", playlist.Name, err)
			}
		}(playlist)
	}

	return playlists, nil
}

func (s *MusicService) GetPlaylist(ctx context.Context, slug string) (*types.Playlist, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] GetPlaylist: %s", slug)
	}

	playlist, err := s.api.GetPlaylist(ctx, slug)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API failed, falling back to storage: %v", err)
		}

		dbPlaylist, dbErr := s.storage.GetPlaylist(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Returned playlist from storage: %s", dbPlaylist.Name)
		}
		return dbPlaylist, nil
	}

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API returned playlist: %s (%d songs)", playlist.Name, len(playlist.Songs))
	}

	go func() {
		for _, song := range playlist.Songs {
			if err := s.cacheSongSafely(ctx, song); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache playlist song %s: %v", song.Name, err)
			}
		}

		if err := s.storage.SavePlaylist(ctx, playlist); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to cache playlist %s: %v", playlist.Name, err)
		}
	}()

	return playlist, nil
}

func (s *MusicService) SearchAll(ctx context.Context, query string) (*types.SearchResponse, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] SearchAll: '%s'", query)
	}

	result, err := s.api.SearchAll(ctx, query)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API search failed, using local search: %v", err)
		}

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

	if s.debug {
		log.Printf("[MUSIC_SERVICE] API search returned - Songs: %d, Albums: %d, Authors: %d",
			len(result.Songs), len(result.Albums), len(result.Authors))
	}

	return result, nil
}
