package wa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/steipete/wacli/internal/sqliteutil"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

func (c *Client) init() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "ERROR", true)
	if err := sqliteutil.ChmodFiles(c.opts.StorePath, 0o600); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", c.opts.StorePath))
	if err != nil {
		return fmt.Errorf("open whatsmeow sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return fmt.Errorf("enable whatsmeow foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=60000;"); err != nil {
		_ = db.Close()
		return fmt.Errorf("set whatsmeow busy timeout: %w", err)
	}
	container := sqlstore.NewWithDB(db, "sqlite", dbLog)
	if err := container.Upgrade(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("open whatsmeow store: %w", err)
	}
	if err := sqliteutil.ChmodFiles(c.opts.StorePath, 0o600); err != nil {
		return err
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			deviceStore = container.NewDevice()
		} else {
			return fmt.Errorf("get device store: %w", err)
		}
	}

	logger := waLog.Stdout("Client", "ERROR", true)
	c.client = whatsmeow.NewClient(deviceStore, logger)
	// Persist recently-sent messages so whatsmeow can answer retry-receipts
	// across process restarts. Without this, recipients whose Signal session
	// has not been freshly bootstrapped (typically other linked devices) see
	// "Waiting for this message" indefinitely because whatsmeow can't find the
	// original plaintext to re-encrypt when the retry arrives.
	c.client.UseRetryMessageStore = true
	return nil
}
