package storage

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

// SyncManager handles synchronization between local storage and remote API
type SyncManager struct {
	api     *api.Client
	storage *Database
	cfg     *config.Config

	mu      sync.RWMutex
	running bool
	stop    chan struct{}
	ticker  *time.Ticker

	onProgress func(string, int, int)
	onError    func(error)
	onComplete func()

	debug bool
}

// SyncStats contains statistics about a synchronization operation
type SyncStats struct {
	SongsTotal      int
	SongsSynced     int
	AlbumsTotal     int
	AlbumsSynced    int
	AuthorsTotal    int
	AuthorsSynced   int
	PlaylistsTotal  int
	PlaylistsSynced int
	StartTime       time.Time
	EndTime         time.Time
	LastSync        time.Time
	Errors          []string
}

// NewSyncManager creates a new sync manager with the given dependencies
func NewSyncManager(api *api.Client, storage *Database, cfg *config.Config) *SyncManager {
	return &SyncManager{
		api:     api,
		storage: storage,
		cfg:     cfg,
		stop:    make(chan struct{}),
		debug:   cfg.Debug,
	}
}

func (sm *SyncManager) debugLog(format string, args ...interface{}) {
	if sm.debug {
		log.Printf("[SYNC] "+format, args...)
	}
}

func extractPageFromURL(urlStr string) int {
	if urlStr == "" {
		return 0
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return 0
	}

	pageStr := u.Query().Get("page")
	if pageStr == "" {
		return 0
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil {
		return 0
	}

	return page
}

// Start begins the synchronization process with periodic updates
func (sm *SyncManager) Start(ctx context.Context) {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = true
	sm.mu.Unlock()

	sm.debugLog("Sync manager starting with interval: %v", time.Duration(sm.cfg.Storage.SyncInterval)*time.Second)

	sm.ticker = time.NewTicker(time.Duration(sm.cfg.Storage.SyncInterval) * time.Second)

	go func() {
		defer func() {
			sm.mu.Lock()
			sm.running = false
			sm.mu.Unlock()
			sm.debugLog("Sync manager stopped")
		}()

		for {
			select {
			case <-ctx.Done():
				sm.debugLog("Sync manager stopping due to context cancellation")
				return
			case <-sm.stop:
				sm.debugLog("Sync manager stopping due to stop signal")
				return
			}
		}
	}()
}

// Stop halts the synchronization process
func (sm *SyncManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.running {
		return
	}

	sm.debugLog("Stopping sync manager...")
	close(sm.stop)
	if sm.ticker != nil {
		sm.ticker.Stop()
	}
}

// FullSync performs a complete synchronization of all data
func (sm *SyncManager) FullSync(ctx context.Context) error {
	if sm.api.IsAnonymous() {
		sm.debugLog("Skipping sync - running in anonymous mode")
		return nil
	}

	stats := &SyncStats{
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	sm.debugLog("=== FULL SYNC STARTED ===")

	if sm.onProgress != nil {
		sm.onProgress("Starting sync...", 0, 100)
	}

	steps := []struct {
		name string
		fn   func(context.Context, *SyncStats) error
	}{
		{"songs", sm.syncSongs},
		{"albums", sm.syncAlbums},
		{"authors", sm.syncAuthors},
		{"playlists", sm.syncPlaylists},
		{"play_history", sm.syncPlayHistory},
		{"user_preferences", sm.syncUserPreferences},
	}

	for i, step := range steps {
		sm.debugLog("Syncing %s... (%d/%d)", step.name, i+1, len(steps))

		if sm.onProgress != nil {
			progress := int(float64(i) / float64(len(steps)) * 100)
			sm.onProgress(fmt.Sprintf("Syncing %s...", step.name), progress, 100)
		}

		if err := step.fn(ctx, stats); err != nil {
			errorMsg := fmt.Sprintf("Failed to sync %s: %v", step.name, err)
			sm.debugLog(errorMsg)
			stats.Errors = append(stats.Errors, errorMsg)
		}
	}

	stats.EndTime = time.Now()
	stats.LastSync = time.Now()

	duration := stats.EndTime.Sub(stats.StartTime)
	sm.debugLog("=== SYNC COMPLETED ===")
	sm.debugLog("Duration: %v", duration)
	sm.debugLog("Songs: %d/%d", stats.SongsSynced, stats.SongsTotal)
	sm.debugLog("Albums: %d/%d", stats.AlbumsSynced, stats.AlbumsTotal)
	sm.debugLog("Authors: %d/%d", stats.AuthorsSynced, stats.AuthorsTotal)
	sm.debugLog("Playlists: %d/%d", stats.PlaylistsSynced, stats.PlaylistsTotal)
	sm.debugLog("Errors: %d", len(stats.Errors))

	if sm.onProgress != nil {
		sm.onProgress("Sync completed", 100, 100)
	}

	if sm.onComplete != nil {
		sm.onComplete()
	}

	if len(stats.Errors) > 0 {
		return fmt.Errorf("sync completed with %d errors", len(stats.Errors))
	}

	return nil
}

func (sm *SyncManager) syncSongs(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing Songs ---")

	page := 1
	pagesFetched := 0
	limit := sm.cfg.Storage.MaxSyncPages
	totalSynced := 0

	sm.debugLog("Starting songs sync with page limit: %d", limit)

	for {
		if limit > 0 && pagesFetched >= limit {
			sm.debugLog("Songs page limit reached (%d), stopping.", limit)
			break
		}

		sm.debugLog("Fetching songs page %d...", page)
		resp, err := sm.api.GetSongs(ctx, page, "")
		if err != nil {
			return fmt.Errorf("get songs page %d: %w", page, err)
		}
		if len(resp.Results) == 0 {
			sm.debugLog("No more songs to sync")
			break
		}

		for i, song := range resp.Results {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			song.LastSync = time.Now()
			if err := sm.storage.SaveSong(ctx, song); err != nil {
				sm.debugLog("Failed to save song %s: %v", song.Slug, err)
				stats.Errors = append(stats.Errors, fmt.Sprintf("save song %s: %v", song.Name, err))
				continue
			}
			totalSynced++
			if (i+1)%50 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
		}

		pagesFetched++
		if resp.Next == nil {
			break
		}
		nextPage := extractPageFromURL(*resp.Next)
		if nextPage <= page {
			break
		}
		page = nextPage
		time.Sleep(100 * time.Millisecond)
	}

	stats.SongsSynced = totalSynced
	stats.SongsTotal = totalSynced
	sm.debugLog("Songs sync completed: %d synced (pages: %d)", totalSynced, pagesFetched)
	return nil
}

func (sm *SyncManager) syncAlbums(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing Albums ---")

	page := 1
	pagesFetched := 0
	limit := sm.cfg.Storage.MaxSyncPages
	totalSynced := 0

	for {
		if limit > 0 && pagesFetched >= limit {
			sm.debugLog("Albums page limit reached (%d), stopping.", limit)
			break
		}
		sm.debugLog("Fetching albums page %d...", page)

		resp, err := sm.api.GetAlbums(ctx, page, "")
		if err != nil {
			return fmt.Errorf("get albums page %d: %w", page, err)
		}

		if len(resp.Results) == 0 {
			sm.debugLog("No more albums to sync")
			break
		}

		sm.debugLog("Processing %d albums from page %d", len(resp.Results), page)

		for i, album := range resp.Results {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			album.LastSync = time.Now()
			if err := sm.storage.SaveAlbum(ctx, album); err != nil {
				sm.debugLog("Failed to save album %s: %v", album.Slug, err)
				stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to save album %s: %v", album.Name, err))
				continue
			}

			totalSynced++
			sm.debugLog("Saved album %d/%d: %s", i+1, len(resp.Results), album.Name)
		}

		if resp.Next == nil {
			break
		}

		nextPage := extractPageFromURL(*resp.Next)
		if nextPage <= page {
			break
		}
		page = nextPage

		time.Sleep(100 * time.Millisecond)
		pagesFetched++
	}

	stats.AlbumsSynced = totalSynced
	stats.AlbumsTotal = totalSynced
	sm.debugLog("Albums sync completed: %d albums synced", totalSynced)

	return nil
}

func (sm *SyncManager) syncAuthors(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing Authors ---")

	page := 1
	pagesFetched := 0
	limit := sm.cfg.Storage.MaxSyncPages
	totalSynced := 0

	for {
		if limit > 0 && pagesFetched >= limit {
			sm.debugLog("Authors page limit reached (%d), stopping.", limit)
			break
		}
		sm.debugLog("Fetching authors page %d...", page)

		resp, err := sm.api.GetAuthors(ctx, page, "")
		if err != nil {
			return fmt.Errorf("get authors page %d: %w", page, err)
		}

		if len(resp.Results) == 0 {
			sm.debugLog("No more authors to sync")
			break
		}

		sm.debugLog("Processing %d authors from page %d", len(resp.Results), page)

		for i, author := range resp.Results {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			author.LastSync = time.Now()
			if err := sm.storage.SaveAuthor(ctx, author); err != nil {
				sm.debugLog("Failed to save author %s: %v", author.Slug, err)
				stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to save author %s: %v", author.Name, err))
				continue
			}

			totalSynced++
			sm.debugLog("Saved author %d/%d: %s", i+1, len(resp.Results), author.Name)
		}

		if resp.Next == nil {
			break
		}

		nextPage := extractPageFromURL(*resp.Next)
		if nextPage <= page {
			break
		}
		page = nextPage

		time.Sleep(100 * time.Millisecond)
		pagesFetched++
	}

	stats.AuthorsSynced = totalSynced
	stats.AuthorsTotal = totalSynced
	sm.debugLog("Authors sync completed: %d authors synced", totalSynced)

	return nil
}

func (sm *SyncManager) syncPlaylists(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing Playlists ---")

	playlists, err := sm.api.GetPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("get playlists: %w", err)
	}

	sm.debugLog("Found %d playlists to sync", len(playlists))
	totalSynced := 0

	for i, playlist := range playlists {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if playlist.LocalOnly {
			sm.debugLog("Skipping local-only playlist: %s", playlist.Name)
			continue
		}

		sm.debugLog("Fetching full playlist data for: %s", playlist.Name)

		fullPlaylist, err := sm.api.GetPlaylist(ctx, playlist.Slug)
		if err != nil {
			sm.debugLog("Failed to get playlist details %s: %v", playlist.Slug, err)
			stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to get playlist %s: %v", playlist.Name, err))
			continue
		}

		fullPlaylist.LastSync = time.Now()
		if err := sm.storage.SavePlaylist(ctx, fullPlaylist); err != nil {
			sm.debugLog("Failed to save playlist %s: %v", playlist.Slug, err)
			stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to save playlist %s: %v", playlist.Name, err))
			continue
		}

		totalSynced++
		sm.debugLog("Saved playlist %d/%d: %s (%d songs)", i+1, len(playlists), fullPlaylist.Name, len(fullPlaylist.Songs))

		time.Sleep(200 * time.Millisecond)
	}

	stats.PlaylistsSynced = totalSynced
	stats.PlaylistsTotal = len(playlists)
	sm.debugLog("Playlists sync completed: %d/%d playlists synced", totalSynced, len(playlists))

	return nil
}

func (sm *SyncManager) syncPlayHistory(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing Play History ---")

	query := `
		SELECT song_slug, user_id, played_at 
		FROM play_history 
		WHERE synced = false 
		ORDER BY played_at ASC 
		LIMIT 100
	`

	rows, err := sm.storage.GetDB().QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query unsynced play history: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			sm.debugLog("Failed to close rows: %v", closeErr)
		}
	}()

	var toSync []types.PlayHistory
	for rows.Next() {
		var history types.PlayHistory
		if err := rows.Scan(&history.SongSlug, &history.UserID, &history.PlayedAt); err != nil {
			sm.debugLog("Failed to scan play history: %v", err)
			continue
		}
		toSync = append(toSync, history)
	}

	sm.debugLog("Found %d play history entries to sync", len(toSync))

	synced := 0
	for _, history := range toSync {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		userID := ""
		if history.UserID != nil {
			userID = *history.UserID
		}

		if err := sm.api.ListenSong(ctx, history.SongSlug, userID); err != nil {
			sm.debugLog("Failed to sync play count for %s: %v", history.SongSlug, err)
			continue
		}

		if _, err := sm.storage.GetDB().ExecContext(ctx,
			"UPDATE play_history SET synced = true WHERE song_slug = ? AND played_at = ?",
			history.SongSlug, history.PlayedAt); err != nil {
			sm.debugLog("Failed to mark play history as synced: %v", err)
			continue
		}

		synced++
		sm.debugLog("Synced play history %d/%d", synced, len(toSync))

		time.Sleep(50 * time.Millisecond)
	}

	sm.debugLog("Play history sync completed: %d/%d entries synced", synced, len(toSync))
	return nil
}

func (sm *SyncManager) syncUserPreferences(ctx context.Context, stats *SyncStats) error {
	sm.debugLog("--- Syncing User Preferences ---")

	if sm.api.IsAnonymous() {
		sm.debugLog("Skipping user preferences sync - anonymous mode")
		return nil
	}

	user, err := sm.api.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	sm.cfg.User.ID = user.ID
	sm.cfg.User.Username = user.Username
	sm.cfg.User.Email = user.Email
	if user.ImageCropped != nil {
		sm.cfg.User.Image = *user.ImageCropped
	}

	if err := sm.cfg.Save(); err != nil {
		sm.debugLog("Failed to save user preferences: %v", err)
		return fmt.Errorf("save user preferences: %w", err)
	}

	sm.debugLog("User preferences synced for: %s", user.Username)
	return nil
}

// ForceSyncSongs performs a forced synchronization of songs only
func (sm *SyncManager) ForceSyncSongs(ctx context.Context) error {
	sm.debugLog("Force syncing songs...")
	stats := &SyncStats{StartTime: time.Now(), Errors: make([]string, 0)}
	return sm.syncSongs(ctx, stats)
}

// ForceSyncAlbums performs a forced synchronization of albums only
func (sm *SyncManager) ForceSyncAlbums(ctx context.Context) error {
	sm.debugLog("Force syncing albums...")
	stats := &SyncStats{StartTime: time.Now(), Errors: make([]string, 0)}
	return sm.syncAlbums(ctx, stats)
}

// ForceSyncAuthors performs a forced synchronization of authors only
func (sm *SyncManager) ForceSyncAuthors(ctx context.Context) error {
	sm.debugLog("Force syncing authors...")
	stats := &SyncStats{StartTime: time.Now(), Errors: make([]string, 0)}
	return sm.syncAuthors(ctx, stats)
}

// ForceSyncPlaylists performs a forced synchronization of playlists only
func (sm *SyncManager) ForceSyncPlaylists(ctx context.Context) error {
	sm.debugLog("Force syncing playlists...")
	stats := &SyncStats{StartTime: time.Now(), Errors: make([]string, 0)}
	return sm.syncPlaylists(ctx, stats)
}

// IsRunning returns true if the sync manager is currently running
func (sm *SyncManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}

// SetInterval updates the sync interval
func (sm *SyncManager) SetInterval(interval time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.cfg.Storage.SyncInterval = int(interval.Seconds())

	if sm.ticker != nil {
		sm.ticker.Stop()
		sm.ticker = time.NewTicker(interval)
	}

	sm.debugLog("Sync interval updated to: %v", interval)
}

// OnProgress sets the progress callback
func (sm *SyncManager) OnProgress(callback func(string, int, int)) {
	sm.onProgress = callback
}

// OnError sets the error callback
func (sm *SyncManager) OnError(callback func(error)) {
	sm.onError = callback
}

// OnComplete sets the completion callback
func (sm *SyncManager) OnComplete(callback func()) {
	sm.onComplete = callback
}

// GetLastSyncTime returns the timestamp of the last successful sync
func (sm *SyncManager) GetLastSyncTime() time.Time {
	query := "SELECT MAX(last_sync) FROM (SELECT last_sync FROM songs UNION SELECT last_sync FROM albums UNION SELECT last_sync FROM authors UNION SELECT last_sync FROM playlists)"

	var lastSync time.Time
	row := sm.storage.GetDB().QueryRow(query)
	if err := row.Scan(&lastSync); err != nil {
		sm.debugLog("Failed to get last sync time: %v", err)
		return time.Time{}
	}

	return lastSync
}

// GetSyncStats returns current synchronization statistics
func (sm *SyncManager) GetSyncStats() *SyncStats {
	stats := &SyncStats{
		LastSync: sm.GetLastSyncTime(),
		Errors:   make([]string, 0),
	}

	ctx := context.Background()

	if songs, err := sm.storage.GetSongs(ctx, 10000, 0); err == nil {
		stats.SongsTotal = len(songs)
		stats.SongsSynced = len(songs)
	}

	if albums, err := sm.storage.GetAlbums(ctx, 10000, 0); err == nil {
		stats.AlbumsTotal = len(albums)
		stats.AlbumsSynced = len(albums)
	}

	if authors, err := sm.storage.GetAuthors(ctx, 10000, 0); err == nil {
		stats.AuthorsTotal = len(authors)
		stats.AuthorsSynced = len(authors)
	}

	if playlists, err := sm.storage.GetPlaylists(ctx); err == nil {
		stats.PlaylistsTotal = len(playlists)
		stats.PlaylistsSynced = len(playlists)
	}

	return stats
}
