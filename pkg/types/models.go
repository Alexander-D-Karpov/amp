package types

import (
	"time"
)

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

type User struct {
	ID           int     `json:"id"`
	Username     string  `json:"username"`
	Email        string  `json:"email"`
	ImageCropped *string `json:"image_cropped"`
	URL          string  `json:"url"`
}

type Meta struct {
	Genre       *string    `json:"genre"`
	Lyrics      *string    `json:"lyrics"`
	Release     *time.Time `json:"release"`
	Explicit    *bool      `json:"explicit"`
	TrackSource *string    `json:"track_source"`
}

type SongListResponse struct {
	Count    int     `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []*Song `json:"results"`
}

type AlbumListResponse struct {
	Count    int      `json:"count"`
	Next     *string  `json:"next"`
	Previous *string  `json:"previous"`
	Results  []*Album `json:"results"`
}

type AuthorListResponse struct {
	Count    int       `json:"count"`
	Next     *string   `json:"next"`
	Previous *string   `json:"previous"`
	Results  []*Author `json:"results"`
}

type SearchResponse struct {
	Songs   []*Song   `json:"songs"`
	Albums  []*Album  `json:"albums"`
	Authors []*Author `json:"authors"`
}

type SearchResults struct {
	Songs   []*Song   `json:"songs"`
	Albums  []*Album  `json:"albums"`
	Authors []*Author `json:"authors"`
	Total   int       `json:"total"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

type PlayHistory struct {
	ID        int64     `db:"id"`
	SongSlug  string    `db:"song_slug"`
	UserID    *string   `db:"user_id"`
	PlayedAt  time.Time `db:"played_at"`
	Synced    bool      `db:"synced"`
	CreatedAt time.Time `db:"created_at"`
}

type DownloadItem struct {
	URL         string     `db:"url"`
	LocalPath   string     `db:"local_path"`
	Progress    float64    `db:"progress"`
	Status      string     `db:"status"`
	Error       *string    `db:"error"`
	CreatedAt   time.Time  `db:"created_at"`
	CompletedAt *time.Time `db:"completed_at"`
}

type CacheEntry struct {
	Key        string    `db:"key"`
	URL        string    `db:"url"`
	LocalPath  string    `db:"local_path"`
	Size       int64     `db:"size"`
	AccessedAt time.Time `db:"accessed_at"`
	CreatedAt  time.Time `db:"created_at"`
}
