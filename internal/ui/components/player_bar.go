package components

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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
	bufferProgress *widget.ProgressBar
	waveformCanvas *canvas.Image
	volumeBar      *widget.Slider
	volumeBtn      *widget.Button
	timeLabel      *widget.Label
	songLabel      *widget.Label
	artistLabel    *widget.Label
	coverImg       *widget.Icon
	volumeDialog   dialog.Dialog

	currentSong *types.Song
	isPlaying   bool
	isShuffled  bool
	repeatMode  RepeatMode
	queue       []*types.Song
	queueIndex  int
	compactMode bool
	breakpoint  float32

	onNext                  func()
	onPrevious              func()
	onShuffle               func(bool)
	onRepeat                func(RepeatMode)
	seekingProgrammatically bool
	userSeeking             bool
	parentWindow            fyne.Window
	lastPosition            time.Duration
	lastDuration            time.Duration
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

func NewPlayerBar(player *audio.Player, storage *storage.Database) *PlayerBar {
	pb := &PlayerBar{
		player:     player,
		storage:    storage,
		queue:      make([]*types.Song, 0),
		queueIndex: -1,
		breakpoint: 800.0,
	}

	pb.setupWidgets()
	pb.setupLayout()
	pb.setupEventHandlers()

	return pb
}

func (pb *PlayerBar) SetConfig(cfg *config.Config) {
	pb.cfg = cfg
}

func (pb *PlayerBar) SetParentWindow(window fyne.Window) {
	pb.parentWindow = window
}

func (pb *PlayerBar) setupWidgets() {
	pb.playBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), pb.togglePlay)
	pb.playBtn.Importance = widget.HighImportance

	pb.prevBtn = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), pb.previousSong)
	pb.nextBtn = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), pb.nextSong)
	pb.shuffleBtn = widget.NewButtonWithIcon("", theme.MediaReplayIcon(), pb.toggleShuffle)
	pb.shuffleBtn.Importance = widget.LowImportance
	pb.repeatBtn = widget.NewButtonWithIcon("", theme.MediaReplayIcon(), pb.toggleRepeat)
	pb.repeatBtn.Importance = widget.LowImportance
	pb.likeBtn = widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), pb.toggleLike)
	pb.likeBtn.Importance = widget.LowImportance

	pb.seekBar = widget.NewSlider(0, 100)
	pb.seekBar.OnChanged = pb.onSeekChanged
	pb.seekBar.OnChangeEnded = pb.onSeekEnded

	pb.bufferProgress = widget.NewProgressBar()
	pb.bufferProgress.Hide()

	pb.volumeBar = widget.NewSlider(0, 100)
	pb.volumeBar.SetValue(70)
	pb.volumeBar.OnChanged = pb.onVolumeChange

	pb.volumeBtn = widget.NewButtonWithIcon("", theme.VolumeUpIcon(), pb.showVolumeDialog)
	pb.volumeBtn.Importance = widget.LowImportance

	pb.timeLabel = widget.NewLabel("0:00 / 0:00")
	pb.timeLabel.TextStyle = fyne.TextStyle{Monospace: true}

	pb.songLabel = widget.NewLabel("No song playing")
	pb.songLabel.TextStyle = fyne.TextStyle{Bold: true}
	pb.songLabel.Truncation = fyne.TextTruncateEllipsis

	pb.artistLabel = widget.NewLabel("")
	pb.artistLabel.Truncation = fyne.TextTruncateEllipsis

	pb.coverImg = widget.NewIcon(theme.MediaMusicIcon())

	pb.updateRepeatButton()
	pb.updateShuffleButton()
}

func (pb *PlayerBar) setupLayout() {
	pb.container = container.NewStack()
	pb.updateLayoutForSize(fyne.NewSize(1200, 100))
}

func (pb *PlayerBar) updateLayoutForSize(size fyne.Size) {
	pb.compactMode = size.Width < pb.breakpoint

	if pb.compactMode {
		pb.setupCompactLayout()
	} else {
		pb.setupDesktopLayout()
	}
}

func (pb *PlayerBar) setupDesktopLayout() {
	songInfo := container.NewVBox(
		pb.songLabel,
		pb.artistLabel,
	)

	leftSection := container.NewHBox(
		container.NewPadded(pb.coverImg),
		songInfo,
	)
	leftSection = container.NewWithoutLayout(leftSection)
	leftSection.Resize(fyne.NewSize(300, 80))

	playerControls := container.NewHBox(
		pb.shuffleBtn,
		pb.prevBtn,
		pb.playBtn,
		pb.nextBtn,
		pb.repeatBtn,
	)

	seekContainer := container.NewStack(
		pb.bufferProgress,
		pb.seekBar,
	)

	timeSeekContainer := container.NewBorder(
		nil, nil,
		pb.timeLabel, nil,
		seekContainer,
	)

	centerSection := container.NewVBox(
		container.NewCenter(playerControls),
		timeSeekContainer,
	)

	volumeControls := container.NewBorder(
		nil, nil,
		pb.volumeBtn, nil,
		pb.volumeBar,
	)
	volumeControls = container.NewWithoutLayout(volumeControls)
	volumeControls.Resize(fyne.NewSize(150, 40))

	rightSection := container.NewHBox(
		pb.likeBtn,
		volumeControls,
	)

	content := container.NewBorder(
		nil, nil,
		leftSection,
		rightSection,
		centerSection,
	)

	pb.container.Objects = []fyne.CanvasObject{content}
	pb.container.Refresh()
}

func (pb *PlayerBar) setupCompactLayout() {
	topControls := container.NewHBox(
		pb.prevBtn,
		pb.playBtn,
		pb.nextBtn,
		layout.NewSpacer(),
		pb.likeBtn,
		pb.volumeBtn,
	)

	songInfo := container.NewVBox(
		pb.songLabel,
		pb.artistLabel,
	)

	seekContainer := container.NewStack(
		pb.bufferProgress,
		pb.seekBar,
	)

	bottomRow := container.NewBorder(
		nil, nil,
		pb.timeLabel, nil,
		seekContainer,
	)

	content := container.NewVBox(
		topControls,
		songInfo,
		bottomRow,
	)

	pb.container.Objects = []fyne.CanvasObject{content}
	pb.container.Refresh()
}

func (pb *PlayerBar) showVolumeDialog() {
	if pb.parentWindow == nil {
		return
	}

	volumeSlider := widget.NewSlider(0, 100)
	volumeSlider.SetValue(pb.volumeBar.Value)
	volumeSlider.OnChanged = func(value float64) {
		pb.volumeBar.SetValue(value)
		pb.onVolumeChange(value)
	}

	volumeLabel := widget.NewLabel(fmt.Sprintf("Volume: %.0f%%", pb.volumeBar.Value))
	volumeSlider.OnChanged = func(value float64) {
		pb.volumeBar.SetValue(value)
		pb.onVolumeChange(value)
		volumeLabel.SetText(fmt.Sprintf("Volume: %.0f%%", value))
	}

	content := container.NewVBox(
		volumeLabel,
		volumeSlider,
	)

	pb.volumeDialog = dialog.NewCustom("Volume Control", "Close", content, pb.parentWindow)
	pb.volumeDialog.Resize(fyne.NewSize(300, 150))
	pb.volumeDialog.Show()
}

func (pb *PlayerBar) SetCompactMode(compact bool) {
	pb.compactMode = compact
	if compact {
		pb.setupCompactLayout()
	} else {
		pb.setupDesktopLayout()
	}
}

func (pb *PlayerBar) Resize(size fyne.Size) {
	pb.updateLayoutForSize(size)
	pb.container.Resize(size)
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
				pb.seekingProgrammatically = true
				pb.seekBar.SetValue(progress)
				pb.seekingProgrammatically = false
				pb.timeLabel.SetText(fmt.Sprintf("%s / %s", formatDuration(pos), formatDuration(dur)))
			} else {
				pb.timeLabel.SetText(fmt.Sprintf("%s / --:--", formatDuration(pos)))
			}

			downloadProgress := pb.player.GetDownloadProgress()
			if downloadProgress < 1.0 {
				pb.bufferProgress.SetValue(downloadProgress)
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

	pb.userSeeking = true

	if pb.lastDuration > 0 {
		position := time.Duration(float64(pb.lastDuration) * value / 100)
		pb.timeLabel.SetText(fmt.Sprintf("%s / %s", formatDuration(position), formatDuration(pb.lastDuration)))
	}
}

func (pb *PlayerBar) onSeekEnded(value float64) {
	pb.userSeeking = false

	if pb.seekingProgrammatically {
		return
	}

	if pb.lastDuration > 0 {
		position := time.Duration(float64(pb.lastDuration) * value / 100)
		if err := pb.player.Seek(position); err != nil {
			log.Printf("[PLAYER_BAR] Seek failed: %v", err)
		}
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

func (pb *PlayerBar) previousSong() {
	if len(pb.queue) == 0 {
		return
	}

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

	pb.queueIndex = nextIndex
	pb.playSong(pb.queue[nextIndex])

	if pb.onPrevious != nil {
		pb.onPrevious()
	}
}

func (pb *PlayerBar) nextSong() {
	if len(pb.queue) == 0 {
		return
	}

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

	pb.queueIndex = nextIndex
	pb.playSong(pb.queue[nextIndex])

	if pb.onNext != nil {
		pb.onNext()
	}
}

func (pb *PlayerBar) handleSongFinished() {
	if pb.currentSong != nil {
		go pb.recordPlay(pb.currentSong)
	}

	time.Sleep(100 * time.Millisecond)
	pb.nextSong()
}

func (pb *PlayerBar) recordPlay(song *types.Song) {
	ctx := context.Background()
	song.Played++

	if err := pb.storage.SaveSong(ctx, song); err != nil {
		log.Printf("[PLAYER_BAR] Failed to update play count for song %s: %v", song.Name, err)
	}
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
		switch pb.repeatMode {
		case RepeatOff:
			pb.repeatBtn.SetIcon(theme.MediaReplayIcon())
			pb.repeatBtn.Importance = widget.LowImportance
		case RepeatAll:
			pb.repeatBtn.SetIcon(theme.MediaReplayIcon())
			pb.repeatBtn.Importance = widget.MediumImportance
		case RepeatOne:
			pb.repeatBtn.SetIcon(theme.MediaReplayIcon())
			pb.repeatBtn.Importance = widget.HighImportance
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
		if v == 0 {
			pb.volumeBtn.SetIcon(theme.VolumeMuteIcon())
		} else if v < 50 {
			pb.volumeBtn.SetIcon(theme.VolumeDownIcon())
		} else {
			pb.volumeBtn.SetIcon(theme.VolumeUpIcon())
		}
	})
}

func (pb *PlayerBar) playSong(song *types.Song) {
	ctx := context.Background()
	if err := pb.player.Play(ctx, song); err != nil {
		log.Printf("[PLAYER_BAR] Failed to play song: %v", err)

		go func() {
			time.Sleep(500 * time.Millisecond)
			pb.nextSong()
		}()
		return
	}

	pb.SetCurrentSong(song)
	pb.isPlaying = true
	pb.updatePlayButton()
}

func (pb *PlayerBar) stop() {
	if err := pb.player.Stop(); err != nil {
		log.Printf("[PLAYER_BAR] Failed to stop: %v", err)
	}
	fyne.Do(func() {
		pb.playBtn.SetIcon(theme.MediaPlayIcon())
		pb.timeLabel.SetText("0:00 / 0:00")
		pb.seekBar.SetValue(0)
		pb.bufferProgress.Hide()
	})
	pb.isPlaying = false
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
	})
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

func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
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
