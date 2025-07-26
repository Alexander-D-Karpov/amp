package ui

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/audio"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/search"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/internal/ui/components"
	"github.com/Alexander-D-Karpov/amp/internal/ui/themes"
	"github.com/Alexander-D-Karpov/amp/internal/ui/views"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	ctx     context.Context
	cfg     *config.Config

	api             *api.Client
	storage         *storage.Database
	player          *audio.Player
	search          *search.Engine
	downloadManager *download.Manager
	syncManager     *storage.SyncManager

	playerBar        *components.PlayerBar
	sidebar          *components.Sidebar
	mainView         *views.MainView
	authDialog       *components.AuthDialog
	statusBar        *widget.Label
	loadingIndicator *widget.ProgressBar

	isAuthenticated bool
	currentQueue    []*types.Song
	currentIndex    int
	lastWindowSize  fyne.Size

	syncInProgress bool
}

func NewApp(ctx context.Context, fyneApp fyne.App, cfg *config.Config) (*App, error) {
	fyneApp.Settings().SetTheme(themes.NewTheme(cfg.UI.Theme))

	apiClient := api.NewClient(cfg)

	storageDB, err := storage.NewDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	syncManager := storage.NewSyncManager(apiClient, storageDB, cfg)

	player, err := audio.NewPlayer(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize audio player: %w", err)
	}

	searchEngine := search.NewEngine(cfg, storageDB)
	downloadManager := download.NewManager(cfg)

	window := fyneApp.NewWindow("AMP - A(dvanced)karpov Music Player")
	window.Resize(fyne.NewSize(float32(cfg.UI.WindowWidth), float32(cfg.UI.WindowHeight)))
	window.CenterOnScreen()

	app := &App{
		fyneApp:         fyneApp,
		window:          window,
		ctx:             ctx,
		cfg:             cfg,
		api:             apiClient,
		storage:         storageDB,
		player:          player,
		search:          searchEngine,
		downloadManager: downloadManager,
		syncManager:     syncManager,
		currentQueue:    make([]*types.Song, 0),
		currentIndex:    -1,
		lastWindowSize:  window.Canvas().Size(),
	}

	app.debugLog("AMP Application initializing...")

	if err := app.setupUI(); err != nil {
		return nil, fmt.Errorf("setup UI: %w", err)
	}

	if err := app.setupEventHandlers(); err != nil {
		return nil, fmt.Errorf("setup event handlers: %w", err)
	}

	app.setupKeyboardShortcuts()
	app.loadSavedState()
	app.startBackgroundTasks()

	app.debugLog("AMP Application initialized successfully")
	return app, nil
}

func (a *App) debugLog(format string, args ...interface{}) {
	if a.cfg.Debug {
		log.Printf("[APP] "+format, args...)
	}
}

func (a *App) setupUI() error {
	a.debugLog("Setting up UI components...")

	a.playerBar = components.NewPlayerBar(a.player, a.storage)
	a.playerBar.SetConfig(a.cfg)

	a.sidebar = components.NewSidebar(a.cfg)
	a.mainView = views.NewMainView(a.api, a.storage, a.search, a.downloadManager, a.cfg)
	a.authDialog = components.NewAuthDialog(a.api)

	a.statusBar = widget.NewLabel("Ready")
	a.loadingIndicator = widget.NewProgressBar()
	a.loadingIndicator.Hide()

	statusContainer := container.NewBorder(
		nil, nil,
		a.statusBar, a.loadingIndicator,
		nil,
	)

	content := container.NewBorder(
		nil,
		a.playerBar.Container(),
		a.sidebar.Container(),
		nil,
		a.mainView.Container(),
	)

	fullContent := container.NewBorder(
		nil,
		statusContainer,
		nil,
		nil,
		content,
	)

	a.window.SetContent(fullContent)

	// Set up window resize callback
	a.window.SetOnClosed(func() {
		a.Close()
	})

	// Monitor for window size changes
	go a.monitorWindowResize()

	a.debugLog("UI components setup complete")
	return nil
}

func (a *App) monitorWindowResize() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.handleWindowResize()
		}
	}
}

func (a *App) handleWindowResize() {
	currentSize := a.window.Canvas().Size()
	if currentSize != a.lastWindowSize {
		a.debugLog("Window resized: %v -> %v", a.lastWindowSize, currentSize)
		a.lastWindowSize = currentSize
		a.updateLayoutForSize(currentSize)

		a.cfg.UI.WindowWidth = int(currentSize.Width)
		a.cfg.UI.WindowHeight = int(currentSize.Height)
		go func() {
			if err := a.cfg.Save(); err != nil {
				a.debugLog("Failed to save window size: %v", err)
			}
		}()
	}
}

func (a *App) updateLayoutForSize(size fyne.Size) {
	isCompact := size.Width < 800

	a.debugLog("Updating layout - compact mode: %v (width: %.0f)", isCompact, size.Width)

	a.sidebar.SetCompactMode(isCompact)
	a.mainView.SetCompactMode(isCompact)

	if size.Height < 600 {
		a.playerBar.SetCompactMode(true)
	} else {
		a.playerBar.SetCompactMode(false)
	}

	a.mainView.RefreshLayout()
}

func (a *App) setupEventHandlers() error {
	a.debugLog("Setting up event handlers...")

	a.mainView.SettingsView.SetParentWindow(a.window)
	a.mainView.PlaylistsView.SetParentWindow(a.window)

	a.sidebar.OnNavigate(func(view string) {
		a.debugLog("Navigation requested: %s", view)
		a.mainView.ShowView(view)
		a.updateStatus("Viewing " + view)
	})

	a.sidebar.OnAuthRequested(func() {
		if a.isAuthenticated {
			a.debugLog("Logout requested")
			a.logout()
		} else {
			a.debugLog("Login requested")
			a.authDialog.Show(a.window)
		}
	})

	a.authDialog.OnAuthenticated(func(token string) {
		a.debugLog("Authentication successful with token: %s...", token[:min(len(token), 10)])
		a.handleAuthentication(token)
	})

	a.mainView.OnSongSelected(func(song *types.Song) {
		a.debugLog("Song selected: %s by %s", song.Name, getArtistNames(song.Authors))
		a.playSong(song)
	})

	a.mainView.OnAlbumSelected(func(album *types.Album) {
		a.debugLog("Album selected: %s", album.Name)
		a.updateStatus(fmt.Sprintf("Selected album: %s", album.Name))
	})

	a.mainView.OnArtistSelected(func(artist *types.Author) {
		a.debugLog("Artist selected: %s", artist.Name)
		a.updateStatus(fmt.Sprintf("Selected artist: %s", artist.Name))
	})

	a.mainView.OnPlaylistSelected(func(playlist *types.Playlist) {
		a.debugLog("Playlist selected: %s (%d songs)", playlist.Name, len(playlist.Songs))
		a.updateStatus(fmt.Sprintf("Selected playlist: %s", playlist.Name))
		if len(playlist.Songs) > 0 {
			a.playPlaylist(playlist)
		}
	})

	a.playerBar.OnNext(func() {
		a.debugLog("Next track requested")
		a.updateStatus("Next song")
	})

	a.playerBar.OnPrevious(func() {
		a.debugLog("Previous track requested")
		a.updateStatus("Previous song")
	})

	a.playerBar.OnShuffle(func(enabled bool) {
		a.debugLog("Shuffle toggled: %v", enabled)
		a.updateStatus(fmt.Sprintf("Shuffle %s", map[bool]string{true: "enabled", false: "disabled"}[enabled]))
	})

	a.playerBar.OnRepeat(func(mode components.RepeatMode) {
		a.debugLog("Repeat mode changed: %s", mode.String())
		a.updateStatus(fmt.Sprintf("Repeat: %s", mode.String()))
	})

	a.downloadManager.OnProgress(func(progress *types.DownloadProgress) {
		switch progress.Status {
		case types.DownloadStatusCompleted:
			a.debugLog("Download completed: %s", progress.Filename)
			a.updateStatus(fmt.Sprintf("Downloaded: %s", progress.Filename))
		case types.DownloadStatusFailed:
			a.debugLog("Download failed: %s - %v", progress.Filename, progress.Error)
			a.updateStatus(fmt.Sprintf("Download failed: %s", progress.Filename))
		}
	})

	a.setupSyncEventHandlers()

	a.debugLog("Event handlers setup complete")
	return nil
}

func (a *App) setupSyncEventHandlers() {
	a.syncManager.OnProgress(func(status string, current, total int) {
		if a.cfg.Debug {
			a.debugLog("Sync progress: %s (%d/%d)", status, current, total)
		}

		if current < total {
			a.showLoading(true)
			a.loadingIndicator.SetValue(float64(current) / float64(total))
			a.updateStatus(status)
		}
	})

	a.syncManager.OnError(func(err error) {
		a.debugLog("Sync error: %v", err)
		a.updateStatus(fmt.Sprintf("Sync error: %v", err))
		a.showLoading(false)
	})

	a.syncManager.OnComplete(func() {
		a.debugLog("Sync completed successfully")
		a.updateStatus("Sync completed")
		a.showLoading(false)
		a.syncInProgress = false

		go func() {
			a.mainView.RefreshData()
			a.updateLibraryStats()
		}()
	})
}

func (a *App) setupKeyboardShortcuts() {
	a.debugLog("Setting up keyboard shortcuts...")

	a.window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		switch key.Name {
		case fyne.KeySpace:
			if a.player.IsPlaying() {
				_ = a.player.Pause()
				a.debugLog("Playback paused via spacebar")
				a.updateStatus("Paused")
			} else {
				_ = a.player.Resume()
				a.debugLog("Playback resumed via spacebar")
				a.updateStatus("Playing")
			}
		case fyne.KeyRight:
			if key.Physical.ScanCode == 0 {
				currentPos := a.player.GetPosition()
				duration := a.player.GetDuration()
				newPos := currentPos + 10*time.Second
				if newPos > duration {
					newPos = duration
				}
				_ = a.player.Seek(newPos)
				a.debugLog("Seek forward: %v", newPos)
			}
		case fyne.KeyLeft:
			if key.Physical.ScanCode == 0 {
				currentPos := a.player.GetPosition()
				newPos := currentPos - 10*time.Second
				if newPos < 0 {
					newPos = 0
				}
				_ = a.player.Seek(newPos)
				a.debugLog("Seek backward: %v", newPos)
			}
		case fyne.KeyF:
			a.window.SetFullScreen(!a.window.FullScreen())
			a.debugLog("Fullscreen toggled: %v", a.window.FullScreen())
		case fyne.KeyEscape:
			if a.window.FullScreen() {
				a.window.SetFullScreen(false)
				a.debugLog("Exited fullscreen")
			}
		}
	})

	a.window.Canvas().SetOnTypedRune(func(r rune) {
		if r == 'f' || r == 'F' {
			a.focusSearch()
		}
	})
}

func (a *App) loadSavedState() {
	a.debugLog("Loading saved application state...")

	if a.cfg.API.Token != "" && !a.cfg.User.IsAnonymous {
		a.debugLog("Found saved authentication for user: %s", a.cfg.User.Username)
		a.isAuthenticated = true
		a.sidebar.SetAuthenticated(true, a.cfg.User.Username)
		a.startSync()
	} else {
		a.debugLog("No saved authentication found, initializing anonymous mode")
		a.initializeAnonymous()
	}
}

func (a *App) initializeAnonymous() {
	a.debugLog("Initializing anonymous mode...")
	a.updateStatus("Initializing offline mode...")

	go func() {
		ctx := context.Background()

		anonID, err := a.api.GetAnonymousToken(ctx)
		if err != nil {
			a.debugLog("Failed to get anonymous token: %v", err)
			a.updateStatus("Offline mode - no network")
		} else {
			a.cfg.User.AnonymousID = anonID
			if err := a.cfg.Save(); err != nil {
				a.debugLog("Failed to save anonymous token: %v", err)
			} else {
				a.debugLog("Anonymous token saved successfully")
			}
		}

		a.startSync()
	}()
}

func (a *App) handleAuthentication(token string) {
	a.isAuthenticated = true
	a.cfg.API.Token = token
	a.cfg.User.IsAnonymous = false
	a.api.SetToken(token)

	go func() {
		ctx := context.Background()
		if user, err := a.api.GetCurrentUser(ctx); err == nil {
			a.cfg.User.ID = user.ID
			a.cfg.User.Username = user.Username
			a.cfg.User.Email = user.Email
			if user.ImageCropped != nil {
				a.cfg.User.Image = *user.ImageCropped
			}

			if err := a.cfg.Save(); err != nil {
				a.debugLog("Failed to save user info: %v", err)
			} else {
				a.debugLog("User info saved: %s (%s)", user.Username, user.Email)
			}
		} else {
			a.debugLog("Failed to get user info: %v", err)
		}
	}()

	a.sidebar.SetAuthenticated(true, a.cfg.User.Username)
	a.updateStatus("Authenticated successfully")
	a.startSync()
}

func (a *App) startBackgroundTasks() {
	a.debugLog("Starting background tasks...")

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				a.debugLog("Background task stopping: library stats updater")
				return
			case <-ticker.C:
				a.updateLibraryStats()
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				a.debugLog("Background task stopping: download checker")
				return
			case <-ticker.C:
				a.checkDownloads()
			}
		}
	}()

	a.debugLog("Background tasks started")
}

func (a *App) playSong(song *types.Song) {
	a.currentQueue = []*types.Song{song}
	a.currentIndex = 0

	a.playerBar.SetQueue(a.currentQueue, a.currentIndex)
	a.updateStatus(fmt.Sprintf("Playing: %s", song.Name))

	if a.cfg.Download.AutoDownload && !song.Downloaded {
		go func() {
			ctx := context.Background()
			if err := a.downloadManager.DownloadSong(ctx, song); err != nil {
				a.debugLog("Auto-download failed for %s: %v", song.Name, err)
			} else {
				a.debugLog("Auto-download started for: %s", song.Name)
			}
		}()
	}
}

func (a *App) playPlaylist(playlist *types.Playlist) {
	if len(playlist.Songs) == 0 {
		a.updateStatus("Playlist is empty")
		return
	}

	a.currentQueue = playlist.Songs
	a.currentIndex = 0

	a.playerBar.SetQueue(a.currentQueue, a.currentIndex)
	a.updateStatus(fmt.Sprintf("Playing playlist: %s", playlist.Name))
}

func (a *App) startSync() {
	if a.syncInProgress {
		a.debugLog("Sync already in progress, skipping")
		return
	}

	a.debugLog("Starting sync process...")
	a.syncInProgress = true
	a.updateStatus("Starting sync...")
	a.showLoading(true)

	go func() {
		a.syncManager.Start(a.ctx)
	}()
}

func (a *App) logout() {
	a.debugLog("Logging out user: %s", a.cfg.User.Username)

	go func() {
		ctx := context.Background()
		if err := a.api.Logout(ctx); err != nil {
			a.debugLog("API logout failed: %v", err)
		}
	}()

	a.isAuthenticated = false
	a.cfg.API.Token = ""
	a.cfg.User.IsAnonymous = true
	a.cfg.User.Username = ""
	a.cfg.User.Email = ""
	a.cfg.User.Image = ""
	a.cfg.User.AnonymousID = ""

	if err := a.cfg.Save(); err != nil {
		a.debugLog("Failed to save logout state: %v", err)
	}

	a.sidebar.SetAuthenticated(false, "")
	a.syncManager.Stop()
	a.api.SetToken("")

	a.initializeAnonymous()
	a.updateStatus("Logged out")
}

func (a *App) updateStatus(message string) {
	a.statusBar.SetText(message)

	go func() {
		time.Sleep(5 * time.Second)
		a.statusBar.SetText("Ready")
	}()
}

func (a *App) showLoading(show bool) {
	if show {
		a.loadingIndicator.Show()
	} else {
		a.loadingIndicator.Hide()
	}
}

func (a *App) updateLibraryStats() {
	go func() {
		ctx := context.Background()
		songs, err := a.storage.GetSongs(ctx, 10000, 0)
		if err != nil {
			a.debugLog("Failed to get songs for stats: %v", err)
			return
		}

		totalPlayed := 0
		for _, song := range songs {
			totalPlayed += song.Played * song.Length
		}

		hours := totalPlayed / 3600
		minutes := (totalPlayed % 3600) / 60
		timeListened := fmt.Sprintf("%dh %dm", hours, minutes)

		a.sidebar.UpdateStats(len(songs), timeListened)

		if a.cfg.Debug && len(songs) > 0 {
			a.debugLog("Library stats updated - Songs: %d, Time listened: %s", len(songs), timeListened)
		}
	}()
}

func (a *App) checkDownloads() {
	downloads := a.downloadManager.GetAllDownloads()
	activeDownloads := 0

	for _, download := range downloads {
		if download.Status == types.DownloadStatusDownloading {
			activeDownloads++
		}
	}

	if activeDownloads > 0 && a.cfg.Debug {
		a.debugLog("Active downloads: %d", activeDownloads)
	}
}

func (a *App) focusSearch() {
	a.debugLog("Focusing search in current view: %s", a.mainView.GetCurrentView())
	a.mainView.SearchInCurrentView("")
}

func (a *App) ShowAndRun() {
	a.debugLog("Starting AMP application window...")
	a.window.ShowAndRun()
}

func (a *App) Close() {
	a.debugLog("Shutting down AMP application...")

	if a.syncManager != nil {
		a.syncManager.Stop()
	}

	if a.player != nil {
		if err := a.player.Close(); err != nil {
			a.debugLog("Error closing audio player: %v", err)
		}
	}

	if a.storage != nil {
		if err := a.storage.Close(); err != nil {
			a.debugLog("Error closing database: %v", err)
		}
	}

	a.debugLog("AMP application shutdown complete")
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
