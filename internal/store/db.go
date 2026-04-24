package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steipete/wacli/internal/fsutil"
	"github.com/steipete/wacli/internal/sqliteutil"
	_ "modernc.org/sqlite"
)

type DB struct {
	path       string
	sql        *sql.DB
	ftsEnabled bool
}

func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	// Reject paths that could inject SQLite URI parameters (#59).
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("db path must not contain '?' or '#'")
	}
	if err := fsutil.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &DB{path: path, sql: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := sqliteutil.ChmodFiles(path, 0o600); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) init() error {
	// Pragmas: keep consistent for writers/readers.
	// Note: _foreign_keys and _busy_timeout are not supported in the modernc.org/sqlite
	// DSN; they are set via PRAGMA below.
	_, _ = d.sql.Exec("PRAGMA journal_mode=WAL;")
	_, _ = d.sql.Exec("PRAGMA synchronous=NORMAL;")
	_, _ = d.sql.Exec("PRAGMA temp_store=MEMORY;")
	_, _ = d.sql.Exec("PRAGMA foreign_keys=ON;")
	_, _ = d.sql.Exec("PRAGMA busy_timeout=60000;")

	if err := d.ensureSchema(); err != nil {
		return err
	}

	// Detect FTS5 availability independently of migration state. The migration
	// sets ftsEnabled only on first run; subsequent opens skip the migration.
	if !d.ftsEnabled {
		d.ftsEnabled = d.detectMessagesFTS()
	}

	return nil
}

func (d *DB) detectMessagesFTS() bool {
	ok, err := d.tableExists("messages_fts")
	if err != nil || !ok {
		return false
	}
	hasDisplayText, err := d.tableHasColumn("messages_fts", "display_text")
	if err != nil || !hasDisplayText {
		return false
	}
	var n int
	return d.sql.QueryRow("SELECT count(*) FROM messages_fts").Scan(&n) == nil
}
