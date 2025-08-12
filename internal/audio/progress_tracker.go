package audio

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/gopxl/beep"
)

type ProgressTracker struct {
	ticker           *time.Ticker
	done             chan struct{}
	running          bool
	callback         func(time.Duration)
	streamer         beep.StreamSeeker
	sampleRate       beep.SampleRate
	expectedDuration time.Duration
	startTime        time.Time
	baseOffset       time.Duration
	mutex            sync.RWMutex
}

func NewProgressTracker(interval time.Duration) *ProgressTracker {
	return &ProgressTracker{
		ticker: time.NewTicker(interval),
		done:   make(chan struct{}),
	}
}

func (pt *ProgressTracker) Start(callback func(time.Duration)) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	if pt.running {
		return
	}

	pt.callback = callback
	pt.running = true
	pt.startTime = time.Now()

	if pt.ticker != nil {
		pt.ticker.Stop()
	}
	pt.ticker = time.NewTicker(50 * time.Millisecond)

	go pt.run()
}

func (pt *ProgressTracker) Stop() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	if !pt.running {
		return
	}

	pt.running = false

	select {
	case <-pt.done:
	default:
		close(pt.done)
		pt.done = make(chan struct{})
	}

	if pt.ticker != nil {
		pt.ticker.Stop()
	}
}

func (pt *ProgressTracker) IsRunning() bool {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	return pt.running
}

func (pt *ProgressTracker) Reset() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.streamer = nil
	pt.sampleRate = 0
	pt.expectedDuration = 0
	pt.baseOffset = 0
	pt.startTime = time.Now()
}

// SetStreamer now accepts baseOffset which is the absolute position (from track start)
// where the current decoder/segment begins. The reported position will be baseOffset + segment time.
func (pt *ProgressTracker) SetStreamer(streamer beep.StreamSeeker, sampleRate beep.SampleRate, expectedDuration time.Duration, baseOffset time.Duration) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.streamer = streamer
	pt.sampleRate = sampleRate
	pt.expectedDuration = expectedDuration
	pt.baseOffset = baseOffset
	pt.startTime = time.Now()
}

func (pt *ProgressTracker) run() {
	defer func() {
		pt.mutex.Lock()
		pt.running = false
		pt.mutex.Unlock()
	}()

	for {
		select {
		case <-pt.ticker.C:
			pt.updatePosition()
		case <-pt.done:
			return
		}
	}
}

func (pt *ProgressTracker) updatePosition() {
	pt.mutex.RLock()
	streamer := pt.streamer
	sampleRate := pt.sampleRate
	expectedDuration := pt.expectedDuration
	callback := pt.callback
	running := pt.running
	startTime := pt.startTime
	baseOffset := pt.baseOffset
	pt.mutex.RUnlock()

	if !running || streamer == nil || callback == nil {
		return
	}

	// When re-decoding from a buffered offset, compute time within this segment and add baseOffset.
	if baseOffset > 0 {
		if sampleRate > 0 {
			callback(baseOffset + sampleRate.D(streamer.Position()))
			return
		}
		// Fallback: elapsed wall time + baseOffset
		elapsed := time.Since(startTime)
		if expectedDuration > 0 && baseOffset+elapsed > expectedDuration {
			elapsed = expectedDuration - baseOffset
			if elapsed < 0 {
				elapsed = 0
			}
		}
		callback(baseOffset + elapsed)
		return
	}

	// Non-segmented path
	if expectedDuration > 0 && streamer.Len() > 0 {
		cur := streamer.Position()
		total := streamer.Len()
		if total > 0 {
			frac := float64(cur) / float64(total)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			callback(time.Duration(frac * float64(expectedDuration)))
			return
		}
	}

	if sampleRate > 0 {
		callback(sampleRate.D(streamer.Position()))
		return
	}

	elapsed := time.Since(startTime)
	if expectedDuration > 0 && elapsed > expectedDuration {
		elapsed = expectedDuration
	}
	callback(elapsed)
}

// -------- Buffering support --------

type BufferManager struct {
	cfg           *config.Config
	minBufferTime time.Duration
	bufferPercent float64
	debug         bool
}

func NewBufferManager(cfg *config.Config, debug bool) *BufferManager {
	return &BufferManager{
		cfg:           cfg,
		minBufferTime: 10 * time.Second,
		bufferPercent: 0.05,
		debug:         debug,
	}
}

func (bm *BufferManager) GetMinBufferPercent() float64 {
	return bm.bufferPercent
}

func (bm *BufferManager) SetMinBufferPercent(percent float64) {
	if percent < 0.01 {
		percent = 0.01
	}
	if percent > 0.5 {
		percent = 0.5
	}
	bm.bufferPercent = percent
}

func (bm *BufferManager) SetMinBufferTime(duration time.Duration) {
	if duration < time.Second {
		duration = time.Second
	}
	bm.minBufferTime = duration
}

// WaitForSufficientBuffer blocks until enough data is available to start smooth playback
// (initial buffer), or until the context is canceled. It works only for our StreamReader;
// other readers are treated as already buffered.
func (bm *BufferManager) WaitForSufficientBuffer(ctx context.Context, reader io.Reader) bool {
	sr, ok := reader.(*StreamReader)
	if !ok {
		return true
	}

	if bm.debug {
		log.Printf("[BUFFER_MANAGER] Waiting for sufficient buffer")
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := 30 * time.Second
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			if bm.debug {
				log.Printf("[BUFFER_MANAGER] Buffer wait canceled")
			}
			return false

		case <-timeoutTimer.C:
			if bm.debug {
				log.Printf("[BUFFER_MANAGER] Buffer wait timeout - proceeding anyway")
			}
			return true

		case <-ticker.C:
			if sr.IsComplete() {
				if bm.debug {
					log.Printf("[BUFFER_MANAGER] Download complete, starting playback")
				}
				return true
			}

			// Prefer reader’s internal readiness (min initial bytes reached)
			if sr.IsBufferReady() {
				downloaded, _, progress := sr.GetProgress()
				if bm.debug {
					log.Printf("[BUFFER_MANAGER] Buffer ready - Downloaded: %d bytes (%.1f%%), starting playback",
						downloaded, progress*100)
				}
				return true
			}

			// Secondary criteria: percentage-based threshold if total size known
			dl, total, pct := sr.GetProgress()
			if total > 0 && pct >= bm.bufferPercent {
				if bm.debug {
					log.Printf("[BUFFER_MANAGER] Percent threshold reached: %.1f%% (%d/%d), starting playback",
						pct*100, dl, total)
				}
				return true
			}
		}
	}
}
