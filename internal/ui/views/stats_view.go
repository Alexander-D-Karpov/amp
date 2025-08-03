package views

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/services"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type StatsView struct {
	musicService *services.MusicService
	container    *fyne.Container

	totalSongsCard   *widget.Card
	totalAlbumsCard  *widget.Card
	totalArtistsCard *widget.Card
	timeListenedCard *widget.Card

	refreshBtn  *widget.Button
	compactMode bool
}

func NewStatsView(musicService *services.MusicService) *StatsView {
	sv := &StatsView{
		musicService: musicService,
	}

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadStats()
	return sv
}

func (sv *StatsView) setupWidgets() {
	sv.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), sv.loadStats)

	sv.totalSongsCard = widget.NewCard("Total Songs", "", widget.NewLabel("Loading..."))
	sv.totalAlbumsCard = widget.NewCard("Total Albums", "", widget.NewLabel("Loading..."))
	sv.totalArtistsCard = widget.NewCard("Total Artists", "", widget.NewLabel("Loading..."))
	sv.timeListenedCard = widget.NewCard("Time Listened", "", widget.NewLabel("Loading..."))
}

func (sv *StatsView) setupLayout() {
	header := container.NewBorder(
		nil, nil,
		widget.NewLabel("Music Library Statistics"),
		sv.refreshBtn,
		nil,
	)

	overviewGrid := container.NewGridWithColumns(4,
		sv.totalSongsCard,
		sv.totalAlbumsCard,
		sv.totalArtistsCard,
		sv.timeListenedCard,
	)

	content := container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabel("Overview"),
		overviewGrid,
	)

	scroll := container.NewScroll(content)
	sv.container = container.NewVBox(scroll)
}

func (sv *StatsView) loadStats() {
	go func() {
		ctx := context.Background()

		songs, _, err := sv.musicService.GetSongs(ctx, 1, "")
		if err != nil {
			return
		}

		albums, _, err := sv.musicService.GetAlbums(ctx, 1, "")
		if err != nil {
			return
		}

		artists, _, err := sv.musicService.GetAuthors(ctx, 1, "")
		if err != nil {
			return
		}

		fyne.Do(func() {
			sv.updateStats(len(songs), len(albums), len(artists), songs)
		})
	}()
}

func (sv *StatsView) updateStats(songCount, albumCount, artistCount int, songs []*types.Song) {
	sv.totalSongsCard.SetContent(container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%d", songCount), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("songs in library", fyne.TextAlignCenter, fyne.TextStyle{}),
	))

	sv.totalAlbumsCard.SetContent(container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%d", albumCount), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("albums collected", fyne.TextAlignCenter, fyne.TextStyle{}),
	))

	sv.totalArtistsCard.SetContent(container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%d", artistCount), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("artists discovered", fyne.TextAlignCenter, fyne.TextStyle{}),
	))

	totalSeconds := 0
	totalPlays := 0
	for _, song := range songs {
		totalSeconds += song.Played * song.Length
		totalPlays += song.Played
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60

	sv.timeListenedCard.SetContent(container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%dh %dm", hours, minutes), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("total listening time", fyne.TextAlignCenter, fyne.TextStyle{}),
	))
}

func (sv *StatsView) SetCompactMode(compact bool) {
	sv.compactMode = compact
	sv.setupLayout()
}

func (sv *StatsView) Refresh() {
	sv.loadStats()
}

func (sv *StatsView) Container() *fyne.Container {
	return sv.container
}
