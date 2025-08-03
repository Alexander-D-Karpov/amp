package audio

import (
	"context"
	"fmt"
	"io"
	"log"
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

var speakerInitialized = false
var speakerMutex sync.Mutex

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
}

func NewPlayer(cfg *config.Config, storage *storage.Database) (*Player, error) {
	p := &Player{
		cfg:        cfg,
		storage:    storage,
		done:       make(chan struct{}),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		sampleRate: beep.SampleRate(cfg.Audio.SampleRate),
		debug:      cfg.Debug,
		playing:    false,
		paused:     false,
	}

	if err := p.initializeSpeaker(); err != nil {
		return nil, fmt.Errorf("failed to initialize speaker: %w", err)
	}

	p.ticker = time.NewTicker(100 * time.Millisecond)
	go p.positionUpdater()

	if p.debug {
		log.Printf("[AUDIO] Player initialized successfully on %s with sample rate: %d", runtime.GOOS, p.sampleRate)
	}

	return p, nil
}

func (p *Player) initializeSpeaker() error {
	speakerMutex.Lock()
	defer speakerMutex.Unlock()

	if speakerInitialized {
		if p.debug {
			log.Printf("[AUDIO] Speaker already initialized")
		}
		return nil
	}

	bufferSize := p.sampleRate.N(time.Second / 10)

	if runtime.GOOS == "linux" {
		if p.debug {
			log.Printf("[AUDIO] Initializing speaker for Linux with optimized settings")
		}
		bufferSize = p.sampleRate.N(time.Second / 5)
	}

	if p.debug {
		log.Printf("[AUDIO] Initializing speaker with sample rate %d, buffer size %d on %s",
			p.sampleRate, bufferSize, runtime.GOOS)
	}

	err := speaker.Init(p.sampleRate, bufferSize)
	if err != nil {
		return fmt.Errorf("speaker initialization failed: %w", err)
	}

	speakerInitialized = true
	if p.debug {
		log.Printf("[AUDIO] Speaker initialized successfully")
	}
	return nil
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

	if song.LocalPath != nil && *song.LocalPath != "" {
		if _, statErr := os.Stat(*song.LocalPath); statErr == nil {
			reader, err = os.Open(*song.LocalPath)
			if p.debug {
				log.Printf("[AUDIO] Using local file: %s", *song.LocalPath)
			}
		}
	}

	if reader == nil {
		if p.debug {
			log.Printf("[AUDIO] Streaming from URL: %s", song.File)
		}
		reader, err = p.streamFromURL(ctx, song.File)
	}

	if err != nil {
		if p.debug {
			log.Printf("[AUDIO] Failed to get audio stream for '%s': %v", song.Name, err)
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

	duration := format.SampleRate.D(streamer.Len())
	p.duration = duration
	p.streamer = streamer

	if p.debug {
		log.Printf("[AUDIO] Audio format - Sample Rate: %d, Channels: %d, Length: %d samples, Duration: %v",
			format.SampleRate, format.NumChannels, streamer.Len(), duration)
	}

	resampled := beep.Resample(4, format.SampleRate, p.sampleRate, streamer)
	p.ctrl = &beep.Ctrl{Streamer: resampled, Paused: false}
	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Volume:   (p.cfg.Audio.DefaultVolume - 1) * 5,
		Silent:   p.cfg.Audio.DefaultVolume == 0,
	}

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
		log.Printf("[AUDIO] Started playback for '%s', duration: %v", song.Name, duration)
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
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	if p.debug {
		log.Printf("[AUDIO] Successfully opened stream, Content-Length: %s", resp.Header.Get("Content-Length"))
	}

	return resp.Body, nil
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

	p.ctrl = nil
	p.volume = nil
	p.position = 0
	p.duration = 0
	p.playing = false
	p.paused = false

	if p.debug {
		log.Printf("[AUDIO] Audio playback stopped and cleaned up")
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

func (p *Player) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.volume != nil {
		p.volume.Volume = (volume - 1) * 5
		p.volume.Silent = volume == 0

		if p.debug {
			log.Printf("[AUDIO] Volume set to: %.2f", volume)
		}
	}
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
	if p.streamer != nil && p.playing && !p.paused {
		currentPos := p.sampleRate.D(p.streamer.Position())
		if currentPos != p.position {
			p.position = currentPos
			callback := p.positionCallback
			pos := p.position
			p.mu.Unlock()

			if callback != nil {
				fyne.Do(func() {
					callback(pos)
				})
			}
		} else {
			p.mu.Unlock()
		}
	} else {
		p.mu.Unlock()
	}
}
