package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB instance with custom helper methods.
type DB struct {
	*sql.DB
}

// NewDatabase connects to SQLite at the given path and applies migrations.
func NewDatabase(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	db := &DB{DB: sqlDB}
	if err := db.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

func (db *DB) migrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL UNIQUE,
		host_type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		last_status TEXT NOT NULL DEFAULT 'UNKNOWN',
		last_http_status INTEGER DEFAULT 0,
		last_response_time INTEGER DEFAULT 0,
		last_checked_at DATETIME,
		last_online_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
		status TEXT NOT NULL,
		check_type TEXT NOT NULL,
		http_status INTEGER DEFAULT 0,
		latency_ms INTEGER DEFAULT 0,
		port INTEGER DEFAULT 0,
		error_message TEXT DEFAULT '',
		checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS admin_users (
		user_id INTEGER PRIMARY KEY,
		first_name TEXT,
		username TEXT,
		is_owner INTEGER NOT NULL DEFAULT 0,
		role TEXT NOT NULL DEFAULT 'USER',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_hosts_enabled ON hosts(enabled);
	CREATE INDEX IF NOT EXISTS idx_checks_host_id ON checks(host_id);
	CREATE INDEX IF NOT EXISTS idx_checks_checked_at ON checks(checked_at);
	`
	_, err := db.ExecContext(ctx, schema)
	if err != nil {
		return err
	}

	// Ensure role column exists if migrated from earlier schema
	_, _ = db.ExecContext(ctx, `ALTER TABLE admin_users ADD COLUMN role TEXT NOT NULL DEFAULT 'USER'`)
	return nil
}
