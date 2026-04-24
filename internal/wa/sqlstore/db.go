package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type scannable interface {
	Scan(dest ...any) error
}

type database struct {
	raw *sql.DB
	tx  *sql.Tx
}

func newDatabase(db *sql.DB) *database {
	return &database{raw: db}
}

func (db *database) Close() error {
	return db.raw.Close()
}

func (db *database) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = sqliteQuery(query)
	if db.tx != nil {
		return db.tx.ExecContext(ctx, query, args...)
	}
	return db.raw.ExecContext(ctx, query, args...)
}

func (db *database) Query(ctx context.Context, query string, args ...any) (rows, error) {
	query = sqliteQuery(query)
	if db.tx != nil {
		return db.tx.QueryContext(ctx, query, args...)
	}
	return db.raw.QueryContext(ctx, query, args...)
}

func (db *database) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	query = sqliteQuery(query)
	if db.tx != nil {
		return db.tx.QueryRowContext(ctx, query, args...)
	}
	return db.raw.QueryRowContext(ctx, query, args...)
}

func (db *database) DoTxn(ctx context.Context, _ any, fn func(context.Context) error) error {
	if db.tx != nil {
		return fn(ctx)
	}
	tx, err := db.raw.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txnDB := &database{raw: db.raw, tx: tx}
	if err := fn(context.WithValue(ctx, txnDBKey{}, txnDB)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type txnDBKey struct{}

func dbFromContext(ctx context.Context, fallback *database) *database {
	if db, ok := ctx.Value(txnDBKey{}).(*database); ok {
		return db
	}
	return fallback
}

var dollarParam = regexp.MustCompile(`\$\d+`)

func sqliteQuery(query string) string {
	return dollarParam.ReplaceAllString(query, "?")
}

func buildMassInsert[T any, C any](base, pattern string, common C, items []T, values func(C, T) []any) (string, []any) {
	idx := strings.Index(base, "VALUES")
	if idx == -1 {
		panic("mass insert query missing VALUES")
	}
	prefix := base[:idx+len("VALUES")]
	vars := make([]any, 0, len(items)*4)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		vals := values(common, item)
		placeholders := make([]any, len(vals))
		for i := range vals {
			vars = append(vars, vals[i])
			placeholders[i] = len(vars)
		}
		parts = append(parts, fmt.Sprintf(pattern, placeholders...))
	}
	return prefix + " " + strings.Join(parts, ",") + base[idx+len("VALUES"):], vars
}
