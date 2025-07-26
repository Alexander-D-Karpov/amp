package search

import (
	"context"
	"sort"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Engine struct {
	cfg     *config.Config
	storage types.Storage
}

func NewEngine(cfg *config.Config, storage types.Storage) *Engine {
	return &Engine{
		cfg:     cfg,
		storage: storage,
	}
}

func (e *Engine) Search(ctx context.Context, query string, limit int) (*types.SearchResults, error) {
	if query == "" {
		return &types.SearchResults{}, nil
	}

	songs, err := e.storage.SearchSongs(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	fuzzyResults, err := e.FuzzySearch(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	results := &types.SearchResults{
		Songs: mergeSongs(songs, fuzzyResults.Songs),
		Total: len(songs),
	}

	if len(results.Songs) > limit {
		results.Songs = results.Songs[:limit]
	}

	return results, nil
}

func (e *Engine) FuzzySearch(ctx context.Context, query string, limit int) (*types.SearchResults, error) {
	songs, err := e.storage.GetSongs(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	albums, err := e.storage.GetAlbums(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	authors, err := e.storage.GetAuthors(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	songResults := e.fuzzySearchSongs(songs, query)
	albumResults := e.fuzzySearchAlbums(albums, query)
	authorResults := e.fuzzySearchAuthors(authors, query)

	results := &types.SearchResults{
		Songs:   songResults,
		Albums:  albumResults,
		Authors: authorResults,
		Total:   len(songResults) + len(albumResults) + len(authorResults),
	}

	return results, nil
}

type ScoredSong struct {
	Song  *types.Song
	Score float64
}

type ScoredAlbum struct {
	Album *types.Album
	Score float64
}

type ScoredAuthor struct {
	Author *types.Author
	Score  float64
}

func (e *Engine) fuzzySearchSongs(songs []*types.Song, query string) []*types.Song {
	var scored []ScoredSong
	queryLower := strings.ToLower(query)

	for _, song := range songs {
		score := 0.0

		if strings.Contains(strings.ToLower(song.Name), queryLower) {
			score += 10.0
		}

		distance := fuzzy.LevenshteinDistance(queryLower, strings.ToLower(song.Name))
		if distance <= len(queryLower)/2 {
			score += float64(len(queryLower) - distance)
		}

		if song.Album != nil {
			if strings.Contains(strings.ToLower(song.Album.Name), queryLower) {
				score += 5.0
			}
		}

		for _, author := range song.Authors {
			if strings.Contains(strings.ToLower(author.Name), queryLower) {
				score += 7.0
			}
		}

		if score > 0 {
			scored = append(scored, ScoredSong{Song: song, Score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	result := make([]*types.Song, 0, len(scored))
	for _, s := range scored {
		result = append(result, s.Song)
	}

	return result
}

func (e *Engine) fuzzySearchAlbums(albums []*types.Album, query string) []*types.Album {
	var scored []ScoredAlbum
	queryLower := strings.ToLower(query)

	for _, album := range albums {
		score := 0.0

		if strings.Contains(strings.ToLower(album.Name), queryLower) {
			score += 10.0
		}

		distance := fuzzy.LevenshteinDistance(queryLower, strings.ToLower(album.Name))
		if distance <= len(queryLower)/2 {
			score += float64(len(queryLower) - distance)
		}

		if score > 0 {
			scored = append(scored, ScoredAlbum{Album: album, Score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	result := make([]*types.Album, 0, len(scored))
	for _, s := range scored {
		result = append(result, s.Album)
	}

	return result
}

func (e *Engine) fuzzySearchAuthors(authors []*types.Author, query string) []*types.Author {
	var scored []ScoredAuthor
	queryLower := strings.ToLower(query)

	for _, author := range authors {
		score := 0.0

		if strings.Contains(strings.ToLower(author.Name), queryLower) {
			score += 10.0
		}

		distance := fuzzy.LevenshteinDistance(queryLower, strings.ToLower(author.Name))
		if distance <= len(queryLower)/2 {
			score += float64(len(queryLower) - distance)
		}

		if score > 0 {
			scored = append(scored, ScoredAuthor{Author: author, Score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	result := make([]*types.Author, 0, len(scored))
	for _, s := range scored {
		result = append(result, s.Author)
	}

	return result
}

func mergeSongs(songs1, songs2 []*types.Song) []*types.Song {
	seen := make(map[string]bool)
	var result []*types.Song

	for _, song := range songs1 {
		if !seen[song.Slug] {
			result = append(result, song)
			seen[song.Slug] = true
		}
	}

	for _, song := range songs2 {
		if !seen[song.Slug] {
			result = append(result, song)
			seen[song.Slug] = true
		}
	}

	return result
}
