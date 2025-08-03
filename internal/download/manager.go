package download

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	streamReaders     map[string]*StreamReader
	semaphore         chan struct{}
	progressCallbacks []func(*types.DownloadProgress)
	httpClient        *http.Client
}

type StreamReader struct {
	url          string
	buffer       []byte
	position     int64
	totalSize    int64
	mu           sync.RWMutex
	downloadDone bool
	manager      *Manager
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:             cfg,
		downloads:       make(map[string]*types.DownloadProgress),
		activeDownloads: make(map[string]context.CancelFunc),
		streamReaders:   make(map[string]*StreamReader),
		semaphore:       make(chan struct{}, cfg.Download.MaxConcurrent),
		httpClient:      &http.Client{Timeout: time.Minute * 10},
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

func (m *Manager) GetStreamReader(ctx context.Context, url string) (*StreamReader, error) {
	m.mu.RLock()
	if reader, exists := m.streamReaders[url]; exists {
		m.mu.RUnlock()
		return reader, nil
	}
	m.mu.RUnlock()

	reader := &StreamReader{
		url:     url,
		buffer:  make([]byte, 0),
		manager: m,
	}

	m.mu.Lock()
	m.streamReaders[url] = reader
	m.mu.Unlock()

	go m.streamingDownload(ctx, url, reader)

	return reader, nil
}

func (m *Manager) streamingDownload(ctx context.Context, url string, reader *StreamReader) {
	m.semaphore <- struct{}{}
	defer func() { <-m.semaphore }()

	progress := &types.DownloadProgress{
		URL:        url,
		Filename:   filepath.Base(url),
		Status:     types.DownloadStatusDownloading,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}

	m.mu.Lock()
	m.downloads[url] = progress
	m.mu.Unlock()

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.mu.Lock()
	m.activeDownloads[url] = cancel
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.activeDownloads, url)
		m.mu.Unlock()
		reader.mu.Lock()
		reader.downloadDone = true
		reader.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(downloadCtx, "GET", url, nil)
	if err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = err
		m.notifyProgress(progress)
		return
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = err
		m.notifyProgress(progress)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		progress.Status = types.DownloadStatusFailed
		progress.Error = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		m.notifyProgress(progress)
		return
	}

	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			progress.Total = size
			reader.mu.Lock()
			reader.totalSize = size
			reader.mu.Unlock()
		}
	}

	buffer := make([]byte, m.cfg.Download.ChunkSize)
	lastProgressUpdate := time.Now()

	for {
		select {
		case <-downloadCtx.Done():
			progress.Status = types.DownloadStatusCancelled
			progress.Error = downloadCtx.Err()
			m.notifyProgress(progress)
			return
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])

			reader.mu.Lock()
			reader.buffer = append(reader.buffer, chunk...)
			reader.mu.Unlock()

			progress.Downloaded += int64(n)

			now := time.Now()
			if now.Sub(lastProgressUpdate) >= time.Millisecond*100 {
				if progress.Total > 0 {
					progress.Progress = float64(progress.Downloaded) / float64(progress.Total) * 100
				}

				elapsed := now.Sub(progress.StartTime).Seconds()
				if elapsed > 0 {
					progress.Speed = float64(progress.Downloaded) / elapsed
				}

				progress.LastUpdate = now
				m.notifyProgress(progress)
				lastProgressUpdate = now
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			progress.Status = types.DownloadStatusFailed
			progress.Error = fmt.Errorf("read response: %w", err)
			m.notifyProgress(progress)
			return
		}
	}

	progress.Status = types.DownloadStatusCompleted
	progress.Progress = 100.0
	m.notifyProgress(progress)
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

	m.setProgressStatus(url, cancel, types.DownloadStatusDownloading)
	defer m.cleanupActiveDownload(url)

	if err := m.downloadHTTP(downloadCtx, url, destination, progress); err != nil {
		progress.Status = types.DownloadStatusFailed
		progress.Error = err
	} else {
		progress.Status = types.DownloadStatusCompleted
		progress.Progress = 100.0
		m.updateProgress(progress, time.Now())
	}

	m.notifyProgress(progress)
}

func (m *Manager) setProgressStatus(url string, cancel context.CancelFunc, status types.DownloadStatus) {
	m.mu.Lock()
	m.activeDownloads[url] = cancel
	if progress, exists := m.downloads[url]; exists {
		progress.Status = status
	}
	m.mu.Unlock()
}

func (m *Manager) cleanupActiveDownload(url string) {
	m.mu.Lock()
	delete(m.activeDownloads, url)
	m.mu.Unlock()
}

func (m *Manager) downloadHTTP(ctx context.Context, url, destination string, progress *types.DownloadProgress) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return m.writeFile(ctx, resp, destination, progress)
}

func (m *Manager) writeFile(ctx context.Context, resp *http.Response, destination string, progress *types.DownloadProgress) error {
	progress.Total = resp.ContentLength

	tempFile := destination + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close file: %v", closeErr)
		}
	}()

	err = m.copyWithProgress(ctx, file, resp.Body, progress)
	if err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			log.Printf("Failed to remove temp file: %v", removeErr)
		}
		return err
	}

	if err := os.Rename(tempFile, destination); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			log.Printf("Failed to remove temp file: %v", removeErr)
		}
		return fmt.Errorf("move file: %w", err)
	}

	return nil
}

func (m *Manager) copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, progress *types.DownloadProgress) error {
	buffer := make([]byte, m.cfg.Download.ChunkSize)
	lastProgressUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			progress.Status = types.DownloadStatusCancelled
			progress.Error = ctx.Err()
			return ctx.Err()
		default:
		}

		n, err := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				progress.Status = types.DownloadStatusFailed
				progress.Error = fmt.Errorf("write file: %w", writeErr)
				return writeErr
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
			progress.Status = types.DownloadStatusFailed
			progress.Error = fmt.Errorf("read response: %w", err)
			return err
		}
	}

	return nil
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

func (sr *StreamReader) Read(p []byte) (int, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	availableData := int64(len(sr.buffer)) - sr.position

	if availableData <= 0 {
		if sr.downloadDone {
			return 0, io.EOF
		}
		return 0, io.ErrNoProgress
	}

	toRead := int64(len(p))
	if toRead > availableData {
		toRead = availableData
	}

	copy(p, sr.buffer[sr.position:sr.position+toRead])
	sr.position += toRead

	return int(toRead), nil
}

func (sr *StreamReader) Seek(offset int64, whence int) (int64, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	switch whence {
	case io.SeekStart:
		sr.position = offset
	case io.SeekCurrent:
		sr.position += offset
	case io.SeekEnd:
		sr.position = int64(len(sr.buffer)) + offset
	}

	if sr.position < 0 {
		sr.position = 0
	}
	if sr.position > int64(len(sr.buffer)) {
		sr.position = int64(len(sr.buffer))
	}

	return sr.position, nil
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
