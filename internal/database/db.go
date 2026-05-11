package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS manga (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	author TEXT NOT NULL,
	genres TEXT NOT NULL,
	status TEXT NOT NULL,
	total_chapters INTEGER DEFAULT 0,
	description TEXT,
	cover_url TEXT
);

CREATE TABLE IF NOT EXISTS user_progress (
	user_id TEXT NOT NULL,
	manga_id TEXT NOT NULL,
	current_chapter INTEGER DEFAULT 0,
	status TEXT DEFAULT 'plan-to-read',
	rating INTEGER DEFAULT 0,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, manga_id),
	FOREIGN KEY (user_id) REFERENCES users(id),
	FOREIGN KEY (manga_id) REFERENCES manga(id)
);

CREATE TABLE IF NOT EXISTS chat_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	username TEXT NOT NULL,
	room TEXT NOT NULL,
	message TEXT NOT NULL,
	timestamp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_manga_title ON manga(title);
CREATE INDEX IF NOT EXISTS idx_progress_user ON user_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_chat_room ON chat_messages(room);
`

func Init(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}
