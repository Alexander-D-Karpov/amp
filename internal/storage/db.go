package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Database struct {
	db       *sql.DB
	cacheDir string
	mu       sync.RWMutex
	closed   bool
	debug    bool
}

func (d *Database) GetDB() *sql.DB {
	return d.db
}

func NewDatabase(cfg *config.Config) (*Database, error) {
	dbDir := filepath.Dir(cfg.Storage.DatabasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	cacheDir := cfg.Storage.CacheDir
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	db, err := openDatabase(cfg.Storage.DatabasePath, cfg.Storage.EnableWAL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	storage := &Database{
		db:       db,
		cacheDir: cacheDir,
		debug:    cfg.Debug,
	}

	if err := storage.runMigrations(); err != nil {
		if closeErr := storage.Close(); closeErr != nil {
			log.Printf("Failed to close database after migration error: %v", closeErr)
		}
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return storage, nil
}

func openDatabase(dbPath string, enableWAL bool) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Printf("Creating new database at %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=memory",
		"PRAGMA cache_size=-64000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=30000",
		"PRAGMA mmap_size=268435456",
	}

	if enableWAL {
		pragmas = append(pragmas, "PRAGMA journal_mode=WAL")
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				log.Printf("Failed to close database after pragma error: %v", closeErr)
			}
			return nil, fmt.Errorf("execute pragma %s: %w", pragma, err)
		}
	}

	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Failed to close database after ping error: %v", closeErr)
		}
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func (d *Database) debugLog(operation string, err error, duration time.Duration) {
	if !d.debug || err == nil {
		return
	}

	log.Printf("[DB] %s failed in %v: %v", operation, duration, err)
}

func (d *Database) checkClosed() error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return fmt.Errorf("database is closed")
	}
	return nil
}

func (d *Database) GetSongs(ctx context.Context, limit, offset int) ([]*types.Song, error) {
	start := time.Now()
	defer func() { d.debugLog("GetSongs", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT s.slug, s.name, s.file, s.image, s.image_cropped, s.length, 
		       s.played, s.link, s.liked, s.volume, s.album_slug, s.local_path, 
		       s.downloaded, s.last_sync, s.created_at, s.updated_at,
		       COALESCE(a.slug, '') as album_slug_ref, 
		       COALESCE(a.name, '') as album_name, 
		       COALESCE(a.image, '') as album_image, 
		       COALESCE(a.image_cropped, '') as album_image_cropped, 
		       COALESCE(a.link, '') as album_link
		FROM songs s
		LEFT JOIN albums a ON s.album_slug = a.slug
		ORDER BY s.created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		d.debugLog("GetSongs", err, time.Since(start))
		return nil, fmt.Errorf("query songs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var songs []*types.Song
	for rows.Next() {
		song, err := d.scanSong(rows)
		if err != nil {
			d.debugLog("GetSongs", err, time.Since(start))
			return nil, fmt.Errorf("scan song: %w", err)
		}
		songs = append(songs, song)
	}

	if err := rows.Err(); err != nil {
		d.debugLog("GetSongs", err, time.Since(start))
		return nil, fmt.Errorf("rows error: %w", err)
	}

	if err := d.loadSongAuthors(ctx, songs); err != nil {
		d.debugLog("GetSongs", err, time.Since(start))
		return nil, fmt.Errorf("load song authors: %w", err)
	}

	return songs, nil
}

func (d *Database) GetSong(ctx context.Context, slug string) (*types.Song, error) {
	start := time.Now()
	defer func() { d.debugLog("GetSong", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT s.slug, s.name, s.file, s.image, s.image_cropped, s.length, 
		       s.played, s.link, s.liked, s.volume, s.album_slug, s.local_path, 
		       s.downloaded, s.last_sync, s.created_at, s.updated_at,
		       COALESCE(a.slug, '') as album_slug_ref, 
		       COALESCE(a.name, '') as album_name, 
		       COALESCE(a.image, '') as album_image, 
		       COALESCE(a.image_cropped, '') as album_image_cropped, 
		       COALESCE(a.link, '') as album_link
		FROM songs s
		LEFT JOIN albums a ON s.album_slug = a.slug
		WHERE s.slug = ?
	`

	row := d.db.QueryRowContext(ctx, query, slug)
	song, err := d.scanSong(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		d.debugLog("GetSong", err, time.Since(start))
		return nil, fmt.Errorf("scan song: %w", err)
	}

	if err := d.loadSongAuthors(ctx, []*types.Song{song}); err != nil {
		d.debugLog("GetSong", err, time.Since(start))
		return nil, fmt.Errorf("load song authors: %w", err)
	}

	return song, nil
}

func (d *Database) SaveSong(ctx context.Context, song *types.Song) error {
	start := time.Now()
	err := fmt.Errorf("save song: %w", nil)
	defer func(err *error) {
		if *err != nil {
			d.debugLog("SaveSong", *err, time.Since(start))
		}
	}(&err)

	if err := d.checkClosed(); err != nil {
		return err
	}

	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			log.Printf("Failed to rollback transaction: %v", rollbackErr)
		}
	}()

	if song.Album != nil {
		if err := d.saveAlbumInTx(ctx, tx, song.Album); err != nil {
			return fmt.Errorf("save album: %w", err)
		}
		song.AlbumSlug = song.Album.Slug
	}

	for _, author := range song.Authors {
		if err := d.saveAuthorInTx(ctx, tx, author); err != nil {
			return fmt.Errorf("save author: %w", err)
		}
	}

	volumeJSON := "[]"
	if len(song.Volume) > 0 {
		if data, err := json.Marshal(song.Volume); err == nil {
			volumeJSON = string(data)
		}
	}

	query := `
		INSERT OR REPLACE INTO songs (
			slug, name, file, image, image_cropped, length, played, link, 
			liked, volume, album_slug, local_path, downloaded, last_sync, 
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if song.CreatedAt.IsZero() {
		song.CreatedAt = now
	}
	song.UpdatedAt = now

	_, err = tx.ExecContext(ctx, query,
		song.Slug, song.Name, song.File, song.Image, song.ImageCropped,
		song.Length, song.Played, song.Link, song.Liked, volumeJSON,
		song.AlbumSlug, song.LocalPath, song.Downloaded, song.LastSync,
		song.CreatedAt, song.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert song: %w", err)
	}

	if err := d.saveSongAuthors(ctx, tx, song); err != nil {
		return fmt.Errorf("save song authors: %w", err)
	}

	return tx.Commit()
}

func (d *Database) DeleteSong(ctx context.Context, slug string) error {
	start := time.Now()
	defer func() { d.debugLog("DeleteSong", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return err
	}

	_, err := d.db.ExecContext(ctx, "DELETE FROM songs WHERE slug = ?", slug)
	return err
}

func (d *Database) SearchSongs(ctx context.Context, query string, limit int) ([]*types.Song, error) {
	start := time.Now()
	defer func() { d.debugLog("SearchSongs", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	searchQuery := `
		SELECT s.slug, s.name, s.file, s.image, s.image_cropped, s.length, 
		       s.played, s.link, s.liked, s.volume, s.album_slug, s.local_path, 
		       s.downloaded, s.last_sync, s.created_at, s.updated_at,
		       COALESCE(a.slug, '') as album_slug_ref, 
		       COALESCE(a.name, '') as album_name, 
		       COALESCE(a.image, '') as album_image, 
		       COALESCE(a.image_cropped, '') as album_image_cropped, 
		       COALESCE(a.link, '') as album_link
		FROM songs s
		LEFT JOIN albums a ON s.album_slug = a.slug
		WHERE s.name LIKE ? OR EXISTS (
			SELECT 1 FROM song_authors sa 
			JOIN authors au ON sa.author_slug = au.slug 
			WHERE sa.song_slug = s.slug AND au.name LIKE ?
		)
		ORDER BY s.created_at DESC
		LIMIT ?
	`

	searchPattern := "%" + query + "%"
	rows, err := d.db.QueryContext(ctx, searchQuery, searchPattern, searchPattern, limit)
	if err != nil {
		d.debugLog("SearchSongs", err, time.Since(start))
		return nil, fmt.Errorf("search songs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var songs []*types.Song
	for rows.Next() {
		song, err := d.scanSong(rows)
		if err != nil {
			d.debugLog("SearchSongs", err, time.Since(start))
			return nil, fmt.Errorf("scan song: %w", err)
		}
		songs = append(songs, song)
	}

	if err := d.loadSongAuthors(ctx, songs); err != nil {
		d.debugLog("SearchSongs", err, time.Since(start))
		return nil, fmt.Errorf("load song authors: %w", err)
	}

	return songs, nil
}

func (d *Database) GetAlbums(ctx context.Context, limit, offset int) ([]*types.Album, error) {
	start := time.Now()
	defer func() { d.debugLog("GetAlbums", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		FROM albums
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		d.debugLog("GetAlbums", err, time.Since(start))
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var albums []*types.Album
	for rows.Next() {
		album, err := d.scanAlbum(rows)
		if err != nil {
			d.debugLog("GetAlbums", err, time.Since(start))
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, album)
	}

	return albums, nil
}

func (d *Database) GetAlbum(ctx context.Context, slug string) (*types.Album, error) {
	start := time.Now()
	defer func() { d.debugLog("GetAlbum", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		FROM albums
		WHERE slug = ?
	`

	row := d.db.QueryRowContext(ctx, query, slug)
	album, err := d.scanAlbum(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		d.debugLog("GetAlbum", err, time.Since(start))
		return nil, fmt.Errorf("scan album: %w", err)
	}

	return album, nil
}

func (d *Database) SaveAlbum(ctx context.Context, album *types.Album) error {
	start := time.Now()
	defer func() { d.debugLog("SaveAlbum", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return err
	}

	query := `
		INSERT OR REPLACE INTO albums (
			slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if album.CreatedAt.IsZero() {
		album.CreatedAt = now
	}
	album.UpdatedAt = now

	_, err := d.db.ExecContext(ctx, query,
		album.Slug, album.Name, album.Image, album.ImageCropped,
		album.Link, album.LastSync, album.CreatedAt, album.UpdatedAt,
	)
	return err
}

func (d *Database) saveAlbumInTx(ctx context.Context, tx *sql.Tx, album *types.Album) error {
	query := `
		INSERT OR REPLACE INTO albums (
			slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if album.CreatedAt.IsZero() {
		album.CreatedAt = now
	}
	album.UpdatedAt = now

	_, err := tx.ExecContext(ctx, query,
		album.Slug, album.Name, album.Image, album.ImageCropped,
		album.Link, album.LastSync, album.CreatedAt, album.UpdatedAt,
	)
	return err
}

func (d *Database) GetAuthors(ctx context.Context, limit, offset int) ([]*types.Author, error) {
	start := time.Now()
	defer func() { d.debugLog("GetAuthors", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		FROM authors
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		d.debugLog("GetAuthors", err, time.Since(start))
		return nil, fmt.Errorf("query authors: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var authors []*types.Author
	for rows.Next() {
		author, err := d.scanAuthor(rows)
		if err != nil {
			d.debugLog("GetAuthors", err, time.Since(start))
			return nil, fmt.Errorf("scan author: %w", err)
		}
		authors = append(authors, author)
	}

	return authors, nil
}

func (d *Database) GetAuthor(ctx context.Context, slug string) (*types.Author, error) {
	start := time.Now()
	defer func() { d.debugLog("GetAuthor", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		FROM authors
		WHERE slug = ?
	`

	row := d.db.QueryRowContext(ctx, query, slug)
	author, err := d.scanAuthor(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		d.debugLog("GetAuthor", err, time.Since(start))
		return nil, fmt.Errorf("scan author: %w", err)
	}

	return author, nil
}

func (d *Database) SaveAuthor(ctx context.Context, author *types.Author) error {
	start := time.Now()
	defer func() { d.debugLog("SaveAuthor", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return err
	}

	query := `
		INSERT OR REPLACE INTO authors (
			slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if author.CreatedAt.IsZero() {
		author.CreatedAt = now
	}
	author.UpdatedAt = now

	_, err := d.db.ExecContext(ctx, query,
		author.Slug, author.Name, author.Image, author.ImageCropped,
		author.Link, author.LastSync, author.CreatedAt, author.UpdatedAt,
	)
	return err
}

func (d *Database) saveAuthorInTx(ctx context.Context, tx *sql.Tx, author *types.Author) error {
	query := `
		INSERT OR REPLACE INTO authors (
			slug, name, image, image_cropped, link, last_sync, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if author.CreatedAt.IsZero() {
		author.CreatedAt = now
	}
	author.UpdatedAt = now

	_, err := tx.ExecContext(ctx, query,
		author.Slug, author.Name, author.Image, author.ImageCropped,
		author.Link, author.LastSync, author.CreatedAt, author.UpdatedAt,
	)
	return err
}

func (d *Database) GetPlaylists(ctx context.Context) ([]*types.Playlist, error) {
	start := time.Now()
	defer func() { d.debugLog("GetPlaylists", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, private, length, local_only, last_sync, created_at, updated_at
		FROM playlists
		ORDER BY created_at DESC
	`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		d.debugLog("GetPlaylists", err, time.Since(start))
		return nil, fmt.Errorf("query playlists: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var playlists []*types.Playlist
	for rows.Next() {
		playlist, err := d.scanPlaylist(rows)
		if err != nil {
			d.debugLog("GetPlaylists", err, time.Since(start))
			return nil, fmt.Errorf("scan playlist: %w", err)
		}
		playlists = append(playlists, playlist)
	}

	return playlists, nil
}

func (d *Database) GetPlaylist(ctx context.Context, slug string) (*types.Playlist, error) {
	start := time.Now()
	defer func() { d.debugLog("GetPlaylist", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return nil, err
	}

	query := `
		SELECT slug, name, private, length, local_only, last_sync, created_at, updated_at
		FROM playlists
		WHERE slug = ?
	`

	row := d.db.QueryRowContext(ctx, query, slug)
	playlist, err := d.scanPlaylist(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		d.debugLog("GetPlaylist", err, time.Since(start))
		return nil, fmt.Errorf("scan playlist: %w", err)
	}

	if err := d.loadPlaylistSongs(ctx, playlist); err != nil {
		d.debugLog("GetPlaylist", err, time.Since(start))
		return nil, fmt.Errorf("load playlist songs: %w", err)
	}

	return playlist, nil
}

func (d *Database) SavePlaylist(ctx context.Context, playlist *types.Playlist) error {
	start := time.Now()
	defer func() { d.debugLog("SavePlaylist", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return err
	}

	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		d.debugLog("SavePlaylist", err, time.Since(start))
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			log.Printf("Failed to rollback transaction: %v", rollbackErr)
		}
	}()

	query := `
		INSERT OR REPLACE INTO playlists (
			slug, name, private, length, local_only, last_sync, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if playlist.CreatedAt.IsZero() {
		playlist.CreatedAt = now
	}
	playlist.UpdatedAt = now

	_, err = tx.ExecContext(ctx, query,
		playlist.Slug, playlist.Name, playlist.Private, playlist.Length,
		playlist.LocalOnly, playlist.LastSync, playlist.CreatedAt, playlist.UpdatedAt,
	)
	if err != nil {
		d.debugLog("SavePlaylist", err, time.Since(start))
		return fmt.Errorf("insert playlist: %w", err)
	}

	if err := d.savePlaylistSongs(ctx, tx, playlist); err != nil {
		d.debugLog("SavePlaylist", err, time.Since(start))
		return fmt.Errorf("save playlist songs: %w", err)
	}

	return tx.Commit()
}

func (d *Database) DeletePlaylist(ctx context.Context, slug string) error {
	start := time.Now()
	defer func() { d.debugLog("DeletePlaylist", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return err
	}

	_, err := d.db.ExecContext(ctx, "DELETE FROM playlists WHERE slug = ?", slug)
	return err
}

func (d *Database) GetCachedFile(ctx context.Context, url string) (string, error) {
	start := time.Now()
	defer func() { d.debugLog("GetCachedFile", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return "", err
	}

	query := "SELECT local_path FROM cache_entries WHERE url = ?"

	var localPath string
	err := d.db.QueryRowContext(ctx, query, url).Scan(&localPath)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		d.debugLog("GetCachedFile", err, time.Since(start))
		return "", fmt.Errorf("get cached file: %w", err)
	}

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		_, _ = d.db.ExecContext(ctx, "DELETE FROM cache_entries WHERE url = ?", url)
		return "", nil
	}

	_, _ = d.db.ExecContext(ctx, "UPDATE cache_entries SET accessed_at = ? WHERE url = ?", time.Now(), url)

	return localPath, nil
}

func (d *Database) SaveCachedFile(ctx context.Context, url string, data io.Reader) (string, error) {
	start := time.Now()
	defer func() { d.debugLog("SaveCachedFile", nil, time.Since(start)) }()

	if err := d.checkClosed(); err != nil {
		return "", err
	}

	filename := filepath.Base(url)
	if filename == "." || filename == "/" {
		filename = "cached_file"
	}

	localPath := filepath.Join(d.cacheDir, filename)

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		d.debugLog("SaveCachedFile", err, time.Since(start))
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		d.debugLog("SaveCachedFile", err, time.Since(start))
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close file: %v", closeErr)
		}
	}()

	size, err := io.Copy(file, data)
	if err != nil {
		if removeErr := os.Remove(localPath); removeErr != nil {
			log.Printf("Failed to remove file after write error: %v", removeErr)
		}
		d.debugLog("SaveCachedFile", err, time.Since(start))
		return "", fmt.Errorf("write file: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO cache_entries (
			key, url, local_path, size, accessed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	_, err = d.db.ExecContext(ctx, query, filename, url, localPath, size, now, now)
	if err != nil {
		if removeErr := os.Remove(localPath); removeErr != nil {
			log.Printf("Failed to remove file after database error: %v", removeErr)
		}
		d.debugLog("SaveCachedFile", err, time.Since(start))
		return "", fmt.Errorf("save cache entry: %w", err)
	}

	return localPath, nil
}

func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	d.closed = true

	if d.db != nil {
		if _, err := d.db.Exec("PRAGMA optimize"); err != nil {
			log.Printf("Warning: Failed to optimize database: %v", err)
		}
		return d.db.Close()
	}

	return nil
}

func (d *Database) scanSong(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Song, error) {
	var song types.Song
	var volumeJSON string
	var albumSlugRef, albumName, albumImage, albumImageCropped, albumLink string

	err := scanner.Scan(
		&song.Slug, &song.Name, &song.File, &song.Image, &song.ImageCropped,
		&song.Length, &song.Played, &song.Link, &song.Liked, &volumeJSON,
		&song.AlbumSlug, &song.LocalPath, &song.Downloaded, &song.LastSync,
		&song.CreatedAt, &song.UpdatedAt,
		&albumSlugRef, &albumName, &albumImage, &albumImageCropped, &albumLink,
	)
	if err != nil {
		return nil, err
	}

	if volumeJSON != "" && volumeJSON != "[]" {
		if unmarshalErr := json.Unmarshal([]byte(volumeJSON), &song.Volume); unmarshalErr != nil {
			log.Printf("Failed to unmarshal volume JSON: %v", unmarshalErr)
		}
	}

	if albumSlugRef != "" {
		song.Album = &types.Album{
			Slug:         albumSlugRef,
			Name:         albumName,
			Image:        stringToPtr(albumImage),
			ImageCropped: stringToPtr(albumImageCropped),
			Link:         albumLink,
		}
	}

	return &song, nil
}

func (d *Database) scanAlbum(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Album, error) {
	var album types.Album

	err := scanner.Scan(
		&album.Slug, &album.Name, &album.Image, &album.ImageCropped,
		&album.Link, &album.LastSync, &album.CreatedAt, &album.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &album, nil
}

func (d *Database) scanAuthor(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Author, error) {
	var author types.Author

	err := scanner.Scan(
		&author.Slug, &author.Name, &author.Image, &author.ImageCropped,
		&author.Link, &author.LastSync, &author.CreatedAt, &author.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &author, nil
}

func (d *Database) scanPlaylist(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Playlist, error) {
	var playlist types.Playlist

	err := scanner.Scan(
		&playlist.Slug, &playlist.Name, &playlist.Private, &playlist.Length,
		&playlist.LocalOnly, &playlist.LastSync, &playlist.CreatedAt, &playlist.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &playlist, nil
}

func (d *Database) loadSongAuthors(ctx context.Context, songs []*types.Song) error {
	if len(songs) == 0 {
		return nil
	}

	slugs := make([]string, len(songs))
	for i, song := range songs {
		slugs[i] = song.Slug
	}

	placeholders := strings.Repeat("?,", len(slugs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(`
		SELECT sa.song_slug, a.slug, a.name, COALESCE(a.image_cropped, '') as image_cropped
		FROM song_authors sa
		JOIN authors a ON sa.author_slug = a.slug
		WHERE sa.song_slug IN (%s)
		ORDER BY sa.song_slug
	`, placeholders)

	args := make([]interface{}, len(slugs))
	for i, slug := range slugs {
		args[i] = slug
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query song authors: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	authorMap := make(map[string][]*types.Author)
	for rows.Next() {
		var songSlug, authorSlug, authorName, authorImage string

		if err := rows.Scan(&songSlug, &authorSlug, &authorName, &authorImage); err != nil {
			return fmt.Errorf("scan song author: %w", err)
		}

		author := &types.Author{
			Slug:         authorSlug,
			Name:         authorName,
			ImageCropped: stringToPtr(authorImage),
		}

		authorMap[songSlug] = append(authorMap[songSlug], author)
	}

	for _, song := range songs {
		song.Authors = authorMap[song.Slug]
	}

	return nil
}

func (d *Database) saveSongAuthors(ctx context.Context, tx *sql.Tx, song *types.Song) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM song_authors WHERE song_slug = ?", song.Slug); err != nil {
		return fmt.Errorf("delete old song authors: %w", err)
	}

	for _, author := range song.Authors {
		if err := d.saveAuthorInTx(ctx, tx, author); err != nil {
			return fmt.Errorf("save author: %w", err)
		}

		_, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO song_authors (song_slug, author_slug) VALUES (?, ?)",
			song.Slug, author.Slug,
		)
		if err != nil {
			return fmt.Errorf("insert song author: %w", err)
		}
	}

	return nil
}

func (d *Database) loadPlaylistSongs(ctx context.Context, playlist *types.Playlist) error {
	query := `
		SELECT s.slug, s.name, s.file, s.image, s.image_cropped, s.length, 
		       s.played, s.link, s.liked, s.volume, s.album_slug, s.local_path, 
		       s.downloaded, s.last_sync, s.created_at, s.updated_at,
		       COALESCE(a.slug, '') as album_slug_ref, 
		       COALESCE(a.name, '') as album_name, 
		       COALESCE(a.image, '') as album_image, 
		       COALESCE(a.image_cropped, '') as album_image_cropped, 
		       COALESCE(a.link, '') as album_link
		FROM playlist_songs ps
		JOIN songs s ON ps.song_slug = s.slug
		LEFT JOIN albums a ON s.album_slug = a.slug
		WHERE ps.playlist_slug = ?
		ORDER BY ps.position
	`

	rows, err := d.db.QueryContext(ctx, query, playlist.Slug)
	if err != nil {
		return fmt.Errorf("query playlist songs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Failed to close rows: %v", closeErr)
		}
	}()

	var songs []*types.Song
	for rows.Next() {
		song, err := d.scanSong(rows)
		if err != nil {
			return fmt.Errorf("scan song: %w", err)
		}
		songs = append(songs, song)
	}

	playlist.Songs = songs

	if err := d.loadSongAuthors(ctx, songs); err != nil {
		return fmt.Errorf("load song authors: %w", err)
	}

	return nil
}

func (d *Database) savePlaylistSongs(ctx context.Context, tx *sql.Tx, playlist *types.Playlist) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM playlist_songs WHERE playlist_slug = ?", playlist.Slug); err != nil {
		return fmt.Errorf("delete old playlist songs: %w", err)
	}

	for i, song := range playlist.Songs {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO playlist_songs (playlist_slug, song_slug, position) VALUES (?, ?, ?)",
			playlist.Slug, song.Slug, i,
		)
		if err != nil {
			return fmt.Errorf("insert playlist song: %w", err)
		}
	}

	return nil
}

func stringToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
