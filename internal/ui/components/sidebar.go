package components

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/config"
)

type Sidebar struct {
	cfg       *config.Config
	container *fyne.Container

	songsBtn    *widget.Button
	albumsBtn   *widget.Button
	artistsBtn  *widget.Button
	playlistBtn *widget.Button
	downloadBtn *widget.Button
	statsBtn    *widget.Button
	settingsBtn *widget.Button

	userCard         *widget.Card
	authBtn          *widget.Button
	userLabel        *widget.Label
	statusLabel      *widget.Label
	statsLabel       *widget.Label
	timeLabel        *widget.Label
	offlineIndicator *widget.Icon

	onNavigate      func(string)
	onAuthRequested func()

	isAuthenticated bool
	currentView     string
	compactMode     bool
	showStats       bool
}

func NewSidebar(cfg *config.Config) *Sidebar {
	s := &Sidebar{
		cfg:       cfg,
		showStats: cfg.UI.ShowStats,
	}

	s.setupWidgets()
	s.setupLayout()
	s.setActiveView("songs")

	return s
}

func (s *Sidebar) setupWidgets() {
	s.songsBtn = widget.NewButtonWithIcon("Songs", theme.MediaMusicIcon(), func() {
		s.navigate("songs")
	})

	s.albumsBtn = widget.NewButtonWithIcon("Albums", theme.FolderIcon(), func() {
		s.navigate("albums")
	})

	s.artistsBtn = widget.NewButtonWithIcon("Artists", theme.AccountIcon(), func() {
		s.navigate("artists")
	})

	s.playlistBtn = widget.NewButtonWithIcon("Playlists", theme.ListIcon(), func() {
		s.navigate("playlists")
	})

	s.downloadBtn = widget.NewButtonWithIcon("Downloads", theme.DownloadIcon(), func() {
		s.navigate("downloads")
	})

	s.statsBtn = widget.NewButtonWithIcon("Statistics", theme.InfoIcon(), func() {
		s.navigate("stats")
	})

	s.settingsBtn = widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		s.navigate("settings")
	})

	s.setupButtonStyles()

	s.userLabel = widget.NewLabel("Not logged in")
	s.userLabel.TextStyle = fyne.TextStyle{Bold: true}

	s.statusLabel = widget.NewLabel("Offline mode")
	s.statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	s.statsLabel = widget.NewLabel("0 songs")
	s.statsLabel.TextStyle = fyne.TextStyle{Italic: true}

	s.timeLabel = widget.NewLabel("0h 0m listened")
	s.timeLabel.TextStyle = fyne.TextStyle{Italic: true}

	s.offlineIndicator = widget.NewIcon(theme.WarningIcon())
	s.offlineIndicator.Resize(fyne.NewSize(16, 16))

	s.authBtn = widget.NewButtonWithIcon("Login", theme.LoginIcon(), func() {
		if s.onAuthRequested != nil {
			s.onAuthRequested()
		}
	})

	s.setupUserCard()
}

func (s *Sidebar) setupButtonStyles() {
	buttons := []*widget.Button{
		s.songsBtn, s.albumsBtn, s.artistsBtn,
		s.playlistBtn, s.downloadBtn, s.statsBtn, s.settingsBtn,
	}

	for _, btn := range buttons {
		btn.Alignment = widget.ButtonAlignLeading
		btn.Importance = widget.MediumImportance
	}
}

func (s *Sidebar) setupUserCard() {
	statusContainer := container.NewHBox(s.statusLabel, s.offlineIndicator)

	userContent := container.NewVBox(
		s.userLabel,
		statusContainer,
		s.authBtn,
	)

	if s.showStats || s.cfg.UI.ShowStats {
		userContent.Add(widget.NewSeparator())
		userContent.Add(widget.NewLabel("Library Stats"))
		userContent.Add(s.statsLabel)
		userContent.Add(s.timeLabel)
	}

	s.userCard = widget.NewCard("", "", userContent)
}

func (s *Sidebar) setupLayout() {
	navigation := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("Library"),
		s.songsBtn,
		s.albumsBtn,
		s.artistsBtn,
		s.playlistBtn,
		widget.NewSeparator(),
		widget.NewLabel("Analytics"),
		s.statsBtn,
		widget.NewSeparator(),
		widget.NewLabel("Tools"),
		s.downloadBtn,
		s.settingsBtn,
		widget.NewSeparator(),
	)

	content := container.NewScroll(navigation)

	headerLabel := widget.NewLabel("AMP")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}

	s.container = container.NewBorder(
		container.NewVBox(
			headerLabel,
			widget.NewSeparator(),
		),
		s.userCard,
		nil,
		nil,
		content,
	)
}

func (s *Sidebar) navigate(view string) {
	s.setActiveView(view)
	if s.onNavigate != nil {
		s.onNavigate(view)
	}
}

func (s *Sidebar) setActiveView(view string) {
	buttons := map[string]*widget.Button{
		"songs":     s.songsBtn,
		"albums":    s.albumsBtn,
		"artists":   s.artistsBtn,
		"playlists": s.playlistBtn,
		"downloads": s.downloadBtn,
		"stats":     s.statsBtn,
		"settings":  s.settingsBtn,
	}

	for name, btn := range buttons {
		if name == view {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
		btn.Refresh()
	}

	s.currentView = view
}

func (s *Sidebar) SetCompactMode(compact bool) {
	s.compactMode = compact

	if compact {
		s.songsBtn.SetText("")
		s.albumsBtn.SetText("")
		s.artistsBtn.SetText("")
		s.playlistBtn.SetText("")
		s.downloadBtn.SetText("")
		s.statsBtn.SetText("")
		s.settingsBtn.SetText("")

		s.userCard.SetTitle("")
	} else {
		s.songsBtn.SetText("Songs")
		s.albumsBtn.SetText("Albums")
		s.artistsBtn.SetText("Artists")
		s.playlistBtn.SetText("Playlists")
		s.downloadBtn.SetText("Downloads")
		s.statsBtn.SetText("Statistics")
		s.settingsBtn.SetText("Settings")

		s.userCard.SetTitle("")
	}
}

func (s *Sidebar) OnNavigate(callback func(string)) {
	s.onNavigate = callback
}

func (s *Sidebar) OnAuthRequested(callback func()) {
	s.onAuthRequested = callback
}

func (s *Sidebar) SetAuthenticated(authenticated bool, username string) {
	s.isAuthenticated = authenticated

	if authenticated {
		s.authBtn.SetText("Logout")
		s.authBtn.SetIcon(theme.LogoutIcon())

		if username != "" {
			s.userLabel.SetText(username)
		} else {
			s.userLabel.SetText("Logged in")
		}

		s.statusLabel.SetText("Online")
		s.offlineIndicator.SetResource(theme.ConfirmIcon())
	} else {
		s.authBtn.SetText("Login")
		s.authBtn.SetIcon(theme.LoginIcon())
		s.userLabel.SetText("Not logged in")
		s.statusLabel.SetText("Offline mode")
		s.offlineIndicator.SetResource(theme.WarningIcon())
	}

	s.setupUserCard()
	s.setupLayout()
}

func (s *Sidebar) UpdateStats(songCount int, timeListened string) {
	if songCount == 0 {
		s.statsLabel.SetText("No songs yet")
	} else if songCount == 1 {
		s.statsLabel.SetText("1 song")
	} else {
		s.statsLabel.SetText(fmt.Sprintf("%d songs", songCount))
	}

	if timeListened == "0h 0m" {
		s.timeLabel.SetText("No listening time")
	} else {
		s.timeLabel.SetText(timeListened + " listened")
	}
}

func (s *Sidebar) SetShowStats(show bool) {
	s.showStats = show
	s.cfg.UI.ShowStats = show
	s.setupUserCard()
	s.setupLayout()
}

func (s *Sidebar) IsStatsVisible() bool {
	return s.showStats || s.cfg.UI.ShowStats
}

func (s *Sidebar) Container() *fyne.Container {
	return s.container
}
