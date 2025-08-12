package audio

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type StreamReader struct {
	url        string
	buffer     []byte
	position   int64
	totalSize  int64
	downloaded int64
	done       bool
	err        error
	ctx        context.Context
	cancel     context.CancelFunc
	mutex      sync.RWMutex
	cond       *sync.Cond
	httpClient *http.Client
	debug      bool

	minBufferSize int64
	bufferReady   bool
	lastReadTime  time.Time
}

func (sm *StreamManager) CreateStream(ctx context.Context, url string) (io.ReadCloser, error) {
	if existing, ok := sm.activeStreams.Load(url); ok {
		reader := existing.(*StreamReader)
		reader.mutex.Lock()
		reader.position = 0
		reader.bufferReady = false
		reader.lastReadTime = time.Now()
		reader.mutex.Unlock()

		if sm.debug {
			log.Printf("[STREAM_MANAGER] Reusing existing stream: %s (buffer: %d bytes)", url, len(reader.buffer))
		}
		return reader, nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	reader := &StreamReader{
		url:           url,
		ctx:           streamCtx,
		cancel:        cancel,
		httpClient:    sm.httpClient,
		debug:         sm.debug,
		minBufferSize: 256 * 1024, // ~256KB before decode starts
		lastReadTime:  time.Now(),
	}
	reader.cond = sync.NewCond(&reader.mutex)

	sm.activeStreams.Store(url, reader)

	if sm.debug {
		log.Printf("[STREAM_MANAGER] Creating new stream: %s", url)
	}

	go reader.startDownload()

	return reader, nil
}

func (sm *StreamManager) GetDownloadProgress() float64 {
	progress := 0.0
	count := 0

	sm.activeStreams.Range(func(key, value interface{}) bool {
		if reader, ok := value.(*StreamReader); ok {
			_, _, pct := reader.GetProgress()
			progress += pct
			count++
		}
		return true
	})

	if count == 0 {
		return 1.0
	}
	return progress / float64(count)
}

func (sm *StreamManager) CleanupStreams() {
	sm.activeStreams.Range(func(key, value interface{}) bool {
		if reader, ok := value.(*StreamReader); ok {
			reader.Close()
		}
		sm.activeStreams.Delete(key)
		return true
	})
}

func (sm *StreamManager) Close() {
	sm.CleanupStreams()
}

func (sr *StreamReader) startDownload() {
	defer func() {
		sr.mutex.Lock()
		sr.done = true
		sr.mutex.Unlock()
		sr.cond.Broadcast()

		if sr.debug {
			log.Printf("[STREAM_READER] Download completed for: %s (total: %d bytes)", sr.url, sr.downloaded)
		}
	}()

	req, err := http.NewRequestWithContext(sr.ctx, "GET", sr.url, nil)
	if err != nil {
		if sr.debug {
			log.Printf("[STREAM_READER] Failed to create request: %v", err)
		}
		sr.mutex.Lock()
		sr.err = err
		sr.mutex.Unlock()
		return
	}

	req.Header.Set("User-Agent", "AMP/1.0.0")
	req.Header.Set("Accept", "audio/mpeg, audio/mp4, audio/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Range", "bytes=0-")

	if sr.debug {
		log.Printf("[STREAM_READER] Starting download with headers: %v", req.Header)
	}

	resp, err := sr.httpClient.Do(req)
	if err != nil {
		if sr.debug {
			log.Printf("[STREAM_READER] Request failed: %v", err)
		}
		sr.mutex.Lock()
		sr.err = err
		sr.mutex.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		if sr.debug {
			log.Printf("[STREAM_READER] HTTP error: %v", err)
		}
		sr.mutex.Lock()
		sr.err = err
		sr.mutex.Unlock()
		return
	}

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if v, perr := strconv.ParseInt(cl, 10, 64); perr == nil {
			sr.mutex.Lock()
			sr.totalSize = v
			sr.mutex.Unlock()
			if sr.debug {
				log.Printf("[STREAM_READER] Content-Length: %d bytes (%.2f MB)", v, float64(v)/(1024*1024))
			}
		}
	}

	if sr.debug {
		log.Printf("[STREAM_READER] Response headers - Content-Type: %s, Accept-Ranges: %s",
			resp.Header.Get("Content-Type"), resp.Header.Get("Accept-Ranges"))
	}

	buf := make([]byte, 64*1024)
	lastLogTime := time.Now()
	lastLoggedDownloaded := int64(0)

	for {
		select {
		case <-sr.ctx.Done():
			if sr.debug {
				log.Printf("[STREAM_READER] Download cancelled: %s", sr.url)
			}
			return
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			sr.mutex.Lock()
			sr.buffer = append(sr.buffer, buf[:n]...)
			sr.downloaded += int64(n)

			if !sr.bufferReady && sr.downloaded >= sr.minBufferSize {
				sr.bufferReady = true
				if sr.debug {
					log.Printf("[STREAM_READER] Initial buffer ready: %d bytes", sr.downloaded)
				}
			}

			sr.mutex.Unlock()
			sr.cond.Broadcast()

			now := time.Now()
			if sr.totalSize > 0 {
				if now.Sub(lastLogTime) > 5*time.Second || sr.downloaded-lastLoggedDownloaded >= 1<<20 {
					pct := float64(sr.downloaded) / float64(sr.totalSize) * 100
					speedKBs := float64(sr.downloaded-lastLoggedDownloaded) / now.Sub(lastLogTime).Seconds() / 1024
					if sr.debug {
						log.Printf("[STREAM_READER] Downloaded: %.1f%% (%d/%d bytes) @ %.1f KB/s",
							pct, sr.downloaded, sr.totalSize, speedKBs)
					}
					lastLogTime = now
					lastLoggedDownloaded = sr.downloaded
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				if sr.debug {
					log.Printf("[STREAM_READER] Download completed successfully: %s", sr.url)
				}
				return
			}
			if sr.debug {
				log.Printf("[STREAM_READER] Read error: %v", err)
			}
			sr.mutex.Lock()
			sr.err = err
			sr.mutex.Unlock()
			return
		}
	}
}

func (sr *StreamReader) Read(p []byte) (int, error) {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	sr.lastReadTime = time.Now()

	for {
		if sr.err != nil && sr.err != io.EOF {
			return 0, sr.err
		}

		available := len(sr.buffer) - int(sr.position)
		if available > 0 {
			n := available
			if n > len(p) {
				n = len(p)
			}
			start := int(sr.position)
			end := start + n
			copy(p, sr.buffer[start:end])
			sr.position += int64(n)

			if sr.debug && (sr.position%1048576 == 0) {
				progress := 0.0
				if len(sr.buffer) > 0 {
					progress = float64(sr.position) / float64(len(sr.buffer)) * 100
				}
				log.Printf("[STREAM_READER] Read progress: %.1f%% (%d/%d bytes)",
					progress, sr.position, len(sr.buffer))
			}
			return n, nil
		}

		if sr.done {
			return 0, io.EOF
		}

		sr.cond.Wait()
	}
}

func (sr *StreamReader) Close() error {
	if sr.cancel != nil {
		sr.cancel()
	}
	sr.mutex.Lock()
	sr.done = true
	sr.mutex.Unlock()
	sr.cond.Broadcast()

	if sr.debug {
		log.Printf("[STREAM_READER] Stream closed: %s", sr.url)
	}
	return nil
}

func (sr *StreamReader) GetProgress() (downloaded, total int64, percentage float64) {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()

	downloaded = sr.downloaded
	total = sr.totalSize

	if total > 0 {
		percentage = float64(downloaded) / float64(total)
		if percentage > 1.0 {
			percentage = 1.0
		}
	}

	return downloaded, total, percentage
}

func (sr *StreamReader) IsComplete() bool {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()
	return sr.done
}

func (sr *StreamReader) IsBufferReady() bool {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()
	return sr.bufferReady || sr.done
}

func (sr *StreamReader) NewSegmentFrom(offset int64) io.ReadCloser {
	if offset < 0 {
		offset = 0
	}
	return &SegmentReader{
		sr:     sr,
		start:  offset,
		cursor: 0,
	}
}

// TotalSize returns the HTTP Content-Length if known (or 0).
func (sr *StreamReader) TotalSize() int64 {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()
	return sr.totalSize
}

// DownloadedSize returns the number of bytes downloaded so far.
func (sr *StreamReader) DownloadedSize() int64 {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()
	return sr.downloaded
}

// SegmentReader is a read-only view into StreamReader that begins at a given byte offset.
// It does NOT close/cancel the underlying stream when closed.
type SegmentReader struct {
	sr     *StreamReader
	start  int64 // absolute start in bytes
	cursor int64 // relative bytes read from `start`
}

func (seg *SegmentReader) Read(p []byte) (int, error) {
	sr := seg.sr

	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	abs := seg.start + seg.cursor

	for {
		available := int64(len(sr.buffer)) - abs
		if available > 0 {
			// We have bytes ready to serve from buffer.
			toRead := int64(len(p))
			if toRead > available {
				toRead = available
			}
			start := abs
			end := abs + toRead
			n := copy(p, sr.buffer[start:end])
			seg.cursor += int64(n)
			return n, nil
		}

		// No bytes available at/after our absolute position.
		if sr.done {
			// End of stream reached.
			return 0, io.EOF
		}

		// Wait for more data to be buffered.
		sr.cond.Wait()
		// loop and re-check
	}
}

func (seg *SegmentReader) Close() error {
	// Intentionally no-op: must not cancel download.
	return nil
}

func (sm *StreamManager) GetStream(url string) (*StreamReader, bool) {
	if v, ok := sm.activeStreams.Load(url); ok {
		if sr, ok2 := v.(*StreamReader); ok2 {
			return sr, true
		}
	}
	return nil, false
}
