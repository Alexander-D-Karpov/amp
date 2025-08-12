package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

func (m *Manager) copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, task *Task) error {
	buffer := make([]byte, m.config.ChunkSize)
	startTime := time.Now()
	lastProgressUpdate := startTime

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("write chunk: %w", writeErr)
			}

			task.Progress.mutex.Lock()
			task.Progress.Downloaded += int64(n)
			downloaded := task.Progress.Downloaded
			total := task.Progress.Total
			task.Progress.mutex.Unlock()

			now := time.Now()
			if now.Sub(lastProgressUpdate) >= 100*time.Millisecond {
				m.updateProgressMetrics(task, downloaded, total, now, startTime)
				m.notifyProgress(task)
				lastProgressUpdate = now
			}
		}

		if err != nil {
			if err == io.EOF {
				task.Progress.mutex.Lock()
				downloaded := task.Progress.Downloaded
				total := task.Progress.Total
				task.Progress.mutex.Unlock()

				m.updateProgressMetrics(task, downloaded, total, time.Now(), startTime)
				m.notifyProgress(task)
				break
			}
			return fmt.Errorf("read chunk: %w", err)
		}
	}

	return nil
}

func (m *Manager) updateProgressMetrics(task *Task, downloaded, total int64, now, startTime time.Time) {
	task.Progress.mutex.Lock()
	defer task.Progress.mutex.Unlock()

	if total > 0 {
		task.Progress.Percentage = float64(downloaded) / float64(total) * 100
	}

	elapsed := now.Sub(startTime).Seconds()
	if elapsed > 0 {
		task.Progress.Speed = float64(downloaded) / elapsed
	}

	if task.Progress.Speed > 0 && total > 0 {
		remaining := total - downloaded
		etaSeconds := float64(remaining) / task.Progress.Speed
		task.Progress.ETA = time.Duration(etaSeconds) * time.Second
	}

	task.Progress.LastUpdate = now
}

func (m *Manager) handleDownloadSuccess(task *Task) {
	if err := m.validateDownload(task); err != nil {
		m.updateTaskState(task, StateFailed, err)
		return
	}

	if task.Song != nil {
		task.Song.LocalPath = &task.Destination
		task.Song.Downloaded = true
		m.debugLog("Updated song metadata: %s -> %s", task.Song.Name, task.Destination)

		go func() {
			_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			m.debugLog("Song download completed and metadata updated: %s", task.Song.Name)
		}()
	}

	task.Progress.mutex.Lock()
	task.Progress.Percentage = 100.0
	task.Progress.LastUpdate = time.Now()
	task.Progress.mutex.Unlock()

	m.updateTaskState(task, StateCompleted, nil)
	m.notifyProgress(task)
	m.debugLog("Download completed successfully: %s", task.Destination)
}

func (m *Manager) validateDownload(task *Task) error {
	stat, err := os.Stat(task.Destination)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	if stat.Size() == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	if strings.HasSuffix(strings.ToLower(task.Destination), ".mp3") {
		if stat.Size() < 1024 {
			return fmt.Errorf("audio file too small: %d bytes", stat.Size())
		}

		if err := m.validateMP3File(task.Destination); err != nil {
			return fmt.Errorf("invalid MP3 file: %w", err)
		}
	}

	m.debugLog("Download validation passed: %s (%d bytes)", task.Destination, stat.Size())
	return nil
}

func (m *Manager) validateMP3File(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 10)
	n, err := file.Read(header)
	if err != nil || n < 3 {
		return fmt.Errorf("cannot read file header")
	}

	if (header[0] == 0xFF && (header[1]&0xE0) == 0xE0) ||
		(n >= 3 && header[0] == 'I' && header[1] == 'D' && header[2] == '3') {
		return nil
	}

	return fmt.Errorf("not a valid MP3 file")
}

func (m *Manager) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "context canceled") ||
		strings.Contains(errStr, "context deadline") {
		return false
	}

	permanentErrors := []string{
		"400", "401", "403", "404", "405", "406", "410", "451",
	}
	for _, code := range permanentErrors {
		if strings.Contains(errStr, code) {
			return false
		}
	}

	if strings.Contains(errStr, "no space left") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "file exists") {
		return false
	}

	retryableErrors := []string{
		"connection refused", "connection reset", "connection timeout",
		"timeout", "temporary failure", "network unreachable", "host unreachable",
		"500", "502", "503", "504", "dns", "resolve", "lookup",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return true
}

func (m *Manager) CleanupFailedDownloads() {
	var toDelete []string

	m.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		task.mutex.RLock()
		state := task.State
		destination := task.Destination
		task.mutex.RUnlock()

		if state == StateFailed {
			if _, err := os.Stat(destination); err == nil {
				if removeErr := os.Remove(destination); removeErr != nil {
					m.debugLog("Failed to remove failed download: %v", removeErr)
				} else {
					m.debugLog("Removed failed download: %s", destination)
				}
			}
			toDelete = append(toDelete, key.(string))
		}

		return true
	})

	for _, key := range toDelete {
		m.tasks.Delete(key)
	}

	if len(toDelete) > 0 {
		m.debugLog("Cleaned up %d failed downloads", len(toDelete))
	}
}

func (m *Manager) updateTaskState(task *Task, state State, err error) {
	task.mutex.Lock()
	task.State = state
	task.Error = err
	if state == StateCompleted || state == StateFailed || state == StateCancelled {
		now := time.Now()
		task.CompletedAt = &now
	}
	task.mutex.Unlock()

	m.debugLog("Task state changed: %s -> %s", task.URL, state.String())
	m.notifyCompletion(task)
}

func (m *Manager) stateToDownloadStatus(state State) types.DownloadStatus {
	switch state {
	case StatePending:
		return types.DownloadStatusPending
	case StateDownloading:
		return types.DownloadStatusDownloading
	case StateCompleted:
		return types.DownloadStatusCompleted
	case StateFailed:
		return types.DownloadStatusFailed
	case StateCancelled:
		return types.DownloadStatusCancelled
	default:
		return types.DownloadStatusFailed
	}
}

func (m *Manager) taskToProgress(task *Task) *types.DownloadProgress {
	task.mutex.RLock()
	defer task.mutex.RUnlock()

	task.Progress.mutex.RLock()
	defer task.Progress.mutex.RUnlock()

	filename := task.Title
	if filename == "" {
		if task.Song != nil && task.Song.Name != "" {
			filename = task.Song.Name
		} else {
			filename = filepath.Base(task.URL)
		}
	}

	status := m.stateToDownloadStatus(task.State)

	eta := task.Progress.ETA
	if eta == 0 && task.Progress.Speed > 0 && task.Progress.Total > 0 {
		remaining := task.Progress.Total - task.Progress.Downloaded
		if remaining > 0 {
			etaSeconds := float64(remaining) / task.Progress.Speed
			eta = time.Duration(etaSeconds) * time.Second
		}
	}

	return &types.DownloadProgress{
		URL:        task.URL,
		Filename:   filename,
		Total:      task.Progress.Total,
		Downloaded: task.Progress.Downloaded,
		Progress:   task.Progress.Percentage,
		Speed:      task.Progress.Speed,
		Status:     status,
		Error:      task.Error,
		StartTime:  task.StartTime,
		LastUpdate: task.Progress.LastUpdate,
		ETA:        eta,
	}
}

func (m *Manager) notifyProgress(task *Task) {
	progress := m.taskToProgress(task)

	m.callbackMutex.RLock()
	callbacks := make([]ProgressCallback, len(m.progressCbs))
	copy(callbacks, m.progressCbs)
	m.callbackMutex.RUnlock()

	for _, callback := range callbacks {
		if callback != nil {
			go func(cb ProgressCallback, p *types.DownloadProgress) {
				defer func() {
					if r := recover(); r != nil {
						m.debugLog("Progress callback panicked: %v", r)
					}
				}()
				cb(p)
			}(callback, progress)
		}
	}
}

func (m *Manager) notifyCompletion(task *Task) {
	m.callbackMutex.RLock()
	callbacks := make([]CompletionCallback, len(m.completionCbs))
	copy(callbacks, m.completionCbs)
	m.callbackMutex.RUnlock()

	for _, callback := range callbacks {
		if callback != nil {
			go func(cb CompletionCallback, t *Task) {
				defer func() {
					if r := recover(); r != nil {
						m.debugLog("Completion callback panicked: %v", r)
					}
				}()
				cb(t)
			}(callback, task)
		}
	}
}
