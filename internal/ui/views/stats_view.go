package views

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type StatsView struct {
	storage   *storage.Database
	container *fyne.Container

	totalSongsCard     *widget.Card
	totalAlbumsCard    *widget.Card
	totalArtistsCard   *widget.Card
	totalPlaylistsCard *widget.Card

	timeListenedCard   *widget.Card
	topSongsCard       *widget.Card
	topArtistsCard     *widget.Card
	recentActivityCard *widget.Card

	refreshBtn  *widget.Button
	compactMode bool
}

func NewStatsView(storage *storage.Database) *StatsView {
	sv := &StatsView{
		storage: storage,
	}

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadStats()

	return sv
}

func (sv *StatsView) SetCompactMode(compact bool) {
	sv.compactMode = compact
	sv.setupLayout()
}

func (sv *StatsView) setupWidgets() {
	sv.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), sv.loadStats)

	sv.totalSongsCard = widget.NewCard("Total Songs", "", widget.NewLabel("Loading..."))
	sv.totalAlbumsCard = widget.NewCard("Total Albums", "", widget.NewLabel("Loading..."))
	sv.totalArtistsCard = widget.NewCard("Total Artists", "", widget.NewLabel("Loading..."))
	sv.totalPlaylistsCard = widget.NewCard("Total Playlists", "", widget.NewLabel("Loading..."))

	sv.timeListenedCard = widget.NewCard("Time Listened", "", widget.NewLabel("Loading..."))

	sv.topSongsCard = widget.NewCard("Most Played Songs", "", widget.NewLabel("Loading..."))
	sv.topArtistsCard = widget.NewCard("Top Artists", "", widget.NewLabel("Loading..."))
	sv.recentActivityCard = widget.NewCard("Recent Activity", "", widget.NewLabel("Loading..."))
}

func (sv *StatsView) setupLayout() {
	header := container.NewBorder(
		nil, nil,
		widget.NewLabel("Music Library Statistics"),
		sv.refreshBtn,
		nil,
	)

	var overviewGrid *fyne.Container
	var topGrid *fyne.Container

	if sv.compactMode {
		overviewGrid = container.NewGridWithColumns(2,
			sv.totalSongsCard,
			sv.totalAlbumsCard,
			sv.totalArtistsCard,
			sv.totalPlaylistsCard,
		)
		topGrid = container.NewGridWithColumns(1,
			sv.topSongsCard,
			sv.topArtistsCard,
		)
	} else {
		overviewGrid = container.NewGridWithColumns(4,
			sv.totalSongsCard,
			sv.totalAlbumsCard,
			sv.totalArtistsCard,
			sv.totalPlaylistsCard,
		)
		topGrid = container.NewGridWithColumns(2,
			sv.topSongsCard,
			sv.topArtistsCard,
		)
	}

	listenGrid := container.NewGridWithColumns(1,
		sv.timeListenedCard,
	)

	activityGrid := container.NewGridWithColumns(1,
		sv.recentActivityCard,
	)

	content := container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabel("Overview"),
		overviewGrid,
		widget.NewSeparator(),
		widget.NewLabel("Listening Stats"),
		listenGrid,
		widget.NewSeparator(),
		widget.NewLabel("Top Content"),
		topGrid,
		widget.NewSeparator(),
		widget.NewLabel("Activity"),
		activityGrid,
	)

	scrollContainer := container.NewScroll(content)
	sv.container = container.NewStack(scrollContainer)
}

func (sv *StatsView) loadStats() {
	go func() {
		ctx := context.Background()

		songs, err := sv.storage.GetSongs(ctx, 10000, 0)
		if err != nil {
			log.Printf("Failed to get songs for stats: %v", err)
			return
		}

		albums, err := sv.storage.GetAlbums(ctx, 10000, 0)
		if err != nil {
			log.Printf("Failed to get albums for stats: %v", err)
			return
		}

		artists, err := sv.storage.GetAuthors(ctx, 10000, 0)
		if err != nil {
			log.Printf("Failed to get artists for stats: %v", err)
			return
		}

		playlists, err := sv.storage.GetPlaylists(ctx)
		if err != nil {
			log.Printf("Failed to get playlists for stats: %v", err)
			return
		}

		sv.updateOverviewStats(len(songs), len(albums), len(artists), len(playlists))
		sv.updateListeningStats(songs)
		sv.updateTopContent(songs, artists)
		sv.updateRecentActivity(songs)
	}()
}

func (sv *StatsView) updateOverviewStats(songCount, albumCount, artistCount, playlistCount int) {
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

	sv.totalPlaylistsCard.SetContent(container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%d", playlistCount), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("playlists created", fyne.TextAlignCenter, fyne.TextStyle{}),
	))
}

func (sv *StatsView) updateListeningStats(songs []*types.Song) {
	totalSeconds := 0
	totalPlays := 0

	for _, song := range songs {
		totalSeconds += song.Played * song.Length
		totalPlays += song.Played
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60

	content := container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%dh %dm", hours, minutes), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(fmt.Sprintf("total listening time"), fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(fmt.Sprintf("%d plays", totalPlays), fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewLabelWithStyle("songs played", fyne.TextAlignCenter, fyne.TextStyle{}),
	)

	if len(songs) > 0 {
		avgPlaysPerSong := float64(totalPlays) / float64(len(songs))
		content.Add(widget.NewSeparator())
		content.Add(widget.NewLabelWithStyle(fmt.Sprintf("%.1f avg plays", avgPlaysPerSong), fyne.TextAlignCenter, fyne.TextStyle{}))
		content.Add(widget.NewLabelWithStyle("per song", fyne.TextAlignCenter, fyne.TextStyle{}))
	}

	sv.timeListenedCard.SetContent(content)
}

func (sv *StatsView) updateTopContent(songs []*types.Song, artists []*types.Author) {
	topSongs := getTopSongs(songs, 5)
	songsList := container.NewVBox()

	for i, song := range topSongs {
		artistName := "Unknown Artist"
		if len(song.Authors) > 0 {
			artistName = song.Authors[0].Name
		}

		songItem := container.NewHBox(
			widget.NewLabel(fmt.Sprintf("%d.", i+1)),
			widget.NewLabel(song.Name),
			widget.NewLabel("-"),
			widget.NewLabel(artistName),
			widget.NewLabel(fmt.Sprintf("(%d plays)", song.Played)),
		)
		songsList.Add(songItem)
	}

	if len(topSongs) == 0 {
		songsList.Add(widget.NewLabel("No played songs yet"))
	}

	sv.topSongsCard.SetContent(songsList)

	artistPlayCounts := make(map[string]int)
	for _, song := range songs {
		for _, author := range song.Authors {
			artistPlayCounts[author.Name] += song.Played
		}
	}

	type artistStat struct {
		name  string
		plays int
	}

	var topArtistStats []artistStat
	for name, plays := range artistPlayCounts {
		topArtistStats = append(topArtistStats, artistStat{name: name, plays: plays})
	}

	for i := 0; i < len(topArtistStats)-1; i++ {
		for j := i + 1; j < len(topArtistStats); j++ {
			if topArtistStats[i].plays < topArtistStats[j].plays {
				topArtistStats[i], topArtistStats[j] = topArtistStats[j], topArtistStats[i]
			}
		}
	}

	if len(topArtistStats) > 5 {
		topArtistStats = topArtistStats[:5]
	}

	artistsList := container.NewVBox()
	for i, artist := range topArtistStats {
		artistItem := container.NewHBox(
			widget.NewLabel(fmt.Sprintf("%d.", i+1)),
			widget.NewLabel(artist.name),
			widget.NewLabel(fmt.Sprintf("(%d plays)", artist.plays)),
		)
		artistsList.Add(artistItem)
	}

	if len(topArtistStats) == 0 {
		artistsList.Add(widget.NewLabel("No artist data yet"))
	}

	sv.topArtistsCard.SetContent(artistsList)
}

func (sv *StatsView) updateRecentActivity(songs []*types.Song) {
	recentSongs := make([]*types.Song, 0)
	for _, song := range songs {
		if song.Played > 0 {
			recentSongs = append(recentSongs, song)
		}
	}

	for i := 0; i < len(recentSongs)-1; i++ {
		for j := i + 1; j < len(recentSongs); j++ {
			if recentSongs[i].UpdatedAt.Before(recentSongs[j].UpdatedAt) {
				recentSongs[i], recentSongs[j] = recentSongs[j], recentSongs[i]
			}
		}
	}

	if len(recentSongs) > 10 {
		recentSongs = recentSongs[:10]
	}

	activityList := container.NewVBox()
	for _, song := range recentSongs {
		artistName := "Unknown Artist"
		if len(song.Authors) > 0 {
			artistName = song.Authors[0].Name
		}

		timeAgo := time.Since(song.UpdatedAt)
		var timeStr string
		if timeAgo < time.Hour {
			timeStr = fmt.Sprintf("%dm ago", int(timeAgo.Minutes()))
		} else if timeAgo < 24*time.Hour {
			timeStr = fmt.Sprintf("%dh ago", int(timeAgo.Hours()))
		} else {
			timeStr = fmt.Sprintf("%dd ago", int(timeAgo.Hours()/24))
		}

		activityItem := container.NewHBox(
			widget.NewLabel(song.Name),
			widget.NewLabel("-"),
			widget.NewLabel(artistName),
			widget.NewLabel(timeStr),
		)
		activityList.Add(activityItem)
	}

	if len(recentSongs) == 0 {
		activityList.Add(widget.NewLabel("No recent activity"))
	}

	sv.recentActivityCard.SetContent(activityList)
}

func getTopSongs(songs []*types.Song, limit int) []*types.Song {
	playedSongs := make([]*types.Song, 0)
	for _, song := range songs {
		if song.Played > 0 {
			playedSongs = append(playedSongs, song)
		}
	}

	for i := 0; i < len(playedSongs)-1; i++ {
		for j := i + 1; j < len(playedSongs); j++ {
			if playedSongs[i].Played < playedSongs[j].Played {
				playedSongs[i], playedSongs[j] = playedSongs[j], playedSongs[i]
			}
		}
	}

	if len(playedSongs) > limit {
		playedSongs = playedSongs[:limit]
	}

	return playedSongs
}

func (sv *StatsView) Refresh() {
	sv.loadStats()
}

func (sv *StatsView) Container() *fyne.Container {
	return sv.container
}
