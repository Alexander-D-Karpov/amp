package download

import (
	"context"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

// State represents the current state of a download operation
type State int

const (
	StatePending State = iota
	StateDownloading
	StateCompleted
	StateFailed
	StateCancelled
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "Pending"
	case StateDownloading:
		return "Downloading"
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// Task represents a single download operation with all its metadata
type Task struct {
	ID          string
	URL         string
	Destination string
	Title       string
	State       State
	Progress    *Progress
	Error       error
	StartTime   time.Time
	CompletedAt *time.Time
	CancelFunc  context.CancelFunc
	Retries     int
	MaxRetries  int
	Song        *types.Song

	mutex sync.RWMutex
}

// Progress tracks download progress and statistics
type Progress struct {
	Total      int64
	Downloaded int64
	Percentage float64
	Speed      float64
	ETA        time.Duration
	LastUpdate time.Time

	mutex sync.RWMutex
}

// Config holds configuration for the download manager
type Config struct {
	MaxConcurrent int
	ChunkSize     int
	RetryAttempts int
	RetryDelay    time.Duration
	Timeout       time.Duration
	UserAgent     string
	TempDir       string
	CacheDir      string
}

// ProgressCallback is called when download progress updates
type ProgressCallback func(*types.DownloadProgress)

// CompletionCallback is called when a download completes or fails
type CompletionCallback func(*Task)

// activeDownload tracks an ongoing download to prevent duplicates
type activeDownload struct {
	task      *Task
	startTime time.Time
}
