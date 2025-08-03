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
	widget.BaseWidget
	cfg *config.Config

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
	breakpoint      float32
}

func NewSidebar(cfg *config.Config) *Sidebar {
	s := &Sidebar{
		cfg:         cfg,
		breakpoint:  800.0,
		currentView: "songs",
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *Sidebar) CreateRenderer() fyne.WidgetRenderer {
	s.songsBtn = widget.NewButtonWithIcon("Songs", theme.MediaMusicIcon(), func() { s.navigate("songs") })
	s.albumsBtn = widget.NewButtonWithIcon("Albums", theme.FolderIcon(), func() { s.navigate("albums") })
	s.artistsBtn = widget.NewButtonWithIcon("Artists", theme.AccountIcon(), func() { s.navigate("artists") })
	s.playlistBtn = widget.NewButtonWithIcon("Playlists", theme.ListIcon(), func() { s.navigate("playlists") })
	s.downloadBtn = widget.NewButtonWithIcon("Downloads", theme.DownloadIcon(), func() { s.navigate("downloads") })
	s.statsBtn = widget.NewButtonWithIcon("Statistics", theme.InfoIcon(), func() { s.navigate("stats") })
	s.settingsBtn = widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() { s.navigate("settings") })

	s.authBtn = widget.NewButtonWithIcon("Login", theme.LoginIcon(), func() {
		if s.onAuthRequested != nil {
			s.onAuthRequested()
		}
	})

	s.userLabel = widget.NewLabel("Not logged in")
	s.userLabel.TextStyle = fyne.TextStyle{Bold: true}
	s.statusLabel = widget.NewLabel("Offline mode")
	s.offlineIndicator = widget.NewIcon(theme.WarningIcon())
	s.statsLabel = widget.NewLabel("0 songs")
	s.timeLabel = widget.NewLabel("0h 0m listened")
	s.userCard = widget.NewCard("", "", nil)

	r := &sidebarRenderer{
		sidebar: s,
	}
	r.mainContainer = container.NewBorder(nil, s.userCard, nil, nil, nil)
	r.Refresh()
	return r
}

func (s *Sidebar) GetBreakpoint() float32 {
	return s.breakpoint
}

func (s *Sidebar) navigate(view string) {
	if s.currentView == view {
		return
	}
	s.currentView = view
	s.Refresh()
	if s.onNavigate != nil {
		s.onNavigate(view)
	}
}

func (s *Sidebar) SetCompactMode(compact bool) {
	if s.compactMode == compact {
		return
	}
	s.compactMode = compact
	s.Refresh()
}

func (s *Sidebar) SetAuthenticated(authenticated bool, username string) {
	s.isAuthenticated = authenticated
	if s.userLabel != nil {
		s.userLabel.SetText(username)
	}
	s.Refresh()
}

func (s *Sidebar) UpdateStats(songCount int, timeListened string) {
	if s.statsLabel != nil {
		s.statsLabel.SetText(fmt.Sprintf("%d songs", songCount))
	}
	if s.timeLabel != nil {
		s.timeLabel.SetText(timeListened)
	}
	s.Refresh()
}

func (s *Sidebar) SetShowStats(show bool) {
	if s.cfg.UI.ShowStats == show {
		return
	}
	s.cfg.UI.ShowStats = show
	s.Refresh()
}

func (s *Sidebar) OnNavigate(callback func(string)) {
	s.onNavigate = callback
}

func (s *Sidebar) OnAuthRequested(callback func()) {
	s.onAuthRequested = callback
}

type sidebarRenderer struct {
	sidebar       *Sidebar
	mainContainer *fyne.Container
}

func (r *sidebarRenderer) Layout(size fyne.Size) {
	r.mainContainer.Resize(size)
}

func (r *sidebarRenderer) MinSize() fyne.Size {
	nav := r.createNavContainer()
	minSize := nav.MinSize()

	if r.sidebar.compactMode {
		minSize.Width = r.sidebar.songsBtn.MinSize().Width + theme.Padding()*2
	} else {
		minSize.Width += r.sidebar.userCard.MinSize().Width
	}
	return minSize
}

func (r *sidebarRenderer) Refresh() {
	r.updateButtonStylesAndText()
	r.updateUserCardContent()

	navContainer := container.NewScroll(r.createNavContainer())
	r.mainContainer.Objects = []fyne.CanvasObject{container.NewBorder(nil, r.sidebar.userCard, nil, nil, navContainer)}
	r.mainContainer.Refresh()
}

func (r *sidebarRenderer) createNavContainer() *fyne.Container {
	var navObjects []fyne.CanvasObject
	if r.sidebar.compactMode {
		navObjects = []fyne.CanvasObject{
			r.sidebar.songsBtn, r.sidebar.albumsBtn, r.sidebar.artistsBtn, r.sidebar.playlistBtn,
			widget.NewSeparator(),
			r.sidebar.downloadBtn, r.sidebar.statsBtn, r.sidebar.settingsBtn,
		}
	} else {
		headerLabel := widget.NewLabel("AMP")
		headerLabel.TextStyle = fyne.TextStyle{Bold: true}
		headerLabel.Alignment = fyne.TextAlignCenter
		navObjects = []fyne.CanvasObject{
			headerLabel, widget.NewSeparator(),
			widget.NewLabel("Library"),
			r.sidebar.songsBtn, r.sidebar.albumsBtn, r.sidebar.artistsBtn, r.sidebar.playlistBtn,
			widget.NewSeparator(), widget.NewLabel("Tools"),
			r.sidebar.downloadBtn, r.sidebar.statsBtn, r.sidebar.settingsBtn,
		}
	}
	return container.NewVBox(navObjects...)
}

func (r *sidebarRenderer) updateUserCardContent() {
	var userContent fyne.CanvasObject
	if r.sidebar.compactMode {
		userContent = r.sidebar.authBtn
	} else {
		statusContainer := container.NewHBox(r.sidebar.statusLabel, r.sidebar.offlineIndicator)
		vbox := container.NewVBox(r.sidebar.userLabel, statusContainer, r.sidebar.authBtn)
		if r.sidebar.cfg.UI.ShowStats {
			vbox.Add(widget.NewSeparator())
			vbox.Add(r.sidebar.statsLabel)
			vbox.Add(r.sidebar.timeLabel)
		}
		userContent = vbox
	}
	r.sidebar.userCard.SetContent(userContent)
}

func (r *sidebarRenderer) updateButtonStylesAndText() {
	buttons := map[string]*widget.Button{
		"songs": r.sidebar.songsBtn, "albums": r.sidebar.albumsBtn, "artists": r.sidebar.artistsBtn,
		"playlists": r.sidebar.playlistBtn, "downloads": r.sidebar.downloadBtn, "stats": r.sidebar.statsBtn, "settings": r.sidebar.settingsBtn,
	}
	labels := map[string]string{
		"songs": "Songs", "albums": "Albums", "artists": "Artists", "playlists": "Playlists",
		"downloads": "Downloads", "stats": "Statistics", "settings": "Settings",
	}

	for name, btn := range buttons {
		if r.sidebar.compactMode {
			btn.SetText("")
		} else {
			btn.SetText(labels[name])
		}
		btn.Alignment = widget.ButtonAlignLeading
		if name == r.sidebar.currentView {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
	}

	if r.sidebar.isAuthenticated {
		r.sidebar.authBtn.SetIcon(theme.LogoutIcon())
		if !r.sidebar.compactMode {
			r.sidebar.authBtn.SetText("Logout")
		} else {
			r.sidebar.authBtn.SetText("")
		}
		r.sidebar.statusLabel.SetText("Online")
		r.sidebar.offlineIndicator.SetResource(theme.ConfirmIcon())
	} else {
		r.sidebar.authBtn.SetIcon(theme.LoginIcon())
		if !r.sidebar.compactMode {
			r.sidebar.authBtn.SetText("Login")
		} else {
			r.sidebar.authBtn.SetText("")
		}
		r.sidebar.statusLabel.SetText("Offline mode")
		r.sidebar.offlineIndicator.SetResource(theme.WarningIcon())
	}
}

func (r *sidebarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.mainContainer}
}

func (r *sidebarRenderer) Destroy() {}
