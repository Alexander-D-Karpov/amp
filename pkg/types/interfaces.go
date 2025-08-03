package types

import (
	"context"
	"io"
	"time"

	"fyne.io/fyne/v2"
)

// APIClient defines the interface for interacting with the music API
type APIClient interface {
	Authenticate(ctx context.Context, token string) error
	GetSongs(ctx context.Context, page int, search string) (*SongListResponse, error)
	GetSong(ctx context.Context, slug string) (*Song, error)
	GetAlbums(ctx context.Context, page int, search string) (*AlbumListResponse, error)
	GetAlbum(ctx context.Context, slug string) (*Album, error)
	GetAuthors(ctx context.Context, page int, search string) (*AuthorListResponse, error)
	GetAuthor(ctx context.Context, slug string) (*Author, error)
	GetPlaylists(ctx context.Context) ([]*Playlist, error)
	GetPlaylist(ctx context.Context, slug string) (*Playlist, error)
	CreatePlaylist(ctx context.Context, playlist *Playlist) error
	UpdatePlaylist(ctx context.Context, playlist *Playlist) error
	DeletePlaylist(ctx context.Context, slug string) error
	LikeSong(ctx context.Context, slug string) error
	DislikeSong(ctx context.Context, slug string) error
	ListenSong(ctx context.Context, slug string, userID string) error
	SearchAll(ctx context.Context, query string) (*SearchResponse, error)
}

// Storage defines the interface for local data persistence
type Storage interface {
	GetSongs(ctx context.Context, limit, offset int) ([]*Song, error)
	GetSong(ctx context.Context, slug string) (*Song, error)
	SaveSong(ctx context.Context, song *Song) error
	DeleteSong(ctx context.Context, slug string) error
	SearchSongs(ctx context.Context, query string, limit int) ([]*Song, error)

	GetAlbums(ctx context.Context, limit, offset int) ([]*Album, error)
	GetAlbum(ctx context.Context, slug string) (*Album, error)
	SaveAlbum(ctx context.Context, album *Album) error

	GetAuthors(ctx context.Context, limit, offset int) ([]*Author, error)
	GetAuthor(ctx context.Context, slug string) (*Author, error)
	SaveAuthor(ctx context.Context, author *Author) error

	GetPlaylists(ctx context.Context) ([]*Playlist, error)
	GetPlaylist(ctx context.Context, slug string) (*Playlist, error)
	SavePlaylist(ctx context.Context, playlist *Playlist) error
	DeletePlaylist(ctx context.Context, slug string) error

	GetCachedFile(ctx context.Context, url string) (string, error)
	SaveCachedFile(ctx context.Context, url string, data io.Reader) (string, error)

	Close() error
}

// AudioPlayer defines the interface for audio playback control
type AudioPlayer interface {
	Play(ctx context.Context, song *Song) error
	Pause() error
	Resume() error
	Stop() error
	Seek(position time.Duration) error
	SetVolume(volume float64) error
	GetPosition() time.Duration
	GetDuration() time.Duration
	IsPlaying() bool
	OnPositionChanged(callback func(time.Duration))
	OnFinished(callback func())
	Close() error
}

// SearchEngine defines the interface for searching music content
type SearchEngine interface {
	Search(ctx context.Context, query string, limit int) (*SearchResults, error)
	FuzzySearch(ctx context.Context, query string, limit int) (*SearchResults, error)
}

// DownloadManager defines the interface for managing file downloads
type DownloadManager interface {
	Download(ctx context.Context, url, destination string) error
	DownloadSong(ctx context.Context, song *Song) error
	GetProgress(url string) (*DownloadProgress, bool)
	Cancel(url string) error
	SetMaxConcurrent(max int)
	GetAllDownloads() []*DownloadProgress
	OnProgress(callback func(*DownloadProgress))
	ClearCompleted()
}

// SyncManager defines the interface for synchronizing data with remote sources
type SyncManager interface {
	Sync(ctx context.Context) error
	SyncSongs(ctx context.Context) error
	SyncPlaylists(ctx context.Context) error
	SyncPlayCounts(ctx context.Context) error
	SyncLikes(ctx context.Context) error
	SetInterval(interval time.Duration)
	Start(ctx context.Context)
	Stop()
}

// View defines the interface for UI components
type View interface {
	Container() *fyne.Container
	Refresh()
}

// PlayerControl defines the interface for player control components
type PlayerControl interface {
	Play()
	Pause()
	Stop()
	Next()
	Previous()
	SetVolume(float64)
	Seek(time.Duration)
}

// DownloadProgress tracks the progress of a file download
type DownloadProgress struct {
	URL        string
	Filename   string
	Total      int64
	Downloaded int64
	Progress   float64
	Speed      float64
	Status     DownloadStatus
	Error      error
	StartTime  time.Time
	LastUpdate time.Time
}

// DownloadStatus represents the current status of a download
type DownloadStatus int

const (
	// DownloadStatusPending indicates the download is queued but not started
	DownloadStatusPending DownloadStatus = iota
	// DownloadStatusDownloading indicates the download is in progress
	DownloadStatusDownloading
	// DownloadStatusCompleted indicates the download finished successfully
	DownloadStatusCompleted
	// DownloadStatusFailed indicates the download failed
	DownloadStatusFailed
	// DownloadStatusCancelled indicates the download was canceled
	DownloadStatusCancelled
)

func (s DownloadStatus) String() string {
	switch s {
	case DownloadStatusPending:
		return "Pending"
	case DownloadStatusDownloading:
		return "Downloading"
	case DownloadStatusCompleted:
		return "Completed"
	case DownloadStatusFailed:
		return "Failed"
	case DownloadStatusCancelled:
		return "Canceled"
	default:
		return "Unknown"
	}
}
