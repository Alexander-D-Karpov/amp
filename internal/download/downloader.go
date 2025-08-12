package download

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

func (m *Manager) performDownload(ctx context.Context, task *Task) error {
	req, err := http.NewRequestWithContext(ctx, "GET", task.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", m.config.UserAgent)
	req.Header.Set("Accept", "*/*")

	m.debugLog("Sending HTTP request: %s", task.URL)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			m.debugLog("Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var contentLength int64
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, parseErr := strconv.ParseInt(cl, 10, 64); parseErr == nil {
			contentLength = size
		}
	}

	task.Progress.mutex.Lock()
	task.Progress.Total = contentLength
	task.Progress.mutex.Unlock()

	m.debugLog("Starting download - Content-Length: %d", contentLength)

	tempFile := task.Destination + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			m.debugLog("Failed to close temp file: %v", closeErr)
		}
	}()

	err = m.copyWithProgress(ctx, file, resp.Body, task)
	if err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			m.debugLog("Failed to remove temp file: %v", removeErr)
		}
		return err
	}

	if err := os.Rename(tempFile, task.Destination); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			m.debugLog("Failed to remove temp file after rename error: %v", removeErr)
		}
		return fmt.Errorf("move file to destination: %w", err)
	}

	m.debugLog("Download completed: %s", task.Destination)
	return nil
}
