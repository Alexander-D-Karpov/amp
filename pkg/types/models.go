package types

import (
	"time"
)

// Song represents a music track with metadata and playback information
type Song struct {
	Slug         string    `json:"slug" db:"slug"`
	Name         string    `json:"name" db:"name"`
	File         string    `json:"file" db:"file"`
	Image        *string   `json:"image" db:"image"`
	ImageCropped *string   `json:"image_cropped" db:"image_cropped"`
	Length       int       `json:"length" db:"length"`
	Played       int       `json:"played" db:"played"`
	Link         string    `json:"link" db:"link"`
	Liked        *bool     `json:"liked" db:"liked"`
	Volume       []int     `json:"volume" db:"volume"`
	Album        *Album    `json:"album" db:"-"`
	Authors      []*Author `json:"authors" db:"-"`
	AlbumSlug    string    `json:"-" db:"album_slug"`
	Meta         *Meta     `json:"meta" db:"-"`

	LocalPath  *string   `json:"-" db:"local_path"`
	Downloaded bool      `json:"-" db:"downloaded"`
	LastSync   time.Time `json:"-" db:"last_sync"`
	CreatedAt  time.Time `json:"-" db:"created_at"`
	UpdatedAt  time.Time `json:"-" db:"updated_at"`
}

// Album represents a music album containing multiple songs
type Album struct {
	Slug         string    `json:"slug" db:"slug"`
	Name         string    `json:"name" db:"name"`
	Image        *string   `json:"image" db:"image"`
	ImageCropped *string   `json:"image_cropped" db:"image_cropped"`
	Link         string    `json:"link" db:"link"`
	Songs        []*Song   `json:"songs" db:"-"`
	Artists      []*Author `json:"artists" db:"-"`
	Meta         *Meta     `json:"meta" db:"-"`

	LastSync  time.Time `json:"-" db:"last_sync"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
}

// Author represents a music artist or author
type Author struct {
	Slug         string   `json:"slug" db:"slug"`
	Name         string   `json:"name" db:"name"`
	Image        *string  `json:"image" db:"image"`
	ImageCropped *string  `json:"image_cropped" db:"image_cropped"`
	Link         string   `json:"link" db:"link"`
	Songs        []*Song  `json:"songs" db:"-"`
	Albums       []*Album `json:"albums" db:"-"`
	Meta         *Meta    `json:"meta" db:"-"`

	LastSync  time.Time `json:"-" db:"last_sync"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
}

// Playlist represents a collection of songs organized by a user
type Playlist struct {
	Slug    string   `json:"slug" db:"slug"`
	Name    string   `json:"name" db:"name"`
	Private bool     `json:"private" db:"private"`
	Creator *User    `json:"creator" db:"-"`
	Images  []string `json:"images" db:"-"`
	Songs   []*Song  `json:"songs" db:"-"`
	Length  int      `json:"length" db:"length"`

	LocalOnly bool      `json:"-" db:"local_only"`
	LastSync  time.Time `json:"-" db:"last_sync"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
}

// User represents a user account in the music system
type User struct {
	ID           int     `json:"id"`
	Username     string  `json:"username"`
	Email        string  `json:"email"`
	ImageCropped *string `json:"image_cropped"`
	URL          string  `json:"url"`
}

// Meta contains additional metadata about music tracks
type Meta struct {
	Genre       *string    `json:"genre"`
	Lyrics      *string    `json:"lyrics"`
	Release     *time.Time `json:"release"`
	Explicit    *bool      `json:"explicit"`
	TrackSource *string    `json:"track_source"`
}

// SongListResponse represents a paginated list of songs from the API
type SongListResponse struct {
	Count    int     `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []*Song `json:"results"`
}

// AlbumListResponse represents a paginated list of albums from the API
type AlbumListResponse struct {
	Count    int      `json:"count"`
	Next     *string  `json:"next"`
	Previous *string  `json:"previous"`
	Results  []*Album `json:"results"`
}

// AuthorListResponse represents a paginated list of authors from the API
type AuthorListResponse struct {
	Count    int       `json:"count"`
	Next     *string   `json:"next"`
	Previous *string   `json:"previous"`
	Results  []*Author `json:"results"`
}

// SearchResponse represents search results from the API
type SearchResponse struct {
	Songs   []*Song   `json:"songs"`
	Albums  []*Album  `json:"albums"`
	Authors []*Author `json:"authors"`
}

// SearchResults represents local search results
type SearchResults struct {
	Songs   []*Song   `json:"songs"`
	Albums  []*Album  `json:"albums"`
	Authors []*Author `json:"authors"`
	Total   int       `json:"total"`
}

// AuthResponse represents an authentication response from the API
type AuthResponse struct {
	Token string `json:"token"`
}

// PlayHistory represents a record of song playback
type PlayHistory struct {
	ID        int64     `db:"id"`
	SongSlug  string    `db:"song_slug"`
	UserID    *string   `db:"user_id"`
	PlayedAt  time.Time `db:"played_at"`
	Synced    bool      `db:"synced"`
	CreatedAt time.Time `db:"created_at"`
}

// DownloadItem represents a download task
type DownloadItem struct {
	URL         string     `db:"url"`
	LocalPath   string     `db:"local_path"`
	Progress    float64    `db:"progress"`
	Status      string     `db:"status"`
	Error       *string    `db:"error"`
	CreatedAt   time.Time  `db:"created_at"`
	CompletedAt *time.Time `db:"completed_at"`
}

// CacheEntry represents a cached file entry
type CacheEntry struct {
	Key        string    `db:"key"`
	URL        string    `db:"url"`
	LocalPath  string    `db:"local_path"`
	Size       int64     `db:"size"`
	AccessedAt time.Time `db:"accessed_at"`
	CreatedAt  time.Time `db:"created_at"`
}
