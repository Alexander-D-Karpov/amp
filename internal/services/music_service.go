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
		debug:   false,
	}
}

func (s *MusicService) GetStorage() *storage.Database {
	return s.storage
}

// LIST METHODS - Only fetch basic information, no automatic enrichment

func (s *MusicService) GetSongs(ctx context.Context, page int, searchQuery string) ([]*types.Song, bool, error) {
	return s.GetSongsWithSort(ctx, page, searchQuery, api.SortDefault)
}

func (s *MusicService) GetSongsWithSort(ctx context.Context, page int, searchQuery string, sortOption api.SortOption) ([]*types.Song, bool, error) {
	if searchQuery != "" {
		// Try API first for search
		resp, err := s.api.GetSongsWithSort(ctx, page, searchQuery, sortOption)
		if err != nil {
			// Fallback to local search
			results, searchErr := s.search.Search(ctx, searchQuery, 100)
			if searchErr != nil {
				return nil, false, fmt.Errorf("search failed: %w", searchErr)
			}
			return results.Songs, false, nil
		}

		// Cache songs in background without fetching additional details
		go s.cacheSongsBasic(ctx, resp.Results)
		return resp.Results, resp.Next != nil, nil
	}

	// No search query - get regular list
	resp, err := s.api.GetSongsWithSort(ctx, page, "", sortOption)
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

	// Cache songs in background without fetching additional details
	go s.cacheSongsBasic(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
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

	// Cache albums in background (basic info only)
	go s.cacheAlbumsBasic(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
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

	// Cache authors in background (basic info only)
	go s.cacheAuthorsBasic(ctx, resp.Results)
	return resp.Results, resp.Next != nil, nil
}

func (s *MusicService) GetPlaylists(ctx context.Context) ([]*types.Playlist, error) {
	playlists, err := s.api.GetPlaylists(ctx)
	if err != nil {
		dbPlaylists, dbErr := s.storage.GetPlaylists(ctx)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}
		return dbPlaylists, nil
	}

	// Cache playlists in background (basic info only)
	go s.cachePlaylistsBasic(ctx, playlists)
	return playlists, nil
}

// DETAILED METHODS - Fetch full information with relationships when explicitly requested

func (s *MusicService) GetAlbum(ctx context.Context, slug string) (*types.Album, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] Fetching detailed album: %s", slug)
	}

	// Try API first for detailed album info
	album, err := s.api.GetAlbum(ctx, slug)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API call failed for album %s: %v", slug, err)
		}

		// Fallback to storage with manual relationship loading
		dbAlbum, dbErr := s.storage.GetAlbum(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		// Load songs for the album from storage
		if dbAlbum != nil {
			songs, songErr := s.getAlbumSongsFromStorage(ctx, slug)
			if songErr == nil {
				dbAlbum.Songs = songs
			}
		}

		return dbAlbum, nil
	}

	if album != nil {
		// Cache the detailed album and its relationships
		go s.cacheAlbumWithRelationships(ctx, album)

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Retrieved album: %s with %d songs", album.Name, len(album.Songs))
		}
	}

	return album, nil
}

func (s *MusicService) GetAuthor(ctx context.Context, slug string) (*types.Author, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] Fetching detailed author: %s", slug)
	}

	// Try API first for detailed author info
	author, err := s.api.GetAuthor(ctx, slug)
	if err != nil {
		if s.debug {
			log.Printf("[MUSIC_SERVICE] API call failed for author %s: %v", slug, err)
		}

		// Fallback to storage with manual relationship loading
		dbAuthor, dbErr := s.storage.GetAuthor(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}

		// Load songs and albums for the author from storage
		if dbAuthor != nil {
			songs, albums := s.getAuthorContentFromStorage(ctx, slug)
			dbAuthor.Songs = songs
			dbAuthor.Albums = albums
		}

		return dbAuthor, nil
	}

	if author != nil {
		// Cache the detailed author and their content
		go s.cacheAuthorWithRelationships(ctx, author)

		if s.debug {
			log.Printf("[MUSIC_SERVICE] Retrieved author: %s with %d songs and %d albums",
				author.Name, len(author.Songs), len(author.Albums))
		}
	}

	return author, nil
}

func (s *MusicService) GetSong(ctx context.Context, slug string) (*types.Song, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] Fetching detailed song: %s", slug)
	}

	// Try API first
	song, err := s.api.GetSong(ctx, slug)
	if err != nil {
		// Fallback to storage
		dbSong, dbErr := s.storage.GetSong(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}
		return dbSong, nil
	}

	if song != nil {
		// Persist volume if DB lacked it
		go s.ensureSongVolumeSaved(ctx, song)

		go s.cacheSongWithRelationships(ctx, song)
	}

	return song, nil
}

func (s *MusicService) ensureSongVolumeSaved(ctx context.Context, song *types.Song) {
	if song == nil {
		return
	}
	dbSong, _ := s.storage.GetSong(ctx, song.Slug)

	// If API did not send volume but DB has it, keep DB's volume in memory
	if (dbSong != nil && len(dbSong.Volume) > 0) && len(song.Volume) == 0 {
		song.Volume = dbSong.Volume
	}

	// If API has volume and DB is missing (or differs), save
	if len(song.Volume) > 0 && (dbSong == nil || len(dbSong.Volume) == 0) {
		if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to persist song volume for %s: %v", song.Slug, err)
		}
	}
}

func (s *MusicService) GetPlaylist(ctx context.Context, slug string) (*types.Playlist, error) {
	if s.debug {
		log.Printf("[MUSIC_SERVICE] Fetching detailed playlist: %s", slug)
	}

	playlist, err := s.api.GetPlaylist(ctx, slug)
	if err != nil {
		dbPlaylist, dbErr := s.storage.GetPlaylist(ctx, slug)
		if dbErr != nil {
			return nil, fmt.Errorf("both API and storage failed: api=%w, storage=%w", err, dbErr)
		}
		return dbPlaylist, nil
	}

	if playlist != nil {
		// Cache the playlist and its songs
		go s.cachePlaylistWithRelationships(ctx, playlist)
	}

	return playlist, nil
}

// SEARCH METHOD

func (s *MusicService) SearchAll(ctx context.Context, query string) (*types.SearchResponse, error) {
	result, err := s.api.SearchAll(ctx, query)
	if err != nil {
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

	// Cache search results (basic info only)
	if result != nil {
		go func() {
			s.cacheSongsBasic(ctx, result.Songs)
			s.cacheAlbumsBasic(ctx, result.Albums)
			s.cacheAuthorsBasic(ctx, result.Authors)
		}()
	}

	return result, nil
}

// HELPER METHODS FOR STORAGE QUERIES

func (s *MusicService) getAlbumSongsFromStorage(ctx context.Context, albumSlug string) ([]*types.Song, error) {
	songs, err := s.storage.GetSongs(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	var albumSongs []*types.Song
	for _, song := range songs {
		if song.Album != nil && song.Album.Slug == albumSlug {
			albumSongs = append(albumSongs, song)
		}
	}

	return albumSongs, nil
}

func (s *MusicService) getAuthorContentFromStorage(ctx context.Context, authorSlug string) ([]*types.Song, []*types.Album) {
	songs, err := s.storage.GetSongs(ctx, 1000, 0)
	if err != nil {
		return nil, nil
	}

	var authorSongs []*types.Song
	albumMap := make(map[string]*types.Album)

	for _, song := range songs {
		for _, author := range song.Authors {
			if author != nil && author.Slug == authorSlug {
				authorSongs = append(authorSongs, song)
				if song.Album != nil {
					albumMap[song.Album.Slug] = song.Album
				}
				break
			}
		}
	}

	var authorAlbums []*types.Album
	for _, album := range albumMap {
		authorAlbums = append(authorAlbums, album)
	}

	return authorSongs, authorAlbums
}

// BASIC CACHING METHODS (for list views) - No additional API calls

func (s *MusicService) cacheSongsBasic(ctx context.Context, songs []*types.Song) {
	for _, song := range songs {
		if song == nil {
			continue
		}

		// Only cache the song itself and basic relationship info that's already present
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

func (s *MusicService) cacheAlbumsBasic(ctx context.Context, albums []*types.Album) {
	for _, album := range albums {
		if album == nil {
			continue
		}

		// Cache basic album info and any artists that are already present
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
	}
}

func (s *MusicService) cacheAuthorsBasic(ctx context.Context, authors []*types.Author) {
	for _, author := range authors {
		if author != nil {
			if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
			}
		}
	}
}

func (s *MusicService) cachePlaylistsBasic(ctx context.Context, playlists []*types.Playlist) {
	for _, playlist := range playlists {
		if playlist == nil {
			continue
		}

		if err := s.storage.SavePlaylist(ctx, playlist); err != nil && s.debug {
			log.Printf("[MUSIC_SERVICE] Failed to cache playlist %s: %v", playlist.Name, err)
		}
	}
}

// DETAILED CACHING METHODS (for detailed views) - Cache full relationship trees

func (s *MusicService) cacheAlbumWithRelationships(ctx context.Context, album *types.Album) {
	if album == nil {
		return
	}

	// Cache the album
	if err := s.storage.SaveAlbum(ctx, album); err != nil && s.debug {
		log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", album.Name, err)
	}

	// Cache all songs in the album
	for _, song := range album.Songs {
		if song != nil {
			// Ensure song has album reference
			if song.Album == nil {
				song.Album = &types.Album{
					Slug:         album.Slug,
					Name:         album.Name,
					Image:        album.Image,
					ImageCropped: album.ImageCropped,
				}
			}
			if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
			}
		}
	}

	// Cache artists
	for _, artist := range album.Artists {
		if artist != nil {
			if err := s.storage.SaveAuthor(ctx, artist); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache artist %s: %v", artist.Name, err)
			}
		}
	}
}

func (s *MusicService) cacheAuthorWithRelationships(ctx context.Context, author *types.Author) {
	if author == nil {
		return
	}

	// Cache the author
	if err := s.storage.SaveAuthor(ctx, author); err != nil && s.debug {
		log.Printf("[MUSIC_SERVICE] Failed to cache author %s: %v", author.Name, err)
	}

	// Cache author songs
	for _, song := range author.Songs {
		if song != nil {
			if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
			}
		}
	}

	// Cache author albums
	for _, album := range author.Albums {
		if album != nil {
			if err := s.storage.SaveAlbum(ctx, album); err != nil && s.debug {
				log.Printf("[MUSIC_SERVICE] Failed to cache album %s: %v", album.Name, err)
			}
		}
	}
}

func (s *MusicService) cacheSongWithRelationships(ctx context.Context, song *types.Song) {
	if song == nil {
		return
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

	// Cache the song
	if err := s.storage.SaveSong(ctx, song); err != nil && s.debug {
		log.Printf("[MUSIC_SERVICE] Failed to cache song %s: %v", song.Name, err)
	}
}

func (s *MusicService) cachePlaylistWithRelationships(ctx context.Context, playlist *types.Playlist) {
	if playlist == nil {
		return
	}

	// Cache all songs and their relationships
	for _, song := range playlist.Songs {
		if song != nil {
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

	// Cache the playlist
	if err := s.storage.SavePlaylist(ctx, playlist); err != nil && s.debug {
		log.Printf("[MUSIC_SERVICE] Failed to cache playlist %s: %v", playlist.Name, err)
	}
}

func (s *MusicService) SetDebug(debug bool) {
	s.debug = debug
}
