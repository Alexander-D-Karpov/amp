package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Manager struct {
	cfg               *config.Config
	mu                sync.RWMutex
	downloads         map[string]*types.DownloadProgress
	activeDownloads   map[string]context.CancelFunc
	semaphore         chan struct{}
	progressCallbacks []func(*types.DownloadProgress)
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:             cfg,
		downloads:       make(map[string]*types.DownloadProgress),
		activeDownloads: make(map[string]context.CancelFunc),
		semaphore:       make(chan struct{}, cfg.Download.MaxConcurrent),
	}
}

func (m *Manager) Download(ctx context.Context, url, destination string) error {
	return m.downloadFile(ctx, url, destination, "")
}

func (m *Manager) DownloadSong(ctx context.Context, song *types.Song) error {
	filename := fmt.Sprintf("%s.mp3", song.Slug)
	destination := filepath.Join(m.cfg.Storage.CacheDir, "songs", filename)

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	err := m.downloadFile(ctx, song.File, destination, song.Name)
	if err == nil {
		song.LocalPath = &destination
		song.Downloaded = true
	}
	return err
}

func (m *Manager) downloadFile(ctx context.Context, url, destination, title string) error {
	m.mu.RLock()
	if progress, exists := m.downloads[url]; exists {
		if progress.Status == types.DownloadStatusDownloading {
			m.mu.RUnlock()
			return fmt.Errorf("already downloading: %s", url)
		}
	}
	m.mu.RUnlock()

	progress := &types.DownloadProgress{
		URL:        url,
		Filename:   title,
		Status:     types.DownloadStatusPending,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}

	m.mu.Lock()
	m.downloads[url] = progress
	m.mu.Unlock()

	go m.performDownload(ctx, url, destination, progress)

	return nil
}

func (m *Manager) performDownload(ctx context.Context, url, destination string, progress *types.DownloadProgress) {
	m.semaphore <- struct{}{}
	defer func() { <-m.semaphore }()

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.mu.Lock()
	m.activeDownloads[url] = cancel
	progress.Status = types.DownloadStatusDownloading
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.activeDownloads, url)
		m.mu.Unlock()
		m.notifyProgress(progress)
	}()

	req, err := http.NewRequestWithContext(downloadCtx, "GET", url, nil)
	if err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("create request: %w", err)
		return
	}

	client := &http.Client{
		Timeout: time.Minute * 10,
	}

	resp, err := client.Do(req)
	if err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("http request: %w", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		return
	}

	progress.Total = resp.ContentLength

	tempFile := destination + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("create file: %w", err)
		return
	}
	defer file.Close()

	buffer := make([]byte, m.cfg.Download.ChunkSize)
	lastProgressUpdate := time.Now()

	for {
		select {
		case <-downloadCtx.Done():
			os.Remove(tempFile)
			progress.Status = types.DownloadStatusCancelled
			progress.Error = downloadCtx.Err()
			return
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				os.Remove(tempFile)
				progress.Status = types.DownloadStatusFailed
				progress.Error = fmt.Errorf("write file: %w", writeErr)
				return
			}

			progress.Downloaded += int64(n)

			now := time.Now()
			if now.Sub(lastProgressUpdate) >= time.Millisecond*100 {
				m.updateProgress(progress, now)
				lastProgressUpdate = now
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			os.Remove(tempFile)
			progress.Status = types.DownloadStatusFailed
			progress.Error = fmt.Errorf("read response: %w", err)
			return
		}
	}

	if err := os.Rename(tempFile, destination); err != nil {
		os.Remove(tempFile)
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("move file: %w", err)
		return
	}

	progress.Status = types.DownloadStatusCompleted
	progress.Progress = 100.0
	m.updateProgress(progress, time.Now())
}

func (m *Manager) updateProgress(progress *types.DownloadProgress, now time.Time) {
	if progress.Total > 0 {
		progress.Progress = float64(progress.Downloaded) / float64(progress.Total) * 100
	}

	elapsed := now.Sub(progress.StartTime).Seconds()
	if elapsed > 0 {
		progress.Speed = float64(progress.Downloaded) / elapsed
	}

	progress.LastUpdate = now
	m.notifyProgress(progress)
}

func (m *Manager) notifyProgress(progress *types.DownloadProgress) {
	for _, callback := range m.progressCallbacks {
		callback(progress)
	}
}

func (m *Manager) GetProgress(url string) (*types.DownloadProgress, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	progress, exists := m.downloads[url]
	return progress, exists
}

func (m *Manager) Cancel(url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.activeDownloads[url]; exists {
		cancel()
		return nil
	}

	return fmt.Errorf("download not found or not active: %s", url)
}

func (m *Manager) GetAllDownloads() []*types.DownloadProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	downloads := make([]*types.DownloadProgress, 0, len(m.downloads))
	for _, progress := range m.downloads {
		downloads = append(downloads, progress)
	}

	return downloads
}

func (m *Manager) OnProgress(callback func(*types.DownloadProgress)) {
	m.progressCallbacks = append(m.progressCallbacks, callback)
}

func (m *Manager) SetMaxConcurrent(max int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.semaphore = make(chan struct{}, max)
}

func (m *Manager) ClearCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for url, progress := range m.downloads {
		if progress.Status == types.DownloadStatusCompleted || progress.Status == types.DownloadStatusFailed {
			delete(m.downloads, url)
		}
	}
}
