package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
	"path/filepath"
	"strings"
)

var (
	speakerInitialized = false
	speakerMutex       sync.Mutex
	globalSampleRate   beep.SampleRate
	speakerOnce        sync.Once
)

type StreamManager struct {
	httpClient    *http.Client
	cfg           *config.Config
	debug         bool
	activeStreams sync.Map
}

type Player struct {
	mu sync.RWMutex

	cfg              *config.Config
	storage          *storage.Database
	currentSong      *types.Song
	streamer         beep.StreamSeekCloser
	ctrl             *beep.Ctrl
	volume           *effects.Volume
	position         time.Duration
	duration         time.Duration
	expectedDuration time.Duration
	positionCallback func(time.Duration)
	finishedCallback func()
	sampleRate       beep.SampleRate
	srcSampleRate    beep.SampleRate
	isSeekable       bool
	ticker           *time.Ticker
	done             chan struct{}
	httpClient       *http.Client
	debug            bool
	playing          bool
	paused           bool
	bufferSize       int
	lastPosition     time.Duration

	// Streaming components
	streamManager   *StreamManager
	progressTracker *ProgressTracker
	bufferManager   *BufferManager

	// Buffered streaming seek state
	activeStream *StreamReader // non-nil when streaming; shared downloader
	baseOffset   time.Duration // absolute time where current segment starts

	// Track completion detection
	playbackStartTime   time.Time
	minPlayTime         time.Duration
	completionThreshold float64

	currentSongSlug string
	loadingCanceled bool
	loadingContext  context.Context
	loadingCancel   context.CancelFunc
}

func NewPlayer(cfg *config.Config, storage *storage.Database) (*Player, error) {
	p := &Player{
		cfg:     cfg,
		storage: storage,
		done:    make(chan struct{}),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Increased from 30 seconds to 10 minutes for streaming
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second, // Added this
				IdleConnTimeout:       90 * time.Second, // Added this
				MaxIdleConns:          10,
				DisableCompression:    true, // Disable compression for audio streaming
			},
		},
		sampleRate:          beep.SampleRate(cfg.Audio.SampleRate),
		srcSampleRate:       beep.SampleRate(cfg.Audio.SampleRate),
		debug:               cfg.Debug,
		playing:             false,
		paused:              false,
		minPlayTime:         5 * time.Second,
		completionThreshold: 0.95,
	}

	p.bufferSize = p.calculateOptimalBufferSize()

	if err := p.initializeSpeaker(); err != nil {
		return nil, fmt.Errorf("failed to initialize speaker: %w", err)
	}

	// Initialize sub-components
	p.streamManager = NewStreamManager(p.httpClient, cfg, p.debug)
	p.progressTracker = NewProgressTracker(50 * time.Millisecond)
	p.bufferManager = NewBufferManager(cfg, p.debug)

	if p.debug {
		log.Printf("[AUDIO] Player initialized - OS: %s, Sample Rate: %d, Buffer: %d",
			runtime.GOOS, p.sampleRate, p.bufferSize)
	}

	return p, nil
}

func NewStreamManager(client *http.Client, cfg *config.Config, debug bool) *StreamManager {
	return &StreamManager{
		httpClient: client,
		cfg:        cfg,
		debug:      debug,
	}
}

func (p *Player) calculateOptimalBufferSize() int {
	baseBuffer := p.cfg.Audio.BufferSize

	switch runtime.GOOS {
	case "linux":
		return baseBuffer * 2
	case "windows":
		return baseBuffer
	case "darwin":
		return baseBuffer
	default:
		return baseBuffer * 2
	}
}

func (p *Player) initializeSpeaker() error {
	var err error
	speakerOnce.Do(func() {
		buf := p.sampleRate.N(200 * time.Millisecond)
		err = speaker.Init(p.sampleRate, buf)
		if p.debug {
			log.Printf("[AUDIO] speaker.Init(%d, %d)", p.sampleRate, buf)
		}
	})
	return err
}

func (p *Player) mkVolume(vol float64) *effects.Volume {
	v := &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
	}
	if vol <= 0 {
		v.Silent = true
	} else {
		v.Volume = (vol - 1) * 5
	}
	return v
}

func (p *Player) Play(ctx context.Context, song *types.Song) error {
	if song == nil {
		return fmt.Errorf("song cannot be nil")
	}

	if p.debug {
		log.Printf("[AUDIO] Starting playback for: %s (Length: %d seconds)", song.Name, song.Length)
	}

	p.mu.Lock()
	// Cancel any ongoing loading
	if p.loadingCancel != nil {
		p.loadingCancel()
		p.loadingCanceled = true
	}

	// Create new loading context
	p.loadingContext, p.loadingCancel = context.WithCancel(ctx)
	loadingCtx := p.loadingContext

	p.stopInternal()
	p.currentSong = song
	p.currentSongSlug = song.Slug
	p.playing = false
	p.paused = false
	p.position = 0
	p.playbackStartTime = time.Now()
	p.loadingCanceled = false

	if song.Length > 0 {
		p.expectedDuration = time.Duration(song.Length) * time.Second
		if p.debug {
			log.Printf("[AUDIO] Expected duration from API: %v", p.expectedDuration)
		}
	} else {
		p.expectedDuration = 0
	}
	p.mu.Unlock()

	go p.loadAndPlay(loadingCtx, song)
	return nil
}

func (p *Player) loadAndPlay(ctx context.Context, song *types.Song) {
	if p.debug {
		log.Printf("[AUDIO] Loading audio for: %s", song.Name)
	}

	select {
	case <-ctx.Done():
		if p.debug {
			log.Printf("[AUDIO] Loading canceled before start for: %s", song.Name)
		}
		return
	default:
	}

	var (
		reader  io.ReadCloser
		err     error
		isLocal bool
	)

	// 1) Explicit local path
	if song.LocalPath != nil && *song.LocalPath != "" {
		if _, statErr := os.Stat(*song.LocalPath); statErr == nil {
			if reader, err = os.Open(*song.LocalPath); err == nil {
				isLocal = true
				if p.debug {
					log.Printf("[AUDIO] Using local file %s", *song.LocalPath)
				}
			}
		}
	}

	// 2) Cached file
	if reader == nil {
		filename := safeFilename(song.Name, song.Slug) + ".mp3"
		candidate := filepath.Join(p.cfg.Storage.CacheDir, "songs", filename)
		if _, statErr := os.Stat(candidate); statErr == nil {
			if p.debug {
				log.Printf("[AUDIO] Found cached file %s", candidate)
			}
			song.LocalPath = &candidate
			song.Downloaded = true

			if reader, err = os.Open(candidate); err == nil {
				isLocal = true
				if p.debug {
					log.Printf("[AUDIO] Using local cached file %s", candidate)
				}
			}
		}
	}

	// 3) Stream from URL
	if reader == nil {
		if p.debug {
			log.Printf("[AUDIO] Streaming %s", song.File)
		}
		reader, err = p.streamManager.CreateStream(ctx, song.File)
		if err != nil {
			if p.debug {
				log.Printf("[AUDIO] Failed to create stream: %v", err)
			}
			return
		}
		isLocal = false
	}

	select {
	case <-ctx.Done():
		reader.Close()
		if p.debug {
			log.Printf("[AUDIO] Loading canceled during setup for: %s", song.Name)
		}
		return
	default:
	}

	// For streaming, wait for initial buffer
	if !isLocal {
		if !p.bufferManager.WaitForSufficientBuffer(ctx, reader) {
			reader.Close()
			if p.debug {
				log.Printf("[AUDIO] Buffer wait failed or canceled for: %s", song.Name)
			}
			return
		}
	}

	// Double-check race
	p.mu.Lock()
	songChanged := p.currentSong == nil || p.currentSong.Slug != song.Slug || p.loadingCanceled
	p.mu.Unlock()
	if songChanged {
		reader.Close()
		if p.debug {
			log.Printf("[AUDIO] Song changed during loading, aborting: %s", song.Name)
		}
		return
	}

	// Decode MP3
	streamer, format, err := mp3.Decode(reader)
	if err != nil {
		if p.debug {
			log.Printf("[AUDIO] Failed to decode MP3 for '%s': %v", song.Name, err)
		}
		reader.Close()
		return
	}
	// IMPORTANT: do NOT defer close here; we close in stop/finish paths.
	// We want the underlying network reader to stay open while we play.

	p.mu.Lock()
	if p.currentSong == nil || p.currentSong.Slug != song.Slug || p.loadingCanceled {
		p.mu.Unlock()
		_ = streamer.Close()
		if p.debug {
			log.Printf("[AUDIO] Song changed after decode, aborting: %s", song.Name)
		}
		return
	}

	// Duration
	var dur time.Duration
	if p.expectedDuration > 0 {
		dur = p.expectedDuration
		if p.debug {
			log.Printf("[AUDIO] Using expected duration from API: %v", dur)
		}
	} else if song.Length > 0 {
		dur = time.Duration(song.Length) * time.Second
	} else {
		// If local and we know length in samples
		if isLocal {
			dur = format.SampleRate.D(streamer.Len())
		}
	}
	p.duration = dur
	p.streamer = streamer // current active streamer (may be replaced on seek)
	p.srcSampleRate = format.SampleRate
	p.baseOffset = 0 // start from beginning for progress tracking

	if p.debug {
		log.Printf("[AUDIO] Audio loaded - Sample Rate: %d, Channels: %d, Duration: %v",
			format.SampleRate, format.NumChannels, dur)
	}

	// Build playback chain
	var source beep.Streamer = streamer
	if format.SampleRate != p.sampleRate {
		source = beep.Resample(4, format.SampleRate, p.sampleRate, streamer)
	}

	p.ctrl = &beep.Ctrl{Streamer: source, Paused: false}
	p.volume = p.mkVolume(p.cfg.Audio.DefaultVolume)

	// Start/replace speaker pipeline
	speaker.Clear()
	done := make(chan struct{})
	seq := beep.Seq(p.volume, beep.Callback(func() { close(done) }))
	speaker.Play(seq)

	p.playing = true
	p.paused = false
	p.position = 0

	// Position tracking with baseOffset support
	p.progressTracker.SetStreamer(p.streamer, p.srcSampleRate, p.expectedDuration, 0)
	if !p.progressTracker.IsRunning() {
		p.progressTracker.Start(p.updatePositionCallback)
	}
	p.mu.Unlock()

	if p.debug {
		log.Printf("[AUDIO] Started playback for '%s' with position tracking", song.Name)
	}

	// Wait for finish or cancellation
	select {
	case <-done:
		if p.shouldTriggerFinished() {
			if p.debug {
				log.Printf("[AUDIO] Playback finished for '%s'", song.Name)
			}
			p.mu.Lock()
			p.playing = false
			p.paused = false
			cb := p.finishedCallback
			// Close the active streamer
			if p.streamer != nil {
				_ = p.streamer.Close()
				p.streamer = nil
			}
			p.mu.Unlock()

			if cb != nil {
				fyne.Do(cb)
			}
		} else {
			if p.debug {
				log.Printf("[AUDIO] Playback ended early for '%s', not triggering finished", song.Name)
			}
			p.mu.Lock()
			p.playing = false
			p.paused = false
			// Close the active streamer
			if p.streamer != nil {
				_ = p.streamer.Close()
				p.streamer = nil
			}
			p.mu.Unlock()
		}
	case <-ctx.Done():
		if p.debug {
			log.Printf("[AUDIO] Playback cancelled for '%s'", song.Name)
		}
		return
	}
}

func (p *Player) updatePositionCallback(pos time.Duration) {
	p.mu.Lock()
	p.position = pos
	p.lastPosition = pos
	callback := p.positionCallback
	p.mu.Unlock()

	if callback != nil {
		// Ensure UI updates happen on the main thread
		fyne.Do(func() {
			callback(pos)
		})
	}
}

func (p *Player) shouldTriggerFinished() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	currentPos := p.position
	expectedDur := p.expectedDuration
	actualDur := p.duration
	playTime := time.Since(p.playbackStartTime)

	if playTime < p.minPlayTime {
		if p.debug {
			log.Printf("[AUDIO] Playback too short (%.1fs), not triggering finished", playTime.Seconds())
		}
		return false
	}

	if expectedDur > 0 {
		playedPercent := float64(currentPos) / float64(expectedDur)
		if playedPercent >= p.completionThreshold {
			if p.debug {
				log.Printf("[AUDIO] Played %.1f%% of expected duration, triggering finished", playedPercent*100)
			}
			return true
		} else {
			if p.debug {
				log.Printf("[AUDIO] Only played %.1f%% of expected duration, not triggering finished", playedPercent*100)
			}
			return false
		}
	}

	if actualDur > 0 {
		playedPercent := float64(currentPos) / float64(actualDur)
		if playedPercent >= p.completionThreshold {
			return true
		}
	}

	return false
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil && p.playing && !p.paused {
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
		p.paused = true

		// Stop position tracking when paused
		if p.progressTracker != nil && p.progressTracker.IsRunning() {
			p.progressTracker.Stop()
		}

		if p.debug {
			log.Printf("[AUDIO] Paused playback at position: %v", p.position)
		}
	}
	return nil
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil && p.playing && p.paused {
		speaker.Lock()
		p.ctrl.Paused = false
		speaker.Unlock()
		p.paused = false

		// Restart position tracking when resumed
		if p.progressTracker != nil && !p.progressTracker.IsRunning() {
			p.progressTracker.Start(p.updatePositionCallback)
		}

		if p.debug {
			log.Printf("[AUDIO] Resumed playback from position: %v", p.position)
		}
	}
	return nil
}

func (p *Player) stopInternal() {
	// Stop position tracking first
	if p.progressTracker != nil {
		p.progressTracker.Stop()
	}

	if p.playing || p.paused {
		speaker.Clear()
	}

	if p.streamer != nil {
		_ = p.streamer.Close()
		p.streamer = nil
	}

	// Close and forget any active network streams
	if p.streamManager != nil {
		p.streamManager.CleanupStreams()
	}
	p.activeStream = nil
	p.baseOffset = 0

	p.ctrl = nil
	p.volume = nil
	p.position = 0
	p.duration = 0
	p.expectedDuration = 0
	p.playing = false
	p.paused = false

	if p.debug {
		log.Printf("[AUDIO] Playback stopped and resources cleaned")
	}
}

func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopInternal()
	p.currentSong = nil

	if p.debug {
		log.Printf("[AUDIO] Stopped playback")
	}
	return nil
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentSong == nil || p.ctrl == nil {
		if p.debug {
			log.Printf("[AUDIO] Cannot seek: no current song or control")
		}
		return fmt.Errorf("no active stream")
	}

	target := position
	if p.expectedDuration > 0 && target > p.expectedDuration {
		target = p.expectedDuration
	}
	if target < 0 {
		target = 0
	}

	// Try native seek first if the current streamer knows its length (local or seekable)
	if p.streamer != nil && p.streamer.Len() > 0 && p.srcSampleRate > 0 {
		// Convert duration to sample index safely
		targetSample := p.srcSampleRate.N(target)
		if targetSample < 0 {
			targetSample = 0
		}
		if l := p.streamer.Len(); l > 0 && targetSample >= l {
			targetSample = l - 1
		}

		speaker.Lock()
		err := p.streamer.Seek(targetSample)
		speaker.Unlock()

		if err == nil {
			p.position = target
			p.lastPosition = target
			p.baseOffset = 0 // native seek means absolute position is the streamer's own position
			if p.progressTracker != nil {
				p.progressTracker.SetStreamer(p.streamer, p.srcSampleRate, p.expectedDuration, 0)
			}
			if p.debug {
				log.Printf("[AUDIO] Native seek to %v (sample=%d)", target, targetSample)
			}
			return nil
		}

		if p.debug {
			log.Printf("[AUDIO] Native seek failed: %v", err)
		}
		// Fall through to buffered re-decode below
	}

	// Buffered re-decode path (streaming without native seek).
	// We rebuild a decoder from already-downloaded bytes and splice it into the pipeline.
	if p.currentSong.File == "" {
		return fmt.Errorf("seek not supported")
	}
	sr, ok := p.streamManager.GetStream(p.currentSong.File)
	if !ok {
		if p.debug {
			log.Printf("[AUDIO] No active stream to buffered-seek")
		}
		return fmt.Errorf("seek not supported")
	}

	// Compute desired byte offset by ratio. Clamp to downloaded region.
	var totalBytes int64
	sr.mutex.RLock()
	totalBytes = sr.totalSize
	downloaded := sr.downloaded
	bufLen := int64(len(sr.buffer))
	sr.mutex.RUnlock()

	if totalBytes <= 0 {
		totalBytes = bufLen
	}
	if totalBytes <= 0 || bufLen <= 0 {
		return fmt.Errorf("buffer not available yet")
	}

	var ratio float64
	if p.expectedDuration > 0 {
		ratio = float64(target) / float64(p.expectedDuration)
	} else if p.duration > 0 {
		ratio = float64(target) / float64(p.duration)
	} else {
		// Fallback: assume zero if unknown
		ratio = 0
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	wantOffset := int64(ratio * float64(totalBytes))

	// Restrict to what we actually have in memory now
	maxAvailable := downloaded
	if maxAvailable > bufLen {
		maxAvailable = bufLen
	}
	if wantOffset > maxAvailable-1 {
		wantOffset = maxAvailable - 1
	}
	if wantOffset < 0 {
		wantOffset = 0
	}

	// Build a zero-copy reader on the buffered slice starting at wantOffset.
	sr.mutex.RLock()
	segment := sr.buffer[wantOffset:]
	sr.mutex.RUnlock()
	if len(segment) == 0 {
		return fmt.Errorf("no buffered data at requested position")
	}

	segmentReader := io.NopCloser(bytes.NewReader(segment))

	// Decode a new mp3 streamer from the buffered segment
	newStreamer, newFormat, err := mp3.Decode(segmentReader)
	if err != nil {
		if p.debug {
			log.Printf("[AUDIO] Buffered decode failed at offset %d: %v", wantOffset, err)
		}
		return err
	}

	// Prepare resampling if needed
	var newSource beep.Streamer = newStreamer
	if newFormat.SampleRate != p.sampleRate {
		newSource = beep.Resample(4, newFormat.SampleRate, p.sampleRate, newStreamer)
	}

	// Swap into the live pipeline
	wasPaused := p.paused
	speaker.Lock()
	p.ctrl.Streamer = newSource
	p.paused = wasPaused
	speaker.Unlock()

	// Update tracking to account for baseOffset (absolute position from track start)
	p.streamer = newStreamer
	p.srcSampleRate = newFormat.SampleRate
	p.position = target
	p.lastPosition = target
	p.baseOffset = target

	if p.progressTracker != nil {
		p.progressTracker.SetStreamer(newStreamer, p.srcSampleRate, p.expectedDuration, p.baseOffset)
	}

	if p.debug {
		percent := ratio * 100
		log.Printf("[AUDIO] Buffered-seek to %v (~%d/%d bytes, %.1f%%)",
			target, wantOffset, totalBytes, percent)
	}

	return nil
}

func (p *Player) CanSeek() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.streamer == nil {
		return false
	}

	// If the streamer knows its length, we can natively seek.
	if p.streamer.Len() > 0 {
		return true
	}

	// Streaming: allow seeking within the buffered region
	if p.currentSong != nil && p.currentSong.File != "" {
		if sr, ok := p.streamManager.GetStream(p.currentSong.File); ok {
			sr.mutex.RLock()
			hasData := len(sr.buffer) > 0
			sr.mutex.RUnlock()
			return hasData
		}
	}

	return false
}

func (p *Player) GetSeekableRange() (time.Duration, time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.streamer == nil {
		return 0, 0
	}

	minSeek := time.Duration(0)

	// If we have exact knowledge (local or seekable), use full duration.
	if p.streamer.Len() > 0 {
		maxSeek := p.GetDuration()
		return minSeek, maxSeek
	}

	// Streaming: limit to downloaded portion
	if p.currentSong != nil && p.currentSong.File != "" {
		if sr, ok := p.streamManager.GetStream(p.currentSong.File); ok {
			sr.mutex.RLock()
			total := sr.totalSize
			if total <= 0 {
				total = int64(len(sr.buffer))
			}
			dl := sr.downloaded
			if dl > int64(len(sr.buffer)) {
				dl = int64(len(sr.buffer))
			}
			sr.mutex.RUnlock()

			if total > 0 && p.expectedDuration > 0 {
				progress := float64(dl) / float64(total)
				if progress > 1 {
					progress = 1
				}
				return minSeek, time.Duration(progress * float64(p.expectedDuration))
			}
		}
	}

	// Fallback: unknown, assume nothing seekable
	return 0, 0
}

func (p *Player) HasSufficientBuffer(position time.Duration) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	downloadProgress := p.streamManager.GetDownloadProgress()
	if downloadProgress >= 1.0 {
		return true // Fully downloaded
	}

	if p.expectedDuration <= 0 {
		return downloadProgress > 0.05 // At least 5% buffer
	}

	requiredProgress := float64(position) / float64(p.expectedDuration)
	bufferMargin := 0.05 // 5% additional buffer

	return downloadProgress >= requiredProgress+bufferMargin
}

func (p *Player) SetVolume(level float64) error {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.volume == nil {
		return nil
	}

	speaker.Lock()
	if level == 0 {
		p.volume.Silent = true
	} else {
		p.volume.Silent = false
		p.volume.Volume = (level - 1) * 5
	}
	speaker.Unlock()
	return nil
}

func (p *Player) GetPosition() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.position
}

func (p *Player) GetDuration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.expectedDuration > 0 {
		return p.expectedDuration
	}
	return p.duration
}

func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.playing && !p.paused && p.ctrl != nil
}

func (p *Player) OnPositionChanged(callback func(time.Duration)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positionCallback = callback
}

func (p *Player) OnFinished(callback func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishedCallback = callback
}

func (p *Player) GetDownloadProgress() float64 {
	return p.streamManager.GetDownloadProgress()
}

func (p *Player) GetCurrentSong() *types.Song {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentSong
}

func (p *Player) Close() error {
	if p.debug {
		log.Printf("[AUDIO] Closing player")
	}

	p.mu.Lock()
	if p.done != nil {
		close(p.done)
		p.done = nil
	}
	p.mu.Unlock()

	p.progressTracker.Stop()
	p.streamManager.Close()

	return p.Stop()
}

func safeFilename(name, slug string) string {
	if slug != "" {
		return slug
	}
	safe := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-", "?", "-",
		"\"", "-", "<", "-", ">", "-", "|", "-",
	).Replace(name)
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return safe
}

type nonCancelReadCloser struct{ r io.Reader }

func (n *nonCancelReadCloser) Read(p []byte) (int, error) { return n.r.Read(p) }
func (n *nonCancelReadCloser) Close() error {
	return nil // No-op, we don't want to cancel the read
}
