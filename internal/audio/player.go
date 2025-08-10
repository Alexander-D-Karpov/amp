package audio

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
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
)

var (
	speakerInitialized = false
	speakerMutex       sync.Mutex
	globalSampleRate   beep.SampleRate
	speakerOnce        sync.Once
)

type Player struct {
	mu sync.Mutex

	cfg              *config.Config
	storage          *storage.Database
	currentSong      *types.Song
	streamer         beep.StreamSeekCloser
	ctrl             *beep.Ctrl
	volume           *effects.Volume
	position         time.Duration
	duration         time.Duration
	positionCallback func(time.Duration)
	finishedCallback func()
	sampleRate       beep.SampleRate
	ticker           *time.Ticker
	done             chan struct{}
	httpClient       *http.Client
	debug            bool
	playing          bool
	paused           bool
	bufferSize       int

	downloadBuffer []byte
	downloadMu     sync.RWMutex
	downloadPos    int64
	totalSize      int64
	isStreaming    bool
	streamReader   *ProgressiveReader
}

type ProgressiveReader struct {
	url          string
	buffer       []byte
	totalSize    int64
	downloaded   int64
	mu           sync.RWMutex
	httpClient   *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	downloadDone bool
	lastRead     time.Time
}

func NewPlayer(cfg *config.Config, storage *storage.Database) (*Player, error) {
	p := &Player{
		cfg:     cfg,
		storage: storage,
		done:    make(chan struct{}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 15 * time.Second,
				MaxIdleConns:        10,
			},
		},
		sampleRate: beep.SampleRate(cfg.Audio.SampleRate),
		debug:      cfg.Debug,
		playing:    false,
		paused:     false,
	}

	p.bufferSize = p.calculateOptimalBufferSize()

	if err := p.initializeSpeaker(); err != nil {
		return nil, fmt.Errorf("failed to initialize speaker: %w", err)
	}

	p.ticker = time.NewTicker(50 * time.Millisecond)
	go p.positionUpdater()

	if p.debug {
		log.Printf("[AUDIO] Player initialized - OS: %s, Sample Rate: %d, Buffer: %d",
			runtime.GOOS, p.sampleRate, p.bufferSize)
	}

	return p, nil
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
		log.Printf("[AUDIO] Starting playback for: %s", song.Name)
	}

	p.mu.Lock()
	p.stopInternal()
	p.currentSong = song
	p.playing = false
	p.paused = false
	p.position = 0
	p.mu.Unlock()

	go p.loadAndPlay(ctx, song)
	return nil
}

func (p *Player) loadAndPlay(ctx context.Context, song *types.Song) {
	if p.debug {
		log.Printf("[AUDIO] Loading audio for: %s", song.Name)
	}

	var reader io.ReadCloser
	var err error
	var isLocal bool

	if song.LocalPath != nil && *song.LocalPath != "" {
		if _, err := os.Stat(*song.LocalPath); err == nil {
			reader, err = os.Open(*song.LocalPath)
			isLocal = true
			if p.debug {
				log.Printf("[AUDIO] Using local file %s", *song.LocalPath)
			}
		}
	}

	if reader == nil {
		p.mu.Lock()
		p.isStreaming = true
		p.mu.Unlock()

		if p.debug {
			log.Printf("[AUDIO] Streaming %s", song.File)
		}

		reader, err = p.streamFromURLWithRetry(ctx, song.File, 3)
		if err != nil {
			if p.debug {
				log.Printf("[AUDIO] Failed to stream after retries: %v", err)
			}
			return
		}
	}

	if reader == nil {
		if p.debug {
			log.Printf("[AUDIO] No audio source available for '%s'", song.Name)
		}
		return
	}
	defer reader.Close()

	streamer, format, err := mp3.Decode(reader)
	if err != nil {
		if p.debug {
			log.Printf("[AUDIO] Failed to decode MP3 for '%s': %v", song.Name, err)
		}
		return
	}
	defer streamer.Close()

	p.mu.Lock()
	if p.currentSong == nil || p.currentSong.Slug != song.Slug {
		p.mu.Unlock()
		if p.debug {
			log.Printf("[AUDIO] Song changed during loading, aborting")
		}
		return
	}

	var duration time.Duration
	if song.Length > 0 {
		duration = time.Duration(song.Length) * time.Second
	} else if isLocal {
		duration = format.SampleRate.D(streamer.Len())
	}

	p.duration = duration
	p.streamer = streamer

	if p.debug {
		log.Printf("[AUDIO] Audio loaded - Sample Rate: %d, Channels: %d, Duration: %v",
			format.SampleRate, format.NumChannels, duration)
	}

	var resampled beep.Streamer
	if format.SampleRate != p.sampleRate {
		resampled = beep.Resample(4, format.SampleRate, p.sampleRate, streamer)
		if p.debug {
			log.Printf("[AUDIO] Resampling from %d to %d", format.SampleRate, p.sampleRate)
		}
	} else {
		resampled = streamer
	}

	p.ctrl = &beep.Ctrl{Streamer: resampled, Paused: false}
	p.volume = p.mkVolume(p.cfg.Audio.DefaultVolume)

	speaker.Clear()

	doneChan := make(chan struct{})
	playbackSequence := beep.Seq(p.volume, beep.Callback(func() {
		close(doneChan)
	}))

	speaker.Play(playbackSequence)
	p.playing = true
	p.paused = false
	p.position = 0

	p.mu.Unlock()

	if p.debug {
		log.Printf("[AUDIO] Started playback for '%s'", song.Name)
	}

	select {
	case <-doneChan:
		if p.debug {
			log.Printf("[AUDIO] Playback finished for '%s'", song.Name)
		}
	case <-ctx.Done():
		if p.debug {
			log.Printf("[AUDIO] Playback cancelled for '%s'", song.Name)
		}
		return
	}

	p.mu.Lock()
	p.playing = false
	p.paused = false
	if p.finishedCallback != nil {
		callback := p.finishedCallback
		p.mu.Unlock()
		fyne.Do(func() {
			callback()
		})
	} else {
		p.mu.Unlock()
	}
}

func (p *Player) streamFromURLWithRetry(ctx context.Context, url string, maxRetries int) (io.ReadCloser, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			if p.debug {
				log.Printf("[AUDIO] Retry attempt %d/%d for %s", attempt+1, maxRetries, url)
			}
			sleepTime := time.Duration(attempt) * time.Second
			select {
			case <-time.After(sleepTime):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		reader, err := p.streamFromURL(ctx, url)
		if err == nil {
			return reader, nil
		}
		lastErr = err

		if p.debug {
			log.Printf("[AUDIO] Stream attempt %d failed: %v", attempt+1, err)
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (p *Player) streamFromURL(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "AMP Music Player/1.0")
	req.Header.Set("Accept", "audio/mpeg, audio/*")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	p.streamReader = &ProgressiveReader{
		url:        url,
		buffer:     make([]byte, 0, 10*1024*1024), // Pre-allocate 10MB
		httpClient: p.httpClient,
		lastRead:   time.Now(),
	}

	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if size, parseErr := parseContentLength(contentLength); parseErr == nil {
			p.streamReader.totalSize = size
			p.totalSize = size
		}
	}

	p.streamReader.ctx, p.streamReader.cancel = context.WithCancel(ctx)
	go p.streamReader.download(resp)

	if p.debug {
		log.Printf("[AUDIO] Stream opened successfully, Content-Length: %s",
			resp.Header.Get("Content-Length"))
	}

	return &streamWrapper{reader: p.streamReader}, nil
}

func parseContentLength(s string) (int64, error) {
	var result int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int64(c-'0')
		} else {
			return 0, fmt.Errorf("invalid character")
		}
	}
	return result, nil
}

type streamWrapper struct {
	reader *ProgressiveReader
}

func (sw *streamWrapper) Read(p []byte) (int, error) {
	return sw.reader.Read(p)
}

func (sw *streamWrapper) Close() error {
	if sw.reader.cancel != nil {
		sw.reader.cancel()
	}
	return nil
}

func (pr *ProgressiveReader) download(resp *http.Response) {
	defer resp.Body.Close()

	buffer := make([]byte, 32768)
	for {
		select {
		case <-pr.ctx.Done():
			pr.mu.Lock()
			pr.downloadDone = true
			pr.mu.Unlock()
			return
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])

			pr.mu.Lock()
			pr.buffer = append(pr.buffer, chunk...)
			pr.downloaded += int64(n)
			pr.mu.Unlock()
		}

		if err != nil {
			pr.mu.Lock()
			pr.downloadDone = true
			pr.mu.Unlock()
			if err == io.EOF {
				break
			}
			return
		}
	}
}

func (pr *ProgressiveReader) Read(p []byte) (int, error) {
	// Wait for initial buffering
	for i := 0; i < 100; i++ {
		pr.mu.RLock()
		available := len(pr.buffer)
		pr.mu.RUnlock()

		if available >= len(p) || available >= 65536 {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	pr.mu.Lock()
	defer pr.mu.Unlock()

	available := len(pr.buffer)
	if available == 0 {
		if pr.downloadDone {
			return 0, io.EOF
		}
		return 0, nil
	}

	n := len(p)
	if n > available {
		n = available
	}

	copy(p, pr.buffer[:n])
	pr.buffer = append([]byte{}, pr.buffer[n:]...)
	pr.lastRead = time.Now()

	return n, nil
}

func (p *Player) stopInternal() {
	if p.playing || p.paused {
		speaker.Clear()
	}

	if p.streamer != nil {
		if closeErr := p.streamer.Close(); closeErr != nil {
			if p.debug {
				log.Printf("[AUDIO] Error closing streamer: %v", closeErr)
			}
		}
		p.streamer = nil
	}

	if p.streamReader != nil && p.streamReader.cancel != nil {
		p.streamReader.cancel()
		p.streamReader = nil
	}

	p.ctrl = nil
	p.volume = nil
	p.position = 0
	p.duration = 0
	p.playing = false
	p.paused = false
	p.isStreaming = false

	if p.debug {
		log.Printf("[AUDIO] Playback stopped and resources cleaned")
	}
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing && !p.paused && p.ctrl != nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil && p.playing && !p.paused {
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
		p.paused = true

		if p.debug {
			log.Printf("[AUDIO] Paused playback")
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

		if p.debug {
			log.Printf("[AUDIO] Resumed playback")
		}
	}
	return nil
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

	if p.streamer != nil && p.playing {
		pos := p.sampleRate.N(position)
		if pos >= 0 && pos < p.streamer.Len() {
			speaker.Lock()
			err := p.streamer.Seek(pos)
			speaker.Unlock()

			if err != nil {
				if p.debug {
					log.Printf("[AUDIO] Seek failed: %v", err)
				}
				return err
			}

			p.position = position

			if p.debug {
				log.Printf("[AUDIO] Seeked to position: %v", position)
			}
		}
	}
	return nil
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
		p.volume.Volume = math.Log2(level)
	}
	speaker.Unlock()
	return nil
}

func (p *Player) GetPosition() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.position
}

func (p *Player) GetDuration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duration
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

func (p *Player) Close() error {
	if p.debug {
		log.Printf("[AUDIO] Closing player")
	}

	close(p.done)
	if p.ticker != nil {
		p.ticker.Stop()
	}
	return p.Stop()
}

func (p *Player) positionUpdater() {
	for {
		select {
		case <-p.ticker.C:
			p.updatePosition()
		case <-p.done:
			return
		}
	}
}

func (p *Player) updatePosition() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer != nil && p.playing && !p.paused {
		currentPos := p.sampleRate.D(p.streamer.Position())

		if currentPos != p.position {
			p.position = currentPos
			if p.positionCallback != nil {
				callback := p.positionCallback
				pos := p.position
				go func() {
					fyne.Do(func() {
						callback(pos)
					})
				}()
			}
		}
	}
}

func (p *Player) GetDownloadProgress() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isStreaming || p.streamReader == nil {
		return 1.0
	}

	p.streamReader.mu.RLock()
	defer p.streamReader.mu.RUnlock()

	if p.streamReader.totalSize == 0 {
		return 0.0
	}

	return float64(p.streamReader.downloaded) / float64(p.streamReader.totalSize)
}

func (p *Player) GetCurrentSong() *types.Song {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentSong
}
