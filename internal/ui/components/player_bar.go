package components

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

	container   *fyne.Container
	playBtn     *widget.Button
	prevBtn     *widget.Button
	nextBtn     *widget.Button
	shuffleBtn  *widget.Button
	repeatBtn   *widget.Button
	likeBtn     *widget.Button
	seekBar     *widget.Slider
	volumeBar   *widget.Slider
	timeLabel   *widget.Label
	songLabel   *widget.Label
	artistLabel *widget.Label
	coverImg    *widget.Icon
	volumeIcon  *widget.Icon

	currentSong *types.Song
	isPlaying   bool
	isShuffled  bool
	repeatMode  RepeatMode
	queue       []*types.Song
	queueIndex  int
	compactMode bool

	onNext     func()
	onPrevious func()
	onShuffle  func(bool)
	onRepeat   func(RepeatMode)
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
	}

	pb.setupWidgets()
	pb.setupLayout()
	pb.setupEventHandlers()

	return pb
}

func (pb *PlayerBar) SetConfig(cfg *config.Config) {
	pb.cfg = cfg
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
	pb.seekBar.OnChanged = pb.onSeek
	pb.seekBar.OnChangeEnded = pb.onSeekEnded

	pb.volumeBar = widget.NewSlider(0, 100)
	pb.volumeBar.SetValue(70)
	pb.volumeBar.OnChanged = pb.onVolumeChange

	pb.timeLabel = widget.NewLabel("0:00 / 0:00")
	pb.songLabel = widget.NewLabel("No song playing")
	pb.songLabel.TextStyle = fyne.TextStyle{Bold: true}
	pb.artistLabel = widget.NewLabel("")

	pb.coverImg = widget.NewIcon(theme.MediaMusicIcon())
	pb.volumeIcon = widget.NewIcon(theme.VolumeUpIcon())

	pb.updateRepeatButton()
	pb.updateShuffleButton()
}

func (pb *PlayerBar) setupLayout() {
	songInfo := container.NewVBox(
		pb.songLabel,
		pb.artistLabel,
	)

	leftSection := container.NewHBox(
		pb.coverImg,
		songInfo,
	)

	playerControls := container.NewHBox(
		pb.shuffleBtn,
		pb.prevBtn,
		pb.playBtn,
		pb.nextBtn,
		pb.repeatBtn,
	)

	timeControls := container.NewBorder(
		nil, nil,
		pb.timeLabel, nil,
		pb.seekBar,
	)

	centerSection := container.NewVBox(
		playerControls,
		timeControls,
	)

	volumeControls := container.NewBorder(
		nil, nil,
		pb.volumeIcon, nil,
		pb.volumeBar,
	)

	rightSection := container.NewVBox(
		container.NewHBox(pb.likeBtn),
		volumeControls,
	)

	pb.container = container.NewBorder(
		nil, nil,
		leftSection,
		rightSection,
		centerSection,
	)
}

func (pb *PlayerBar) SetCompactMode(compact bool) {
	pb.compactMode = compact

	if compact {
		songInfo := container.NewHBox(pb.songLabel, widget.NewLabel("-"), pb.artistLabel)

		playerControls := container.NewHBox(
			pb.prevBtn,
			pb.playBtn,
			pb.nextBtn,
		)

		compactLeft := container.NewVBox(songInfo, pb.seekBar)
		compactRight := container.NewHBox(pb.likeBtn, pb.volumeIcon)

		pb.container = container.NewBorder(
			nil, nil,
			compactLeft,
			compactRight,
			playerControls,
		)
	} else {
		pb.setupLayout()
	}

	pb.container.Refresh()
}

func (pb *PlayerBar) setupEventHandlers() {
	pb.player.OnPositionChanged(func(position time.Duration) {
		duration := pb.player.GetDuration()
		if duration > 0 {
			progress := float64(position) / float64(duration) * 100
			pb.seekBar.SetValue(progress)
			pb.timeLabel.SetText(fmt.Sprintf("%s / %s",
				formatDuration(position),
				formatDuration(duration)))
		}
	})

	pb.player.OnFinished(func() {
		pb.handleSongFinished()
	})
}

func (pb *PlayerBar) togglePlay() {
	if pb.isPlaying {
		_ = pb.player.Pause()
		pb.playBtn.SetIcon(theme.MediaPlayIcon())
		pb.isPlaying = false
	} else {
		if pb.currentSong == nil && len(pb.queue) > 0 {
			pb.playSong(pb.queue[0])
			pb.queueIndex = 0
		} else {
			_ = pb.player.Resume()
			pb.playBtn.SetIcon(theme.MediaPauseIcon())
			pb.isPlaying = true
		}
	}
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

	pb.nextSong()
}

func (pb *PlayerBar) recordPlay(song *types.Song) {
	ctx := context.Background()
	song.Played++

	if err := pb.storage.SaveSong(ctx, song); err != nil {
		log.Printf("Failed to update play count for song %s: %v", song.Name, err)
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
			log.Printf("Failed to update like status: %v", err)
		}
	}()

	pb.updateLikeButton()
}

func (pb *PlayerBar) updateShuffleButton() {
	if pb.isShuffled {
		pb.shuffleBtn.Importance = widget.MediumImportance
	} else {
		pb.shuffleBtn.Importance = widget.LowImportance
	}
	pb.shuffleBtn.Refresh()
}

func (pb *PlayerBar) updateRepeatButton() {
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
}

func (pb *PlayerBar) updateLikeButton() {
	if pb.currentSong != nil && pb.currentSong.Liked != nil && *pb.currentSong.Liked {
		pb.likeBtn.SetIcon(theme.ConfirmIcon())
		pb.likeBtn.Importance = widget.MediumImportance
	} else {
		pb.likeBtn.SetIcon(theme.VisibilityOffIcon())
		pb.likeBtn.Importance = widget.LowImportance
	}
	pb.likeBtn.Refresh()
}

func (pb *PlayerBar) onSeek(value float64) {
	duration := pb.player.GetDuration()
	position := time.Duration(float64(duration) * value / 100)
	pb.timeLabel.SetText(fmt.Sprintf("%s / %s",
		formatDuration(position),
		formatDuration(duration)))
}

func (pb *PlayerBar) onSeekEnded(value float64) {
	duration := pb.player.GetDuration()
	position := time.Duration(float64(duration) * value / 100)
	_ = pb.player.Seek(position)
}

func (pb *PlayerBar) onVolumeChange(value float64) {
	_ = pb.player.SetVolume(value / 100)

	if value == 0 {
		pb.volumeIcon.SetResource(theme.VolumeDownIcon())
	} else if value < 50 {
		pb.volumeIcon.SetResource(theme.VolumeDownIcon())
	} else {
		pb.volumeIcon.SetResource(theme.VolumeUpIcon())
	}
}

func (pb *PlayerBar) playSong(song *types.Song) {
	ctx := context.Background()
	if err := pb.player.Play(ctx, song); err != nil {
		log.Printf("Failed to play song: %v", err)
		return
	}

	pb.SetCurrentSong(song)
	pb.playBtn.SetIcon(theme.MediaPauseIcon())
	pb.isPlaying = true
}

func (pb *PlayerBar) stop() {
	_ = pb.player.Stop()
	pb.playBtn.SetIcon(theme.MediaPlayIcon())
	pb.isPlaying = false
	pb.timeLabel.SetText("0:00 / 0:00")
	pb.seekBar.SetValue(0)
}

func (pb *PlayerBar) SetCurrentSong(song *types.Song) {
	pb.currentSong = song
	if song != nil {
		pb.songLabel.SetText(song.Name)
		pb.artistLabel.SetText(getArtistNames(song.Authors))
		pb.updateLikeButton()
	} else {
		pb.songLabel.SetText("No song playing")
		pb.artistLabel.SetText("")
	}
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
	if len(authors) == 1 {
		return authors[0].Name
	}

	if len(authors) == 2 {
		return fmt.Sprintf("%s & %s", authors[0].Name, authors[1].Name)
	}
	return fmt.Sprintf("%s & %d others", authors[0].Name, len(authors)-1)
}
