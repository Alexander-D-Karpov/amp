package download

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Manager struct {
	config        *Config
	httpClient    *http.Client
	semaphore     chan struct{}
	tasks         sync.Map
	activeStreams sync.Map
	progressCbs   []ProgressCallback
	completionCbs []CompletionCallback
	callbackMutex sync.RWMutex
	debug         bool
}

func NewManager(cfg *config.Config) *Manager {
	downloadConfig := &Config{
		MaxConcurrent: cfg.Download.MaxConcurrent,
		ChunkSize:     cfg.Download.ChunkSize,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
		Timeout:       time.Minute * 10,
		UserAgent:     cfg.API.UserAgent,
		TempDir:       cfg.Download.TempDir,
		CacheDir:      cfg.Storage.CacheDir,
	}

	manager := &Manager{
		config:    downloadConfig,
		semaphore: make(chan struct{}, downloadConfig.MaxConcurrent),
		httpClient: &http.Client{
			Timeout: downloadConfig.Timeout,
		},
		debug: cfg.Debug,
	}

	if err := os.MkdirAll(downloadConfig.TempDir, 0755); err != nil {
		log.Printf("[DOWNLOAD] Failed to create temp directory: %v", err)
	}
	if err := os.MkdirAll(downloadConfig.CacheDir, 0755); err != nil {
		log.Printf("[DOWNLOAD] Failed to create cache directory: %v", err)
	}

	manager.debugLog("Download manager initialized - max concurrent: %d", downloadConfig.MaxConcurrent)
	return manager
}

func (m *Manager) Download(ctx context.Context, url, destination string) error {
	return m.downloadWithOptions(ctx, url, destination, "", nil)
}

func (m *Manager) DownloadSong(ctx context.Context, song *types.Song) error {
	if song == nil {
		return fmt.Errorf("song cannot be nil")
	}

	filename := m.generateSafeFilename(song.Name, song.Slug) + ".mp3"
	destination := filepath.Join(m.config.CacheDir, "songs", filename)

	if stat, err := os.Stat(destination); err == nil && stat.Size() > 0 {
		m.debugLog("Song already in cache: %s", destination)
		song.LocalPath = &destination
		song.Downloaded = true
		return nil
	}

	if song.Downloaded && song.LocalPath != nil {
		if _, err := os.Stat(*song.LocalPath); err == nil {
			m.debugLog("Song metadata indicates already downloaded: %s", *song.LocalPath)
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	return m.downloadWithOptions(ctx, song.File, destination, song.Name, song)
}

func (m *Manager) downloadWithOptions(ctx context.Context, url, destination, title string, song *types.Song) error {
	taskID := m.generateTaskID(url, destination)

	if existingTask, exists := m.tasks.Load(taskID); exists {
		task := existingTask.(*Task)
		task.mutex.RLock()
		state := task.State
		task.mutex.RUnlock()

		if state == StateDownloading || state == StatePending {
			m.debugLog("Download already in progress: %s", url)
			return fmt.Errorf("download already in progress")
		}
	}

	taskCtx, cancel := context.WithCancel(ctx)
	task := &Task{
		ID:          taskID,
		URL:         url,
		Destination: destination,
		Title:       title,
		State:       StatePending,
		Progress:    &Progress{},
		StartTime:   time.Now(),
		CancelFunc:  cancel,
		MaxRetries:  m.config.RetryAttempts,
		Song:        song,
	}

	m.tasks.Store(taskID, task)
	m.debugLog("Created download task: %s -> %s", url, destination)

	go m.executeDownload(taskCtx, task)

	return nil
}

func (m *Manager) executeDownload(ctx context.Context, task *Task) {
	select {
	case m.semaphore <- struct{}{}:
		defer func() { <-m.semaphore }()
	case <-ctx.Done():
		m.updateTaskState(task, StateCancelled, ctx.Err())
		return
	}

	m.updateTaskState(task, StateDownloading, nil)
	m.debugLog("Starting download: %s", task.URL)

	var lastErr error
	for attempt := 0; attempt <= task.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * m.config.RetryDelay
			m.debugLog("Retrying download (attempt %d/%d) after %v: %s",
				attempt+1, task.MaxRetries+1, delay, task.URL)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				m.updateTaskState(task, StateCancelled, ctx.Err())
				return
			}
		}

		err := m.performDownload(ctx, task)
		if err == nil {
			m.handleDownloadSuccess(task)
			return
		}

		lastErr = err
		task.mutex.Lock()
		task.Retries = attempt
		task.mutex.Unlock()

		if !m.shouldRetry(err) {
			break
		}
	}

	m.updateTaskState(task, StateFailed, lastErr)
	m.debugLog("Download failed after %d attempts: %s - %v", task.MaxRetries+1, task.URL, lastErr)
}

func (m *Manager) GetProgress(url string) (*types.DownloadProgress, bool) {
	var foundTask *Task
	m.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		if task.URL == url {
			foundTask = task
			return false
		}
		return true
	})

	if foundTask == nil {
		return nil, false
	}

	return m.taskToProgress(foundTask), true
}

func (m *Manager) Cancel(url string) error {
	var foundTask *Task
	m.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		if task.URL == url {
			foundTask = task
			return false
		}
		return true
	})

	if foundTask == nil {
		return fmt.Errorf("download not found: %s", url)
	}

	foundTask.mutex.Lock()
	if foundTask.CancelFunc != nil {
		foundTask.CancelFunc()
	}
	foundTask.mutex.Unlock()

	m.updateTaskState(foundTask, StateCancelled, fmt.Errorf("cancelled by user"))
	m.debugLog("Cancelled download: %s", url)
	return nil
}

func (m *Manager) GetAllDownloads() []*types.DownloadProgress {
	var downloads []*types.DownloadProgress

	m.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		downloads = append(downloads, m.taskToProgress(task))
		return true
	})

	return downloads
}

func (m *Manager) OnProgress(callback ProgressCallback) {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()
	m.progressCbs = append(m.progressCbs, callback)
}

func (m *Manager) OnCompletion(callback CompletionCallback) {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()
	m.completionCbs = append(m.completionCbs, callback)
}

func (m *Manager) SetMaxConcurrent(max int) {
	m.config.MaxConcurrent = max
	m.semaphore = make(chan struct{}, max)
	m.debugLog("Updated max concurrent downloads: %d", max)
}

func (m *Manager) ClearCompleted() {
	var toDelete []string

	m.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		task.mutex.RLock()
		state := task.State
		task.mutex.RUnlock()

		if state == StateCompleted || state == StateFailed {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, key := range toDelete {
		m.tasks.Delete(key)
	}

	m.debugLog("Cleared %d completed downloads", len(toDelete))
}

func (m *Manager) generateTaskID(url, destination string) string {
	hash := sha256.Sum256([]byte(url + destination))
	return fmt.Sprintf("%x", hash)[:16]
}

func (m *Manager) generateSafeFilename(name, slug string) string {
	if slug != "" {
		return slug
	}

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

func (m *Manager) debugLog(format string, args ...interface{}) {
	if m.debug {
		log.Printf("[DOWNLOAD] "+format, args...)
	}
}
