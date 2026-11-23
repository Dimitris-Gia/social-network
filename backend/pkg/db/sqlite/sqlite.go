// Owner: C
// Package sqlite opens the SQLite database and applies the migrations in
// pkg/db/migrations/sqlite on every start.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	sqlite3migrate "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

// MigrationsPath is relative to the backend's working directory, which is
// backend/ locally and /app in Docker.
const MigrationsPath = "file://pkg/db/migrations/sqlite"

// Open connects to the database at path (creating its folder if needed),
// turns on foreign keys and applies every pending migration.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db folder: %w", err)
	}

	// Foreign keys are off by default in SQLite; without _foreign_keys=on no
	// ON DELETE CASCADE happens. busy_timeout avoids "database is locked"
	// errors when two requests write at the same time.
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies every migration that has not run yet.
func Migrate(db *sql.DB) error {
	driver, err := sqlite3migrate.WithInstance(db, &sqlite3migrate.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(MigrationsPath, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
