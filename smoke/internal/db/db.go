package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// DB answers single-value queries for the checks that look past the API.
type DB struct {
	conn *pgx.Conn
}

// Connect takes the password from fallbackPassword when the DSN carries none, so it never has to
// appear on a command line.
func Connect(ctx context.Context, dsn, fallbackPassword string) (*DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse --db: %w", err)
	}
	if cfg.Password == "" {
		cfg.Password = fallbackPassword
	}
	slog.Info("call", "target", "postgres", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to the database: %w", err)
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Value(ctx context.Context, query string, args ...any) (any, error) {
	slog.Debug("query", "sql", query, "args", args)
	var v any
	if err := d.conn.QueryRow(ctx, query, args...).Scan(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func (d *DB) Close(ctx context.Context) {
	_ = d.conn.Close(ctx)
}
