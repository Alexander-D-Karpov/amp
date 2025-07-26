package audio

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Player struct {
	mu               sync.RWMutex
	cfg              *config.Config
	currentSong      *types.Song
	streamer         beep.StreamSeekCloser
	ctrl             *beep.Ctrl
	volume           *effects.Volume
	position         time.Duration
	duration         time.Duration
	isPlaying        bool
	positionCallback func(time.Duration)
	finishedCallback func()
	sampleRate       beep.SampleRate
	ticker           *time.Ticker
	done             chan struct{}
}

func NewPlayer(cfg *config.Config) (*Player, error) {
	sampleRate := beep.SampleRate(cfg.Audio.SampleRate)

	err := speaker.Init(sampleRate, sampleRate.N(time.Second/10))
	if err != nil {
		return nil, fmt.Errorf("init speaker: %w", err)
	}

	p := &Player{
		cfg:        cfg,
		sampleRate: sampleRate,
		done:       make(chan struct{}),
	}

	p.ticker = time.NewTicker(100 * time.Millisecond)
	go p.positionUpdater()

	return p, nil
}

func (p *Player) Play(ctx context.Context, song *types.Song) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer != nil {
		_ = p.streamer.Close()
		speaker.Clear()
	}

	var reader io.ReadCloser
	var err error

	if song.LocalPath != nil && *song.LocalPath != "" {
		reader, err = os.Open(*song.LocalPath)
		if err != nil {
			return fmt.Errorf("open local file: %w", err)
		}
	} else {
		return fmt.Errorf("no local file or streaming not implemented")
	}
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	streamer, format, err := mp3.Decode(reader)
	if err != nil {
		return fmt.Errorf("decode MP3: %w", err)
	}

	resampled := beep.Resample(4, format.SampleRate, p.sampleRate, streamer)

	p.ctrl = &beep.Ctrl{Streamer: resampled, Paused: false}
	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Volume:   0,
		Silent:   false,
	}

	_ = p.SetVolume(p.cfg.Audio.DefaultVolume)

	p.streamer = streamer
	p.currentSong = song
	p.duration = format.SampleRate.D(streamer.Len())
	p.position = 0
	p.isPlaying = true

	speaker.Play(beep.Seq(p.volume, beep.Callback(func() {
		p.mu.Lock()
		p.isPlaying = false
		p.mu.Unlock()

		if p.finishedCallback != nil {
			p.finishedCallback()
		}
	})))

	return nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil {
		p.ctrl.Paused = true
		p.isPlaying = false
	}

	return nil
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil {
		p.ctrl.Paused = false
		p.isPlaying = true
	}

	return nil
}

func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer != nil {
		_ = p.streamer.Close()
		p.streamer = nil
	}

	speaker.Clear()
	p.ctrl = nil
	p.volume = nil
	p.currentSong = nil
	p.position = 0
	p.duration = 0
	p.isPlaying = false

	return nil
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer == nil {
		return fmt.Errorf("no song playing")
	}

	samplePos := p.sampleRate.N(position)
	if samplePos < 0 {
		samplePos = 0
	}
	if samplePos >= p.streamer.Len() {
		samplePos = p.streamer.Len() - 1
	}

	if err := p.streamer.Seek(samplePos); err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	p.position = position
	return nil
}

func (p *Player) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}

	if p.volume != nil {
		speaker.Lock()
		p.volume.Volume = (volume - 1) * 10
		p.volume.Silent = volume == 0
		speaker.Unlock()
	}

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
	return p.duration
}

func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isPlaying
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
	close(p.done)
	p.ticker.Stop()
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

	if p.streamer != nil && p.isPlaying && !p.ctrl.Paused {
		currentPos := p.streamer.Position()
		p.position = p.sampleRate.D(currentPos)

		if p.positionCallback != nil {
			p.positionCallback(p.position)
		}
	}
}
