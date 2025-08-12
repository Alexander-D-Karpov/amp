package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type StreamReader struct {
	url         string
	buffer      *streamBuffer
	manager     *Manager
	httpClient  *http.Client
	ctx         context.Context
	cancel      context.CancelFunc
	startOffset int64

	mutex sync.RWMutex
}

type streamBuffer struct {
	data         []byte
	position     int64
	totalSize    int64
	downloadDone bool
	lastAccess   time.Time

	mutex sync.RWMutex
}

func (m *Manager) GetStreamReader(ctx context.Context, url string) (*StreamReader, error) {
	if existing, ok := m.activeStreams.Load(url); ok {
		reader := existing.(*StreamReader)
		reader.buffer.mutex.Lock()
		reader.buffer.lastAccess = time.Now()
		reader.buffer.mutex.Unlock()
		m.debugLog("Reusing existing stream reader: %s", url)
		return reader, nil
	}

	streamCtx, cancel := context.WithCancel(ctx)

	reader := &StreamReader{
		url:        url,
		buffer:     &streamBuffer{},
		manager:    m,
		httpClient: m.httpClient,
		ctx:        streamCtx,
		cancel:     cancel,
	}

	m.activeStreams.Store(url, reader)
	m.debugLog("Created new stream reader: %s", url)

	go reader.startStreaming()

	return reader, nil
}

func (sr *StreamReader) startStreaming() {
	defer func() {
		sr.manager.activeStreams.Delete(sr.url)
		sr.manager.debugLog("Stream reader cleaned up: %s", sr.url)
	}()

	req, err := http.NewRequestWithContext(sr.ctx, "GET", sr.url, nil)
	if err != nil {
		sr.manager.debugLog("Failed to create stream request: %v", err)
		return
	}

	req.Header.Set("User-Agent", sr.manager.config.UserAgent)
	req.Header.Set("Accept", "*/*")

	if sr.startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", sr.startOffset))
	}

	sr.manager.debugLog("Starting stream download: %s", sr.url)

	resp, err := sr.httpClient.Do(req)
	if err != nil {
		sr.manager.debugLog("Stream request failed: %v", err)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			sr.manager.debugLog("Failed to close stream response: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		sr.manager.debugLog("Stream request failed with status: %d", resp.StatusCode)
		return
	}

	var contentLength int64
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, parseErr := strconv.ParseInt(cl, 10, 64); parseErr == nil {
			contentLength = size + sr.startOffset
		}
	}

	sr.buffer.mutex.Lock()
	sr.buffer.totalSize = contentLength
	sr.buffer.mutex.Unlock()

	sr.manager.debugLog("Stream content length: %d", contentLength)

	buffer := make([]byte, sr.manager.config.ChunkSize)
	startTime := time.Now()

	for {
		select {
		case <-sr.ctx.Done():
			sr.manager.debugLog("Stream download cancelled: %s", sr.url)
			return
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])

			sr.buffer.mutex.Lock()
			sr.buffer.data = append(sr.buffer.data, chunk...)
			sr.buffer.lastAccess = time.Now()
			sr.buffer.mutex.Unlock()

			if time.Since(startTime) > 5*time.Second {
				sr.buffer.mutex.RLock()
				downloaded := int64(len(sr.buffer.data))
				sr.buffer.mutex.RUnlock()
				sr.manager.debugLog("Stream progress: %d/%d bytes", downloaded, contentLength)
				startTime = time.Now()
			}
		}

		if err != nil {
			if err == io.EOF {
				sr.buffer.mutex.Lock()
				sr.buffer.downloadDone = true
				sr.buffer.mutex.Unlock()
				sr.manager.debugLog("Stream download completed: %s", sr.url)
				break
			}
			sr.manager.debugLog("Stream read error: %v", err)
			return
		}
	}
}

func (sr *StreamReader) Read(p []byte) (int, error) {
	for {
		sr.buffer.mutex.RLock()
		available := int64(len(sr.buffer.data)) - sr.buffer.position
		downloadDone := sr.buffer.downloadDone
		sr.buffer.mutex.RUnlock()

		if available > 0 {
			break
		}

		if downloadDone {
			return 0, io.EOF
		}

		select {
		case <-sr.ctx.Done():
			return 0, sr.ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	sr.buffer.mutex.Lock()
	defer sr.buffer.mutex.Unlock()

	sr.buffer.lastAccess = time.Now()

	available := int64(len(sr.buffer.data)) - sr.buffer.position
	toRead := int64(len(p))
	if toRead > available {
		toRead = available
	}

	if toRead <= 0 {
		if sr.buffer.downloadDone {
			return 0, io.EOF
		}
		return 0, nil
	}

	start := sr.buffer.position
	end := start + toRead
	copy(p, sr.buffer.data[start:end])
	sr.buffer.position = end

	return int(toRead), nil
}

func (sr *StreamReader) Seek(offset int64, whence int) (int64, error) {
	sr.buffer.mutex.Lock()
	defer sr.buffer.mutex.Unlock()

	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = sr.buffer.position + offset
	case io.SeekEnd:
		if sr.buffer.totalSize > 0 {
			newPos = sr.buffer.totalSize + offset
		} else {
			newPos = int64(len(sr.buffer.data)) + offset
		}
	default:
		return 0, fmt.Errorf("invalid whence value: %d", whence)
	}

	if newPos < 0 {
		newPos = 0
	}

	if newPos > int64(len(sr.buffer.data)) {
		if sr.buffer.downloadDone {
			newPos = int64(len(sr.buffer.data))
		} else {
			return 0, fmt.Errorf("seek beyond available data")
		}
	}

	sr.buffer.position = newPos
	sr.buffer.lastAccess = time.Now()

	return newPos, nil
}

func (sr *StreamReader) Close() error {
	if sr.cancel != nil {
		sr.cancel()
	}
	sr.manager.activeStreams.Delete(sr.url)
	sr.manager.debugLog("Stream reader closed: %s", sr.url)
	return nil
}

func (sr *StreamReader) GetDownloadProgress() (downloaded, total int64, percentage float64) {
	sr.buffer.mutex.RLock()
	defer sr.buffer.mutex.RUnlock()

	downloaded = int64(len(sr.buffer.data))
	total = sr.buffer.totalSize

	if total > 0 {
		percentage = float64(downloaded) / float64(total) * 100
	}

	return downloaded, total, percentage
}

func (sr *StreamReader) IsDownloadComplete() bool {
	sr.buffer.mutex.RLock()
	defer sr.buffer.mutex.RUnlock()
	return sr.buffer.downloadDone
}

func (sr *StreamReader) GetAvailableDuration(bitrate int) time.Duration {
	if bitrate <= 0 {
		return 0
	}

	sr.buffer.mutex.RLock()
	available := int64(len(sr.buffer.data)) - sr.buffer.position
	sr.buffer.mutex.RUnlock()

	if available <= 0 {
		return 0
	}

	bytesPerSecond := float64(bitrate) / 8.0
	seconds := float64(available) / bytesPerSecond

	return time.Duration(seconds * float64(time.Second))
}

func (m *Manager) CleanupInactiveStreams(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	m.activeStreams.Range(func(key, value interface{}) bool {
		reader := value.(*StreamReader)
		reader.buffer.mutex.RLock()
		lastAccess := reader.buffer.lastAccess
		reader.buffer.mutex.RUnlock()

		if lastAccess.Before(cutoff) {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, url := range toDelete {
		if reader, ok := m.activeStreams.LoadAndDelete(url); ok {
			if sr := reader.(*StreamReader); sr != nil {
				sr.Close()
			}
		}
	}

	if len(toDelete) > 0 {
		m.debugLog("Cleaned up %d inactive streams", len(toDelete))
	}
}
