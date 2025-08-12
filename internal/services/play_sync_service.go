package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Alexander-D-Karpov/amp/internal/api"
	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/storage"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type PlaySyncService struct {
	api     *api.Client
	storage *storage.Database
	cfg     *config.Config
	debug   bool
	ticker  *time.Ticker
	stopCh  chan struct{}
}

func NewPlaySyncService(api *api.Client, storage *storage.Database, cfg *config.Config, debug bool) *PlaySyncService {
	return &PlaySyncService{
		api:     api,
		storage: storage,
		cfg:     cfg,
		debug:   debug,
		stopCh:  make(chan struct{}),
	}
}

func (p *PlaySyncService) Start() {
	if p.ticker != nil {
		return
	}

	p.ticker = time.NewTicker(5 * time.Minute)

	go func() {
		time.Sleep(30 * time.Second)
		p.syncPlayHistory()

		for {
			select {
			case <-p.ticker.C:
				p.syncPlayHistory()
			case <-p.stopCh:
				return
			}
		}
	}()

	if p.debug {
		log.Printf("[PLAY_SYNC] Play history sync service started")
	}
}

func (p *PlaySyncService) Stop() {
	if p.ticker != nil {
		p.ticker.Stop()
		p.ticker = nil
	}

	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}

	if p.debug {
		log.Printf("[PLAY_SYNC] Play history sync service stopped")
	}
}

func (p *PlaySyncService) RecordAndSendListen(ctx context.Context, song *types.Song) error {
	if song == nil {
		return fmt.Errorf("song is nil")
	}

	userID := p.getUserID()

	if p.debug {
		log.Printf("[PLAY_SYNC] Recording listen for song: %s (user: %s, anonymous: %v)",
			song.Name, userID, p.api.IsAnonymous())
	}

	if err := p.sendListenImmediately(ctx, song.Slug, userID); err != nil {
		if p.debug {
			log.Printf("[PLAY_SYNC] Failed to send immediate listen for %s: %v", song.Name, err)
		}

		if err := p.recordLocalPlay(ctx, song.Slug, userID); err != nil {
			log.Printf("[PLAY_SYNC] Failed to record local play for %s: %v", song.Name, err)
			return err
		}
		return nil
	}

	if p.debug {
		log.Printf("[PLAY_SYNC] Successfully sent immediate listen for song: %s", song.Name)
	}

	if err := p.recordLocalPlayAsSynced(ctx, song.Slug, userID); err != nil {
		if p.debug {
			log.Printf("[PLAY_SYNC] Failed to record synced play locally for %s: %v", song.Name, err)
		}
	}

	return nil
}

func (p *PlaySyncService) sendListenImmediately(ctx context.Context, songSlug, userID string) error {
	return p.api.ListenSong(ctx, songSlug, userID)
}

func (p *PlaySyncService) recordLocalPlay(ctx context.Context, songSlug, userID string) error {
	query := `
		INSERT INTO play_history (song_slug, user_id, played_at, synced) 
		VALUES (?, ?, ?, false)
	`

	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	_, err := p.storage.GetDB().ExecContext(ctx, query, songSlug, userIDPtr, time.Now())
	return err
}

func (p *PlaySyncService) recordLocalPlayAsSynced(ctx context.Context, songSlug, userID string) error {
	query := `
		INSERT INTO play_history (song_slug, user_id, played_at, synced) 
		VALUES (?, ?, ?, true)
	`

	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	_, err := p.storage.GetDB().ExecContext(ctx, query, songSlug, userIDPtr, time.Now())
	return err
}

func (p *PlaySyncService) getUserID() string {
	if p.cfg.User.IsAnonymous {
		if p.cfg.User.AnonymousID != "" {
			return p.cfg.User.AnonymousID
		}
		if p.api.GetToken() != "" {
			return p.api.GetToken()
		}
		return ""
	}

	if p.cfg.User.ID != 0 {
		return fmt.Sprintf("%d", p.cfg.User.ID)
	}

	return ""
}

func (p *PlaySyncService) syncPlayHistory() {
	ctx := context.Background()

	query := `
		SELECT song_slug, user_id, played_at 
		FROM play_history 
		WHERE synced = false 
		ORDER BY played_at ASC 
		LIMIT 50
	`

	rows, err := p.storage.GetDB().QueryContext(ctx, query)
	if err != nil {
		if p.debug {
			log.Printf("[PLAY_SYNC] Failed to query unsynced play history: %v", err)
		}
		return
	}
	defer rows.Close()

	var toSync []struct {
		songSlug string
		userID   *string
		playedAt time.Time
	}

	for rows.Next() {
		var entry struct {
			songSlug string
			userID   *string
			playedAt time.Time
		}

		if err := rows.Scan(&entry.songSlug, &entry.userID, &entry.playedAt); err != nil {
			if p.debug {
				log.Printf("[PLAY_SYNC] Failed to scan play history: %v", err)
			}
			continue
		}
		toSync = append(toSync, entry)
	}

	if len(toSync) == 0 {
		if p.debug {
			log.Printf("[PLAY_SYNC] No play history to sync")
		}
		return
	}

	if p.debug {
		log.Printf("[PLAY_SYNC] Syncing %d play history entries", len(toSync))
	}

	synced := 0
	for _, history := range toSync {
		userID := ""
		if history.userID != nil {
			userID = *history.userID
		}

		if err := p.api.ListenSong(ctx, history.songSlug, userID); err != nil {
			if p.debug {
				log.Printf("[PLAY_SYNC] Failed to sync play count for %s: %v", history.songSlug, err)
			}
			continue
		}

		if _, err := p.storage.GetDB().ExecContext(ctx,
			"UPDATE play_history SET synced = true WHERE song_slug = ? AND played_at = ?",
			history.songSlug, history.playedAt); err != nil {
			if p.debug {
				log.Printf("[PLAY_SYNC] Failed to mark play history as synced: %v", err)
			}
			continue
		}

		synced++
		time.Sleep(100 * time.Millisecond)
	}

	if p.debug {
		log.Printf("[PLAY_SYNC] Successfully synced %d/%d play history entries", synced, len(toSync))
	}
}

func (p *PlaySyncService) ForceSyncNow() {
	go p.syncPlayHistory()
}
