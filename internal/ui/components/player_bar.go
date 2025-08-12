package components

import (
	"context"
	"fmt"
	"github.com/Alexander-D-Karpov/amp/internal/services"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/audio"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type PlayerBar struct {
	player  *audio.Player
	storage *storage.Database
	cfg     *config.Config

	container      *fyne.Container
	playBtn        *widget.Button
	prevBtn        *widget.Button
	nextBtn        *widget.Button
	shuffleBtn     *widget.Button
	repeatBtn      *widget.Button
	likeBtn        *widget.Button
	seekBar        *widget.Slider
	bufferProgress *bufferBar
	waveform       *waveformBar
	volumeBar      *widget.Slider
	volumeBtn      *widget.Button
	timeLabel      *widget.Label
	songLabel      *widget.Label
	artistLabel    *widget.Label
	imageService   *services.ImageService
	coverImg       *canvas.Image
	volumeDialog   dialog.Dialog
	closeBtn       *widget.Button

	seekStack *fyne.Container

	currentSong   *types.Song
	isPlaying     bool
	loading       bool
	loadingStopCh chan struct{}
	isShuffled    bool
	repeatMode    RepeatMode
	queue         []*types.Song
	queueIndex    int
	compactMode   bool
	breakpoint    float32

	currentHeight float32
	desiredHeight float32
	minHeight     float32
	maxHeight     float32
	screenSize    fyne.Size

	onNext                  func()
	onPrevious              func()
	onShuffle               func(bool)
	onRepeat                func(RepeatMode)
	seekingProgrammatically bool
	userSeeking             bool
	parentWindow            fyne.Window
	lastPosition            time.Duration
	lastDuration            time.Duration
	loadingLabel            *widget.Label
	onPlayed                func(*types.Song)
	onPrefetchNext          func(*types.Song)

	playStartTime   time.Time
	minPlayDuration time.Duration
	debug           bool
	statusLabel     *widget.Label
}

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatOne
	RepeatAll
)

func (r RepeatMode) String() string {
	switch r {
	case RepeatOff:
		return "Off"
	case RepeatOne:
		return "One"
	case RepeatAll:
		return "All"
	default:
		return "Off"
	}
}

func NewPlayerBar(player *audio.Player, storage *storage.Database, imageService *services.ImageService, debug bool) *PlayerBar {
	pb := &PlayerBar{
		player:          player,
		storage:         storage,
		imageService:    imageService,
		queue:           make([]*types.Song, 0),
		queueIndex:      -1,
		breakpoint:      800.0,
		minHeight:       54.0,
		maxHeight:       132.0,
		screenSize:      fyne.NewSize(800, 54),
		minPlayDuration: 30 * time.Second, // Minimum 30 seconds to count as played
		debug:           debug,
	}
	pb.setupWidgets()
	pb.setupLayout()
	pb.setupEventHandlers()
	pb.calculateDesiredHeight()
	return pb
}

func (pb *PlayerBar) SetConfig(cfg *config.Config) {
	pb.cfg = cfg
}

func (pb *PlayerBar) SetParentWindow(window fyne.Window) {
	pb.parentWindow = window
}

func (pb *PlayerBar) SetScreenSize(size fyne.Size) {
	pb.screenSize = size
	pb.compactMode = size.Width < pb.breakpoint
	pb.calculateDesiredHeight()
	pb.setupLayout()
}

func (pb *PlayerBar) calculateDesiredHeight() {
	if pb.compactMode {
		pb.desiredHeight = pb.screenSize.Height / 6
		if pb.desiredHeight < pb.minHeight {
			pb.desiredHeight = pb.minHeight
		}
		if pb.desiredHeight > pb.maxHeight {
			pb.desiredHeight = pb.maxHeight
		}
	} else {
		pb.desiredHeight = pb.minHeight
	}
	pb.currentHeight = pb.desiredHeight
}

func (pb *PlayerBar) setupWidgets() {
	pb.playBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), pb.togglePlay)
	pb.prevBtn = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), pb.previousSong)
	pb.nextBtn = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), pb.nextSong)

	pb.closeBtn = widget.NewButtonWithIcon("", theme.CancelIcon(), pb.closeAndHide)
	pb.closeBtn.Importance = widget.LowImportance

	pb.likeBtn = widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), nil)
	pb.likeBtn.Hide()

	pb.volumeBar = widget.NewSlider(0, 100)
	pb.volumeBar.SetValue(70)
	pb.volumeBar.OnChanged = pb.onVolumeChange
	pb.volumeBtn = widget.NewButtonWithIcon("", volumeIconFor(pb.volumeBar.Value), pb.showVolumeDialog)

	pb.timeLabel = widget.NewLabel("0:00 / 0:00")
	pb.timeLabel.TextStyle = fyne.TextStyle{Monospace: true}
	pb.loadingLabel = widget.NewLabel("")
	pb.loadingLabel.Hide()

	pb.seekBar = widget.NewSlider(0, 100)
	pb.seekBar.OnChanged = pb.onSeekChanged
	pb.seekBar.OnChangeEnded = pb.onSeekEnded

	pb.bufferProgress = newBufferBar()
	pb.bufferProgress.Hide()

	pb.songLabel = widget.NewLabel("No song playing")
	pb.songLabel.TextStyle = fyne.TextStyle{Bold: true}
	pb.songLabel.Truncation = fyne.TextTruncateEllipsis

	pb.artistLabel = widget.NewLabel("")
	pb.artistLabel.Truncation = fyne.TextTruncateEllipsis

	pb.setupSeekBar()
	pb.setupStatusLabel()

	pb.coverImg = canvas.NewImageFromResource(theme.MediaMusicIcon())
}

func (pb *PlayerBar) setupLayout() {
	if pb.container == nil {
		pb.container = container.NewStack()
	} else {
		pb.container.Objects = nil
	}

	if pb.compactMode {
		pb.setupCompactLayout()
	} else {
		pb.setupDesktopLayout()
	}

	pb.container.Resize(fyne.NewSize(pb.container.Size().Width, pb.desiredHeight))
	pb.container.Refresh()
}

func (pb *PlayerBar) setupDesktopLayout() {
	coverSize := fyne.NewSize(pb.desiredHeight-10, pb.desiredHeight-10)
	pb.coverImg.Resize(coverSize)
	info := container.NewVBox(pb.songLabel, pb.artistLabel)
	infoWrap := container.NewGridWrap(fyne.NewSize(260, coverSize.Height), info)

	left := container.NewHBox(pb.coverImg, infoWrap)

	controls := container.NewHBox(pb.prevBtn, pb.playBtn, pb.nextBtn)

	volWidth := float32(200)
	volWrap := container.NewGridWrap(fyne.NewSize(volWidth, pb.volumeBar.MinSize().Height), pb.volumeBar)
	volRow := container.NewBorder(nil, nil, pb.volumeBtn, nil, volWrap)

	right := container.NewHBox(volRow, pb.closeBtn)

	row := container.NewBorder(nil, nil, left, right, container.NewCenter(controls))

	content := container.NewVBox(
		pb.topSeekRow(),
		container.NewHBox(pb.timeLabel),
		row,
	)

	pb.container.Objects = []fyne.CanvasObject{content}
	pb.container.Refresh()
}

func (pb *PlayerBar) setupCompactLayout() {
	coverSize := fyne.NewSize(48, 48)
	pb.coverImg.Resize(coverSize)

	info := container.NewVBox(pb.songLabel, pb.artistLabel)
	infoWrap := container.NewGridWrap(fyne.NewSize(220, coverSize.Height), info)

	left := container.NewHBox(pb.coverImg, infoWrap)

	controls := container.NewHBox(pb.prevBtn, pb.playBtn, pb.nextBtn)

	right := container.NewHBox(pb.volumeBtn, pb.closeBtn)

	row := container.NewBorder(nil, nil, left, right, container.NewCenter(controls))

	content := container.NewVBox(
		pb.topSeekRow(),
		container.NewHBox(pb.loadingLabel, pb.timeLabel),
		row,
	)

	pb.container.Objects = []fyne.CanvasObject{content}
	pb.container.Refresh()
}

func (pb *PlayerBar) setupEventHandlers() {
	pb.player.OnPositionChanged(func(pos time.Duration) {
		fyne.Do(func() {
			if pb.userSeeking {
				return
			}

			pb.lastPosition = pos
			dur := pb.player.GetDuration()
			pb.lastDuration = dur

			if dur > 0 {
				progress := float64(pos) / float64(dur) * 100
				if progress > 100 {
					progress = 100
				}
				if progress < 0 {
					progress = 0
				}

				pb.seekingProgrammatically = true
				pb.seekBar.SetValue(progress)
				pb.seekingProgrammatically = false

				pb.timeLabel.SetText(fmt.Sprintf("%s / %s", formatDuration(pos), formatDuration(dur)))

			} else {
				pb.timeLabel.SetText(fmt.Sprintf("%s / --:--", formatDuration(pos)))
			}

			// Update buffer progress
			dp := pb.player.GetDownloadProgress()
			if dp < 1.0 && dp > 0 {
				pb.bufferProgress.SetValue(dp)
				pb.bufferProgress.Show()
			} else {
				pb.bufferProgress.Hide()
			}
		})
	})

	pb.player.OnFinished(func() {
		pb.handleSongFinished()
	})
}

func (pb *PlayerBar) onSeekChanged(value float64) {
	if pb.seekingProgrammatically {
		return
	}

	if pb.debug {
		log.Printf("[PLAYER_BAR] User seeking to: %.1f%%", value)
	}

	pb.userSeeking = true

	if pb.lastDuration > 0 {
		// Check if player supports seeking
		if !pb.player.CanSeek() {
			// Reset to current position if seeking not supported
			pb.seekingProgrammatically = true
			currentProgress := float64(pb.lastPosition) / float64(pb.lastDuration) * 100
			pb.seekBar.SetValue(currentProgress)
			pb.seekingProgrammatically = false
			pb.userSeeking = false
			return
		}

		// Get seekable range for streaming content
		minSeek, maxSeek := pb.player.GetSeekableRange()

		// Convert percentage to time position
		pos := time.Duration(float64(pb.lastDuration) * value / 100.0)

		// Clamp to seekable range
		if pos < minSeek {
			pos = minSeek
			value = float64(minSeek) / float64(pb.lastDuration) * 100
		}
		if pos > maxSeek {
			pos = maxSeek
			value = float64(maxSeek) / float64(pb.lastDuration) * 100
		}

		// Update seek bar if we had to clamp the value
		if pb.seekBar.Value != value {
			pb.seekingProgrammatically = true
			pb.seekBar.SetValue(value)
			pb.seekingProgrammatically = false
		}

		// Update time display
		pb.timeLabel.SetText(fmt.Sprintf("%s / %s", formatDuration(pos), formatDuration(pb.lastDuration)))

		// Show buffer progress if streaming
		if !pb.player.HasSufficientBuffer(pos) {
			bufferProgress := pb.player.GetDownloadProgress() * 100
			if bufferProgress < 100 {
				pb.showBufferingIndicator(fmt.Sprintf("Buffering... %.0f%%", bufferProgress))
			}
		}
	}
}

func (pb *PlayerBar) onSeekEnded(value float64) {
	pb.userSeeking = false

	if pb.seekingProgrammatically || pb.lastDuration <= 0 {
		return
	}

	// Check if player supports seeking
	if !pb.player.CanSeek() {
		pb.showTemporaryMessage("Seeking not available for this track")
		return
	}

	// Get seekable range
	minSeek, maxSeek := pb.player.GetSeekableRange()
	pos := time.Duration(float64(pb.lastDuration) * value / 100.0)

	// Clamp to seekable range
	if pos < minSeek {
		pos = minSeek
	}
	if pos > maxSeek {
		pos = maxSeek
		pb.showTemporaryMessage(fmt.Sprintf("Can only seek to %.0f%% (buffered)",
			float64(maxSeek)/float64(pb.lastDuration)*100))
	}

	if pb.debug {
		log.Printf("[PLAYER_BAR] Seeking to position: %v (%.1f%%)", pos, value)
	}

	if !pb.player.HasSufficientBuffer(pos) {
		bufferProgress := pb.player.GetDownloadProgress() * 100
		pb.showTemporaryMessage(fmt.Sprintf("Buffering to %.0f%%...", bufferProgress))
	}

	if err := pb.player.Seek(pos); err != nil {
		log.Printf("[PLAYER_BAR] Seek failed: %v", err)
		pb.showTemporaryMessage("Seek failed")

		// Reset to current position
		currentProgress := float64(pb.lastPosition) / float64(pb.lastDuration) * 100
		pb.seekingProgrammatically = true
		pb.seekBar.SetValue(currentProgress)
		pb.seekingProgrammatically = false
		return
	}

	// Update the time label immediately after successful seek
	pb.timeLabel.SetText(fmt.Sprintf("%s / %s", formatDuration(pos), formatDuration(pb.lastDuration)))
}

func (pb *PlayerBar) showTemporaryMessage(message string) {
	if pb.statusLabel != nil {
		originalText := pb.statusLabel.Text
		pb.statusLabel.SetText(message)
		pb.statusLabel.Show()

		// Restore original text after 3 seconds
		time.AfterFunc(3*time.Second, func() {
			fyne.Do(func() {
				if pb.statusLabel != nil {
					pb.statusLabel.SetText(originalText)
					if originalText == "" {
						pb.statusLabel.Hide()
					}
				}
			})
		})
	}
}

// Helper method to show buffering indicator
func (pb *PlayerBar) showBufferingIndicator(message string) {
	if pb.loadingLabel != nil {
		pb.loadingLabel.SetText(message)
		pb.loadingLabel.Show()

		// Hide after 2 seconds if no new buffering updates
		time.AfterFunc(2*time.Second, func() {
			fyne.Do(func() {
				if pb.loadingLabel != nil && pb.loadingLabel.Text == message {
					pb.loadingLabel.Hide()
				}
			})
		})
	}
}

func (pb *PlayerBar) setupSeekBar() {
	pb.seekBar = widget.NewSlider(0, 100)
	pb.seekBar.OnChanged = pb.onSeekChanged
	pb.seekBar.OnChangeEnded = pb.onSeekEnded

	pb.bufferProgress = newBufferBar()
	pb.bufferProgress.Hide()

	pb.waveform = newWaveformBar()
	pb.waveform.Hide()

	// Order: waveform at bottom, then buffer, then slider on top
	pb.seekStack = container.NewStack(pb.waveform, pb.bufferProgress, pb.seekBar)
}

func (pb *PlayerBar) topSeekRow() fyne.CanvasObject {
	// Initialize seekStack if not already done
	if pb.seekStack == nil {
		pb.seekStack = container.NewStack(pb.bufferProgress, pb.seekBar)
	}
	return pb.seekStack
}

func (pb *PlayerBar) updateBufferProgress() {
	if pb.bufferProgress == nil || pb.player == nil {
		return
	}

	downloadProgress := pb.player.GetDownloadProgress()

	if downloadProgress < 1.0 && downloadProgress > 0 {
		pb.bufferProgress.SetValue(downloadProgress)
		pb.bufferProgress.Show()
	} else {
		pb.bufferProgress.Hide()
	}
}

func (pb *PlayerBar) setupStatusLabel() {
	pb.statusLabel = widget.NewLabel("")
	pb.statusLabel.Hide()
	pb.statusLabel.TextStyle = fyne.TextStyle{Italic: true}
}

func (pb *PlayerBar) playSong(song *types.Song) {
	if song == nil {
		if pb.debug {
			log.Printf("[PLAYER_BAR] Cannot play nil song")
		}
		return
	}

	if pb.debug {
		log.Printf("[PLAYER_BAR] Starting playback for: %s", song.Name)
	}

	// Reset UI state
	pb.seekBar.SetValue(0)
	pb.bufferProgress.SetValue(0)
	pb.timeLabel.SetText("0:00 / 0:00")
	pb.lastPosition = 0
	pb.lastDuration = 0

	pb.setLoading(true)

	go func() {
		defer pb.setLoading(false)

		ctx := context.Background()
		if err := pb.player.Play(ctx, song); err != nil {
			log.Printf("[PLAYER_BAR] Failed to play song: %v", err)

			// Try next song if this one fails
			fyne.Do(func() {
				time.Sleep(1 * time.Second)
				if len(pb.queue) > 1 { // Only try next if we have more songs
					pb.nextSong()
				}
			})
			return
		}

		fyne.Do(func() {
			pb.SetCurrentSong(song)
			pb.isPlaying = true
			pb.playStartTime = time.Now()
			pb.updatePlayButton()

			if pb.debug {
				log.Printf("[PLAYER_BAR] Playback started successfully for: %s", song.Name)
			}
		})
	}()
}

// Helper function to ensure proper duration formatting
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0:00"
	}

	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	if minutes >= 60 {
		hours := minutes / 60
		minutes = minutes % 60
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func volumeIconFor(v float64) fyne.Resource {
	switch {
	case v == 0:
		return theme.VolumeMuteIcon()
	case v < 50:
		return theme.VolumeDownIcon()
	default:
		return theme.VolumeUpIcon()
	}
}

func (pb *PlayerBar) togglePlay() {
	if pb.isPlaying {
		if err := pb.player.Pause(); err != nil {
			log.Printf("[PLAYER_BAR] Pause failed: %v", err)
			return
		}
		pb.isPlaying = false
		pb.updatePlayButton()
	} else {
		if pb.currentSong == nil && len(pb.queue) > 0 {
			pb.playSong(pb.queue[0])
			pb.queueIndex = 0
		} else {
			if err := pb.player.Resume(); err != nil {
				log.Printf("[PLAYER_BAR] Resume failed: %v", err)
				return
			}
			pb.isPlaying = true
			pb.updatePlayButton()
		}
	}
}

func (pb *PlayerBar) updatePlayButton() {
	fyne.Do(func() {
		if pb.isPlaying {
			pb.playBtn.SetIcon(theme.MediaPauseIcon())
		} else {
			pb.playBtn.SetIcon(theme.MediaPlayIcon())
		}
		pb.playBtn.Refresh()
	})
}

// Updated PlayerBar methods to prevent getting stuck

func (pb *PlayerBar) nextSong() {
	if len(pb.queue) == 0 {
		if pb.debug {
			log.Printf("[PLAYER_BAR] No queue for next song")
		}
		return
	}

	// Disable the button temporarily to prevent rapid clicks
	pb.nextBtn.Disable()
	go func() {
		time.Sleep(500 * time.Millisecond)
		fyne.Do(func() {
			if pb.nextBtn != nil {
				pb.nextBtn.Enable()
			}
		})
	}()

	switch pb.repeatMode {
	case RepeatOne:
		if pb.currentSong != nil {
			pb.playSong(pb.currentSong)
		}
		return
	}

	var nextIndex int
	if pb.isShuffled {
		nextIndex = (pb.queueIndex + 1) % len(pb.queue)
	} else {
		nextIndex = pb.queueIndex + 1
		if nextIndex >= len(pb.queue) {
			if pb.repeatMode == RepeatAll {
				nextIndex = 0
			} else {
				pb.stop()
				return
			}
		}
	}

	if nextIndex >= 0 && nextIndex < len(pb.queue) {
		pb.queueIndex = nextIndex
		pb.playSong(pb.queue[nextIndex])

		if pb.onNext != nil {
			pb.onNext()
		}
	}
}

func (pb *PlayerBar) previousSong() {
	if len(pb.queue) == 0 {
		if pb.debug {
			log.Printf("[PLAYER_BAR] No queue for previous song")
		}
		return
	}

	// Disable the button temporarily to prevent rapid clicks
	pb.prevBtn.Disable()
	go func() {
		time.Sleep(500 * time.Millisecond)
		fyne.Do(func() {
			if pb.prevBtn != nil {
				pb.prevBtn.Enable()
			}
		})
	}()

	switch pb.repeatMode {
	case RepeatOne:
		if pb.currentSong != nil {
			pb.playSong(pb.currentSong)
		}
		return
	}

	var nextIndex int
	if pb.isShuffled {
		nextIndex = (pb.queueIndex - 1 + len(pb.queue)) % len(pb.queue)
	} else {
		nextIndex = pb.queueIndex - 1
		if nextIndex < 0 {
			if pb.repeatMode == RepeatAll {
				nextIndex = len(pb.queue) - 1
			} else {
				return
			}
		}
	}

	if nextIndex >= 0 && nextIndex < len(pb.queue) {
		pb.queueIndex = nextIndex
		pb.playSong(pb.queue[nextIndex])

		if pb.onPrevious != nil {
			pb.onPrevious()
		}
	}
}

func (pb *PlayerBar) handleSongFinished() {
	if pb.currentSong != nil {
		playedDuration := time.Since(pb.playStartTime)
		if playedDuration >= pb.minPlayDuration {
			go pb.recordPlay(pb.currentSong)
		}
	}

	// Small delay to ensure clean transition
	time.Sleep(200 * time.Millisecond)

	// Prefetch next song
	if len(pb.queue) > 0 {
		next := (pb.queueIndex + 1) % len(pb.queue)
		if next >= 0 && next < len(pb.queue) && pb.onPrefetchNext != nil {
			pb.onPrefetchNext(pb.queue[next])
		}
	}

	// Move to next song
	fyne.Do(func() {
		pb.nextSong()
	})
}

// Improved setLoading to prevent UI issues
func (pb *PlayerBar) setLoading(loading bool) {
	fyne.Do(func() {
		pb.loading = loading

		if loading {
			if pb.loadingLabel != nil {
				pb.loadingLabel.Show()
			}
			pb.seekBar.Disable()
			// Don't disable nav buttons here - let individual methods handle it
		} else {
			if pb.loadingLabel != nil {
				pb.loadingLabel.Hide()
			}
			pb.seekBar.Enable()
			pb.prevBtn.Enable()
			pb.nextBtn.Enable()
		}
	})
}

func (pb *PlayerBar) recordPlay(song *types.Song) {
	ctx := context.Background()
	song.Played++

	if err := pb.storage.SaveSong(ctx, song); err != nil {
		log.Printf("[PLAYER_BAR] Failed to update play count for song %s: %v", song.Name, err)
	}

	if err := pb.storage.AddPlayHistory(ctx, song.Slug, nil); err != nil {
		log.Printf("[PLAYER_BAR] Failed to add play history for %s: %v", song.Slug, err)
	}

	if pb.onPlayed != nil {
		pb.onPlayed(song)
	}

	log.Printf("[PLAYER_BAR] Recorded play for song: %s (total plays: %d)", song.Name, song.Played)
}

func (pb *PlayerBar) toggleShuffle() {
	pb.isShuffled = !pb.isShuffled
	pb.updateShuffleButton()

	if pb.onShuffle != nil {
		pb.onShuffle(pb.isShuffled)
	}
}

func (pb *PlayerBar) toggleRepeat() {
	switch pb.repeatMode {
	case RepeatOff:
		pb.repeatMode = RepeatAll
	case RepeatAll:
		pb.repeatMode = RepeatOne
	case RepeatOne:
		pb.repeatMode = RepeatOff
	}

	pb.updateRepeatButton()

	if pb.onRepeat != nil {
		pb.onRepeat(pb.repeatMode)
	}
}

func (pb *PlayerBar) toggleLike() {
	if pb.currentSong == nil {
		return
	}

	liked := pb.currentSong.Liked == nil || !*pb.currentSong.Liked
	pb.currentSong.Liked = &liked

	go func() {
		ctx := context.Background()
		if err := pb.storage.SaveSong(ctx, pb.currentSong); err != nil {
			log.Printf("[PLAYER_BAR] Failed to update like status: %v", err)
		}
	}()

	pb.updateLikeButton()
}

func (pb *PlayerBar) updateShuffleButton() {
	fyne.Do(func() {
		pb.shuffleBtn.SetIcon(theme.ViewRefreshIcon())
		if pb.isShuffled {
			pb.shuffleBtn.Importance = widget.MediumImportance
		} else {
			pb.shuffleBtn.Importance = widget.LowImportance
		}
		pb.shuffleBtn.Refresh()
	})
}

func (pb *PlayerBar) updateRepeatButton() {
	fyne.Do(func() {
		pb.repeatBtn.SetIcon(theme.MediaReplayIcon())
		switch pb.repeatMode {
		case RepeatOff:
			pb.repeatBtn.Importance = widget.LowImportance
			pb.repeatBtn.SetText("")
		case RepeatAll:
			pb.repeatBtn.Importance = widget.MediumImportance
			pb.repeatBtn.SetText("")
		case RepeatOne:
			pb.repeatBtn.Importance = widget.HighImportance
			pb.repeatBtn.SetText("1")
		}
		pb.repeatBtn.Refresh()
	})
}

func (pb *PlayerBar) updateLikeButton() {
	fyne.Do(func() {
		if pb.currentSong != nil && pb.currentSong.Liked != nil && *pb.currentSong.Liked {
			pb.likeBtn.SetIcon(theme.ConfirmIcon())
			pb.likeBtn.Importance = widget.MediumImportance
		} else {
			pb.likeBtn.SetIcon(theme.VisibilityOffIcon())
			pb.likeBtn.Importance = widget.LowImportance
		}
		pb.likeBtn.Refresh()
	})
}

func (pb *PlayerBar) onVolumeChange(v float64) {
	if err := pb.player.SetVolume(v / 100); err != nil {
		log.Printf("[PLAYER_BAR] Failed to set volume: %v", err)
	}

	fyne.Do(func() {
		if pb.volumeBtn == nil {
			return
		}
		pb.volumeBtn.SetIcon(volumeIconFor(v))
	})
}

func (pb *PlayerBar) showVolumeDialog() {
	if !pb.compactMode || pb.parentWindow == nil {
		return
	}
	volumeSlider := widget.NewSlider(0, 100)
	volumeSlider.SetValue(pb.volumeBar.Value)
	volumeLabel := widget.NewLabel(fmt.Sprintf("Volume: %.0f%%", pb.volumeBar.Value))
	volumeSlider.OnChanged = func(value float64) {
		pb.volumeBar.SetValue(value)
		pb.onVolumeChange(value)
		volumeLabel.SetText(fmt.Sprintf("Volume: %.0f%%", value))
	}
	content := container.NewVBox(volumeLabel, volumeSlider)
	pb.volumeDialog = dialog.NewCustom("Volume", "Close", content, pb.parentWindow)
	pb.volumeDialog.Resize(fyne.NewSize(300, 150))
	pb.volumeDialog.Show()
}

func (pb *PlayerBar) watchBufferUntilReady() {
	pb.setLoading(true)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastShownPct := -1
	seenAnyProgress := false

	for {
		if pb.lastPosition > 0 {
			pb.setLoading(false)
			return
		}

		p := pb.player.GetDownloadProgress()
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}

		cur := int(p * 100)
		if cur != lastShownPct {
			lastShownPct = cur
			fyne.Do(func() {
				if pb.loadingLabel != nil {
					pct := cur
					if pct < 0 {
						pct = 0
					}
					if pct > 100 {
						pct = 100
					}
					pb.loadingLabel.SetText(fmt.Sprintf("Loading… (%d%%)", pct))
					pb.loadingLabel.Show()
				}
				if pb.bufferProgress != nil {
					pb.bufferProgress.SetValue(p)
					pb.bufferProgress.Show()
				}
			})
		}

		if p >= 0.02 && seenAnyProgress {
			pb.setLoading(false)
			return
		}
		if p > 0 {
			seenAnyProgress = true
		}

		<-ticker.C
	}
}

func (pb *PlayerBar) stop() {
	if err := pb.player.Stop(); err != nil {
		log.Printf("[PLAYER_BAR] Failed to stop: %v", err)
	}
	fyne.Do(func() {
		pb.playBtn.SetIcon(theme.MediaPlayIcon())
		pb.seekBar.SetValue(0)
		pb.bufferProgress.Hide()
	})
	pb.isPlaying = false
	pb.stopLoadingTicker()
	pb.loading = false
}

func (pb *PlayerBar) SetCurrentSong(song *types.Song) {
	pb.currentSong = song
	fyne.Do(func() {
		if song != nil {
			pb.songLabel.SetText(song.Name)
			pb.artistLabel.SetText(getArtistNames(song.Authors))
			pb.updateLikeButton()
		} else {
			pb.songLabel.SetText("No song playing")
			pb.artistLabel.SetText("")
		}

		var target fyne.Size
		if pb.compactMode {
			target = fyne.NewSize(40, 40)
		} else {
			h := pb.desiredHeight - 10
			if h < 40 {
				h = 40
			}
			target = fyne.NewSize(h, h)
		}
		pb.coverImg.SetMinSize(target)
		pb.coverImg.Resize(target)

		url := pb.imageService.PreferredCoverURL(song)
		if pb.imageService == nil || url == "" {
			pb.coverImg.Resource = theme.MediaMusicIcon()
			pb.coverImg.Refresh()
			return
		}
		pb.imageService.GetImageWithSize(url, target, func(res fyne.Resource, err error) {
			if err != nil || res == nil {
				res = theme.MediaMusicIcon()
			}
			pb.coverImg.Resource = res
			pb.coverImg.Refresh()
		})
		// Waveform handling
		pb.setWaveformFromSong(song)
	})
}

// Set or clear the waveform from a song struct.
func (pb *PlayerBar) setWaveformFromSong(song *types.Song) {
	if pb.waveform == nil {
		return
	}
	if song == nil || len(song.Volume) == 0 {
		pb.waveform.Clear()
		pb.waveform.Hide()
		return
	}
	pb.waveform.SetDataInt(song.Volume)
	pb.waveform.Show()
}

func (pb *PlayerBar) SetWaveform(vol []int) {
	if pb.waveform == nil {
		return
	}
	if len(vol) == 0 {
		pb.waveform.Clear()
		pb.waveform.Hide()
		return
	}
	pb.waveform.SetDataInt(vol)
	pb.waveform.Show()
}

func (pb *PlayerBar) SetQueue(songs []*types.Song, startIndex int) {
	pb.queue = songs
	pb.queueIndex = startIndex

	if startIndex >= 0 && startIndex < len(songs) {
		pb.playSong(songs[startIndex])
	}
}

func (pb *PlayerBar) AddToQueue(song *types.Song) {
	pb.queue = append(pb.queue, song)
}

func (pb *PlayerBar) GetQueue() []*types.Song {
	return pb.queue
}

func (pb *PlayerBar) GetCurrentIndex() int {
	return pb.queueIndex
}

func (pb *PlayerBar) SetCompactMode(compact bool) {
	if pb.compactMode != compact {
		pb.compactMode = compact
		pb.calculateDesiredHeight()
		pb.setupLayout()
	}
}

func (pb *PlayerBar) GetDesiredHeight() float32 {
	return pb.desiredHeight
}

func (pb *PlayerBar) OnNext(callback func()) {
	pb.onNext = callback
}

func (pb *PlayerBar) OnPrevious(callback func()) {
	pb.onPrevious = callback
}

func (pb *PlayerBar) OnShuffle(callback func(bool)) {
	pb.onShuffle = callback
}

func (pb *PlayerBar) OnRepeat(callback func(RepeatMode)) {
	pb.onRepeat = callback
}

func (pb *PlayerBar) Container() *fyne.Container {
	return pb.container
}

func getArtistNames(authors []*types.Author) string {
	if len(authors) == 0 {
		return "Unknown Artist"
	}

	names := make([]string, 0, len(authors))
	for _, author := range authors {
		if author != nil && author.Name != "" {
			names = append(names, author.Name)
		}
	}

	if len(names) == 0 {
		return "Unknown Artist"
	}

	if len(names) == 1 {
		return names[0]
	}

	if len(names) == 2 {
		return names[0] + " & " + names[1]
	}

	return strings.Join(names, ", ")
}

func (pb *PlayerBar) startLoadingTicker() {
	if pb.loadingStopCh != nil {
		return
	}
	pb.loadingStopCh = make(chan struct{})
	go func(stop <-chan struct{}) {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				perc := int(pb.player.GetDownloadProgress() * 100)
				fyne.Do(func() { pb.timeLabel.SetText(fmt.Sprintf("Loading… (%d%%)", perc)) })
			case <-stop:
				return
			}
		}
	}(pb.loadingStopCh)
}

func (pb *PlayerBar) stopLoadingTicker() {
	if pb.loadingStopCh != nil {
		close(pb.loadingStopCh)
		pb.loadingStopCh = nil
	}
}

func (pb *PlayerBar) closeAndHide() {
	pb.stop()
	pb.SetCurrentSong(nil)
	pb.container.Hide()
}

func (pb *PlayerBar) OnPlayed(cb func(*types.Song))       { pb.onPlayed = cb }
func (pb *PlayerBar) OnPrefetchNext(cb func(*types.Song)) { pb.onPrefetchNext = cb }
