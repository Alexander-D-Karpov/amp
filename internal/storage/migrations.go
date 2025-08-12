package storage

import (
	"fmt"
)

func (d *Database) runMigrations() error {
	migrations := []string{
		createTables,
		createIndexes,
	}

	for i, migration := range migrations {
		if _, err := d.db.Exec(migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

const createTables = `
CREATE TABLE IF NOT EXISTS songs (
	slug TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	file TEXT NOT NULL,
	image TEXT,
	image_cropped TEXT,
	length INTEGER DEFAULT 0,
	played INTEGER DEFAULT 0,
	link TEXT DEFAULT '',
	liked INTEGER,
	volume TEXT DEFAULT '[]',
	album_slug TEXT,
	local_path TEXT,
	downloaded BOOLEAN DEFAULT FALSE,
	last_sync TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (album_slug) REFERENCES albums(slug) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS albums (
	slug TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	image TEXT,
	image_cropped TEXT,
	link TEXT DEFAULT '',
	last_sync TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS authors (
	slug TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	image TEXT,
	image_cropped TEXT,
	link TEXT DEFAULT '',
	last_sync TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS song_authors (
	song_slug TEXT NOT NULL,
	author_slug TEXT NOT NULL,
	PRIMARY KEY (song_slug, author_slug),
	FOREIGN KEY (song_slug) REFERENCES songs(slug) ON DELETE CASCADE,
	FOREIGN KEY (author_slug) REFERENCES authors(slug) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS album_artists (
	album_slug TEXT NOT NULL,
	author_slug TEXT NOT NULL,
	PRIMARY KEY (album_slug, author_slug),
	FOREIGN KEY (album_slug) REFERENCES albums(slug) ON DELETE CASCADE,
	FOREIGN KEY (author_slug) REFERENCES authors(slug) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS playlists (
	slug TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	private BOOLEAN DEFAULT FALSE,
	length INTEGER DEFAULT 0,
	local_only BOOLEAN DEFAULT FALSE,
	last_sync TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playlist_songs (
	playlist_slug TEXT NOT NULL,
	song_slug TEXT NOT NULL,
	position INTEGER NOT NULL,
	PRIMARY KEY (playlist_slug, song_slug),
	FOREIGN KEY (playlist_slug) REFERENCES playlists(slug) ON DELETE CASCADE,
	FOREIGN KEY (song_slug) REFERENCES songs(slug) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS play_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    song_slug TEXT NOT NULL,
    user_id TEXT,
    played_at DATETIME NOT NULL,
    synced BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (song_slug) REFERENCES songs(slug)
);

CREATE INDEX IF NOT EXISTS idx_play_history_song_slug ON play_history(song_slug);
CREATE INDEX IF NOT EXISTS idx_play_history_user_id ON play_history(user_id);
CREATE INDEX IF NOT EXISTS idx_play_history_played_at ON play_history(played_at);
CREATE INDEX IF NOT EXISTS idx_play_history_synced ON play_history(synced);
CREATE INDEX IF NOT EXISTS idx_play_history_sync_query ON play_history(synced, played_at);

CREATE TABLE IF NOT EXISTS download_items (
	url TEXT PRIMARY KEY,
	local_path TEXT NOT NULL,
	progress REAL DEFAULT 0.0,
	status TEXT DEFAULT 'pending',
	error TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cache_entries (
	key TEXT PRIMARY KEY,
	url TEXT NOT NULL UNIQUE,
	local_path TEXT NOT NULL,
	size INTEGER DEFAULT 0,
	accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const createIndexes = `
CREATE INDEX IF NOT EXISTS idx_songs_album ON songs(album_slug);
CREATE INDEX IF NOT EXISTS idx_songs_downloaded ON songs(downloaded);
CREATE INDEX IF NOT EXISTS idx_songs_last_sync ON songs(last_sync);
CREATE INDEX IF NOT EXISTS idx_songs_created_at ON songs(created_at);
CREATE INDEX IF NOT EXISTS idx_songs_name ON songs(name);

CREATE INDEX IF NOT EXISTS idx_song_authors_song ON song_authors(song_slug);
CREATE INDEX IF NOT EXISTS idx_song_authors_author ON song_authors(author_slug);

CREATE INDEX IF NOT EXISTS idx_album_artists_album ON album_artists(album_slug);
CREATE INDEX IF NOT EXISTS idx_album_artists_author ON album_artists(author_slug);

CREATE INDEX IF NOT EXISTS idx_playlist_songs_playlist ON playlist_songs(playlist_slug);
CREATE INDEX IF NOT EXISTS idx_playlist_songs_position ON playlist_songs(playlist_slug, position);

CREATE INDEX IF NOT EXISTS idx_cache_entries_url ON cache_entries(url);
CREATE INDEX IF NOT EXISTS idx_cache_entries_accessed_at ON cache_entries(accessed_at);

CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(name);
CREATE INDEX IF NOT EXISTS idx_albums_name ON albums(name);
`
