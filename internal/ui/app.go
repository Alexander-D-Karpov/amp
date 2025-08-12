package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/audio"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/internal/handlers"
	"github.com/Alexander-D-Karpov/amp/internal/media"
	"github.com/Alexander-D-Karpov/amp/internal/search"
	"github.com/Alexander-D-Karpov/amp/internal/services"
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

	core     *Core
	ui       *UIComponents
	state    *AppState
	eventBus *handlers.EventBus

	mainContainer *fyne.Container
	lastSize      fyne.Size
}

type Core struct {
	api             *api.Client
	storage         *storage.Database
	player          *audio.Player
	searchEngine    *search.SearchEngine
	downloadManager *download.Manager
	syncManager     *storage.SyncManager
	imageLoader     *media.ImageLoader
	musicService    *services.MusicService
	imageService    *services.ImageService
	playSyncService *services.PlaySyncService
}

type UIComponents struct {
	playerBar        *components.PlayerBar
	sidebar          *components.Sidebar
	mainView         *views.MainView
	authDialog       *components.AuthDialog
	statusBar        *widget.Label
	loadingIndicator *widget.ProgressBarInfinite
}

type AppState struct {
	isAuthenticated bool
	currentQueue    []*types.Song
	currentIndex    int
	compactMode     bool
	syncInProgress  bool
}

func NewApp(ctx context.Context, fyneApp fyne.App, cfg *config.Config) (*App, error) {
	fyneApp.Settings().SetTheme(themes.NewTheme(cfg.UI.Theme))

	core, err := initCore(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize core: %w", err)
	}

	window := fyneApp.NewWindow("AMP - A(dvanced)karpov Music Player")
	window.Resize(fyne.NewSize(float32(cfg.UI.WindowWidth), float32(cfg.UI.WindowHeight)))
	window.CenterOnScreen()

	app := &App{
		fyneApp: fyneApp,
		window:  window,
		ctx:     ctx,
		cfg:     cfg,
		core:    core,
		state: &AppState{
			currentQueue: make([]*types.Song, 0),
			currentIndex: -1,
		},
		eventBus: handlers.NewEventBus(),
		lastSize: window.Canvas().Size(),
	}

	if err := app.initUI(); err != nil {
		return nil, fmt.Errorf("initialize UI: %w", err)
	}

	app.setupEventHandlers()
	app.setupKeyboardShortcuts()
	app.loadSavedState()
	app.startBackgroundTasks()
	app.startResizePolling()

	if cfg.Debug {
		log.Printf("[APP] AMP Application initialized successfully")
	}
	return app, nil
}

func initCore(cfg *config.Config) (*Core, error) {
	apiClient := api.NewClient(cfg)
	if cfg.User.IsAnonymous && cfg.API.Token == "" {
		if _, err := apiClient.EnsureAnonymousToken(context.Background()); err != nil {
			log.Printf("anon token create failed: %v", err)
		}
	}

	storageDB, err := storage.NewDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	imageLoader, err := media.NewImageLoader(cfg, storageDB)
	if err != nil {
		return nil, fmt.Errorf("initialize image loader: %w", err)
	}
	player, err := audio.NewPlayer(cfg, storageDB)
	if err != nil {
		return nil, fmt.Errorf("initialize audio player: %w", err)
	}
	searchEngine := search.NewSearchEngine(cfg, storageDB)
	downloadManager := download.NewManager(cfg)
	syncManager := storage.NewSyncManager(apiClient, storageDB, cfg)
	musicService := services.NewMusicService(apiClient, storageDB, searchEngine)
	imageService := services.NewImageService(imageLoader)
	playSyncService := services.NewPlaySyncService(apiClient, storageDB, cfg, cfg.Debug)

	if !cfg.Debug {
		musicService.SetDebug(false)
		imageService.SetDebug(false)
	}

	return &Core{
		api:             apiClient,
		storage:         storageDB,
		player:          player,
		searchEngine:    searchEngine,
		downloadManager: downloadManager,
		syncManager:     syncManager,
		imageLoader:     imageLoader,
		musicService:    musicService,
		imageService:    imageService,
		playSyncService: playSyncService,
	}, nil
}

func (a *App) initUI() error {
	a.ui = &UIComponents{
		playerBar:        components.NewPlayerBar(a.core.player, a.core.storage, a.core.imageService, a.cfg.Debug),
		sidebar:          components.NewSidebar(a.cfg),
		authDialog:       components.NewAuthDialog(a.core.api),
		statusBar:        widget.NewLabel("Ready"),
		loadingIndicator: widget.NewProgressBarInfinite(),
	}

	a.ui.statusBar.Hide()

	a.ui.playerBar.SetConfig(a.cfg)
	a.ui.playerBar.SetParentWindow(a.window)

	a.ui.playerBar.OnPrefetchNext(func(s *types.Song) {
		go func() {
			if a.cfg.Debug {
				log.Printf("[APP] Prefetching next song: %s by %s", s.Name, getArtistNames(s.Authors))
			}
			if s != nil && s.File != "" {
				_ = a.core.downloadManager.DownloadSong(context.Background(), s)
			}
		}()
	})

	a.ui.loadingIndicator.Hide()
	a.ui.mainView = views.NewMainView(a.core.musicService, a.core.imageService, a.core.downloadManager, a.core.playSyncService, a.cfg)
	a.ui.mainView.SetParentWindow(a.window)

	a.createLayout()
	a.window.SetContent(a.mainContainer)
	a.window.SetOnClosed(a.Close)

	a.handleWindowResize(a.window.Canvas().Size())
	return nil
}

func (a *App) createLayout() {
	statusContainer := container.NewBorder(
		nil, nil,
		a.ui.statusBar, a.ui.loadingIndicator,
		nil,
	)

	bottomBar := container.NewVBox(
		a.ui.playerBar.Container(),
		statusContainer,
	)

	a.mainContainer = container.NewBorder(nil, bottomBar, a.ui.sidebar, nil, a.ui.mainView.Container())
}

func (a *App) startResizePolling() {
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				if a.window == nil || a.window.Canvas() == nil {
					continue
				}
				currentSize := a.window.Canvas().Size()
				if currentSize.Width != a.lastSize.Width || currentSize.Height != a.lastSize.Height {
					a.lastSize = currentSize
					fyne.Do(func() {
						a.handleWindowResize(currentSize)
					})
				}
			}
		}
	}()
}

func (a *App) handleWindowResize(size fyne.Size) {
	isCompact := size.Width < a.ui.sidebar.GetBreakpoint()
	if isCompact != a.state.compactMode {
		a.state.compactMode = isCompact
		a.ui.sidebar.SetCompactMode(isCompact)
		a.ui.mainView.SetCompactMode(isCompact)
		a.ui.playerBar.SetCompactMode(isCompact)
	}
}

func (a *App) setupEventHandlers() {
	a.ui.mainView.SettingsView.SetParentWindow(a.window)

	a.ui.sidebar.OnNavigate(func(view string) {
		a.ui.mainView.ShowView(view)
		a.updateStatus("Viewing " + view)
	})

	a.ui.sidebar.OnAuthRequested(func() {
		if a.state.isAuthenticated {
			a.logout()
		} else {
			a.ui.authDialog.Show(a.window)
		}
	})

	a.ui.authDialog.OnAuthenticated(func(token string) {
		a.handleAuthentication(token)
	})

	a.ui.mainView.OnSongSelected(func(song *types.Song, playlist []*types.Song) {
		a.playSong(song, playlist)
	})

	a.ui.mainView.OnAlbumSelected(func(album *types.Album) {
		a.ui.mainView.OpenAlbumDetail(album)
	})

	a.ui.mainView.OnArtistSelected(func(artist *types.Author) {
		a.ui.mainView.OpenAuthorDetail(artist)
	})

	a.ui.mainView.OnPlaylistSelected(func(playlist *types.Playlist) {
		a.updateStatus(fmt.Sprintf("Selected playlist: %s", playlist.Name))
		if len(playlist.Songs) > 0 {
			a.playPlaylist(playlist)
		}
	})

	a.ui.playerBar.OnNext(func() {
		a.updateStatus("Next song")
	})

	a.ui.playerBar.OnPrevious(func() {
		a.updateStatus("Previous song")
	})

	a.ui.playerBar.OnShuffle(func(enabled bool) {
		status := "disabled"
		if enabled {
			status = "enabled"
		}
		a.updateStatus(fmt.Sprintf("Shuffle %s", status))
	})

	a.ui.playerBar.OnRepeat(func(mode components.RepeatMode) {
		a.updateStatus(fmt.Sprintf("Repeat: %s", mode.String()))
	})

	a.core.downloadManager.OnProgress(func(progress *types.DownloadProgress) {
		switch progress.Status {
		case types.DownloadStatusCompleted:
			a.updateStatus(fmt.Sprintf("Downloaded: %s", progress.Filename))
		case types.DownloadStatusFailed:
			a.updateStatus(fmt.Sprintf("Download failed: %s", progress.Filename))
		}
	})

	a.setupSyncEventHandlers()
}

func (a *App) setupSyncEventHandlers() {
	a.core.syncManager.OnProgress(func(status string, current, total int) {
		if total > 0 && current < total {
			a.showLoading(true)
		}
	})

	a.core.syncManager.OnError(func(err error) {
		a.showLoading(false)
		a.state.syncInProgress = false
	})

	a.core.syncManager.OnComplete(func() {
		a.showLoading(false)
		a.state.syncInProgress = false
		go func() {
			time.Sleep(100 * time.Millisecond)
			fyne.Do(func() {
				a.ui.mainView.RefreshData()
				a.updateLibraryStats()
			})
		}()
	})
}

func (a *App) setupKeyboardShortcuts() {
	a.window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		switch key.Name {
		case fyne.KeySpace:
			if a.core.player.IsPlaying() {
				a.core.player.Pause()
			} else {
				a.core.player.Resume()
			}
		case fyne.KeyRight:
			a.core.player.Seek(a.core.player.GetPosition() + 10*time.Second)
		case fyne.KeyLeft:
			a.core.player.Seek(a.core.player.GetPosition() - 10*time.Second)
		case fyne.KeyF:
			a.window.SetFullScreen(!a.window.FullScreen())
		case fyne.KeyEscape:
			if a.window.FullScreen() {
				a.window.SetFullScreen(false)
			}
		}
	})

	a.window.Canvas().SetOnTypedRune(func(r rune) {
		if r == 's' || r == 'S' {
			a.focusSearch()
		}
	})
}

func (a *App) loadSavedState() {
	if a.cfg.API.Token != "" && !a.cfg.User.IsAnonymous {
		a.state.isAuthenticated = true
		a.ui.sidebar.SetAuthenticated(true, a.cfg.User.Username)
		a.startSync()
	} else {
		a.ui.sidebar.SetAuthenticated(false, "")
		a.initializeAnonymous()
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		fyne.Do(func() {
			a.loadInitialSongs()
		})
	}()
}

func (a *App) loadInitialSongs() {
	ctx := context.Background()
	songs, err := a.core.storage.GetSongs(ctx, 20, 0)
	if err != nil || len(songs) > 0 {
		return
	}
	go func() {
		resp, err := a.core.api.GetSongs(ctx, 1, "")
		if err == nil && len(resp.Results) > 0 {
			fyne.Do(func() {
				a.ui.mainView.RefreshData()
			})
		}
	}()
}

func (a *App) initializeAnonymous() {
	go func() {
		ctx := context.Background()
		anonID, err := a.core.api.EnsureAnonymousToken(ctx)
		if err != nil {
			return
		}
		a.cfg.User.AnonymousID = anonID
		a.cfg.Save()
	}()
}

func (a *App) handleAuthentication(token string) {
	a.state.isAuthenticated = true
	a.cfg.API.Token = token
	a.cfg.User.IsAnonymous = false
	a.core.api.SetToken(token)

	go func() {
		ctx := context.Background()
		user, err := a.core.api.GetCurrentUser(ctx)
		if err != nil {
			return
		}
		a.cfg.User.ID = user.ID
		a.cfg.User.Username = user.Username
		a.cfg.User.Email = user.Email
		if user.ImageCropped != nil {
			a.cfg.User.Image = *user.ImageCropped
		}
		a.cfg.Save()
		fyne.Do(func() {
			a.ui.sidebar.SetAuthenticated(true, user.Username)
		})
	}()
	a.startSync()
}

func (a *App) startBackgroundTasks() {
	if a.core.playSyncService != nil {
		a.core.playSyncService.Start()
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.updateLibraryStats()
			}
		}
	}()
}

func (a *App) playSong(song *types.Song, playlist []*types.Song) {
	if a.cfg.Debug {
		log.Printf("[APP] Playing song: %s", song.Name)
	}
	a.state.currentQueue = playlist
	a.state.currentIndex = -1
	for i, s := range a.state.currentQueue {
		if s.Slug == song.Slug {
			a.state.currentIndex = i
			break
		}
	}
	if a.state.currentIndex == -1 {
		a.state.currentQueue = []*types.Song{song}
		a.state.currentIndex = 0
	}
	a.ui.playerBar.SetQueue(a.state.currentQueue, a.state.currentIndex)

	if a.cfg.Download.AutoDownload && !song.Downloaded {
		go a.core.downloadManager.DownloadSong(context.Background(), song)
	}
}

func (a *App) playPlaylist(playlist *types.Playlist) {
	if len(playlist.Songs) == 0 {
		return
	}
	a.state.currentQueue = playlist.Songs
	a.state.currentIndex = 0
	a.ui.playerBar.SetQueue(a.state.currentQueue, a.state.currentIndex)
}

func (a *App) startSync() {
	if a.state.syncInProgress {
		return
	}
	a.state.syncInProgress = true
	a.showLoading(true)
	go a.core.syncManager.Start(a.ctx)
}

func (a *App) logout() {
	go a.core.api.Logout(context.Background())
	a.state.isAuthenticated = false
	a.cfg.API.Token = ""
	a.cfg.User.IsAnonymous = true
	a.cfg.User.Username = ""
	a.cfg.User.Email = ""
	a.cfg.User.Image = ""
	a.cfg.User.AnonymousID = ""
	a.cfg.Save()
	a.ui.sidebar.SetAuthenticated(false, "")
	a.core.syncManager.Stop()
	a.core.api.SetToken("")
	a.initializeAnonymous()
}

func (a *App) updateStatus(message string) {
	fyne.Do(func() {
		if a.ui.statusBar != nil {
			a.ui.statusBar.SetText(message)
			a.ui.statusBar.Resize(fyne.NewSize(0, a.ui.statusBar.MinSize().Height/2))
		}
	})
	time.AfterFunc(5*time.Second, func() {
		fyne.Do(func() {
			if a.ui.statusBar != nil && a.ui.statusBar.Text == message {
				a.ui.statusBar.SetText("Ready")
				a.ui.statusBar.Resize(fyne.NewSize(0, a.ui.statusBar.MinSize().Height/2))
			}
		})
	})
}

func (a *App) showLoading(show bool) {
	fyne.Do(func() {
		if a.ui.loadingIndicator != nil {
			if show {
				a.ui.loadingIndicator.Show()
			} else {
				a.ui.loadingIndicator.Hide()
			}
		}
	})
}

func (a *App) updateLibraryStats() {
	go func() {
		ctx := context.Background()
		songs, err := a.core.storage.GetSongs(ctx, 100000, 0)
		if err != nil {
			return
		}
		totalSeconds := 0
		for _, song := range songs {
			totalSeconds += song.Played * song.Length
		}
		hours := totalSeconds / 3600
		minutes := (totalSeconds % 3600) / 60
		timeListened := fmt.Sprintf("%dh %dm", hours, minutes)
		fyne.Do(func() {
			if a.ui.sidebar != nil {
				a.ui.sidebar.UpdateStats(len(songs), timeListened)
			}
		})
	}()
}

func (a *App) focusSearch() {
	a.ui.mainView.SearchInCurrentView("")
}

func (a *App) ShowAndRun() {
	a.window.ShowAndRun()
}

func (a *App) Close() {
	if a.core.playSyncService != nil {
		a.core.playSyncService.Stop()
	}
	if a.core.syncManager != nil {
		a.core.syncManager.Stop()
	}
	if a.core.player != nil {
		a.core.player.Close()
	}
	if a.core.storage != nil {
		a.core.storage.Close()
	}
}

func getArtistNames(authors []*types.Author) string {
	if len(authors) == 0 {
		return "Unknown Artist"
	}
	names := make([]string, len(authors))
	for i, author := range authors {
		if author != nil {
			names[i] = author.Name
		}
	}
	return strings.Join(names, ", ")
}
