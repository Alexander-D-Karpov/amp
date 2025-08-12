package types

import (
	"context"
	"fyne.io/fyne/v2"
	"time"
)

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

type DownloadProgress struct {
	URL        string         `json:"url"`
	Filename   string         `json:"filename"`
	Total      int64          `json:"total"`
	Downloaded int64          `json:"downloaded"`
	Progress   float64        `json:"progress"`
	Speed      float64        `json:"speed"`
	Status     DownloadStatus `json:"status"`
	Error      error          `json:"error,omitempty"`
	StartTime  time.Time      `json:"start_time"`
	LastUpdate time.Time      `json:"last_update"`
	ETA        time.Duration  `json:"eta"`
}

// DownloadStatus represents the status of a download
type DownloadStatus int

const (
	DownloadStatusPending DownloadStatus = iota
	DownloadStatusDownloading
	DownloadStatusCompleted
	DownloadStatusFailed
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
		return "Cancelled"
	default:
		return "Unknown"
	}
}
