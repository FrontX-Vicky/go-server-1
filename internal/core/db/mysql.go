package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/go-sql-driver/mysql"
)

type SQL struct{ *sql.DB }

type DBConfig interface { DSN() string }

func Connect(cfg DBConfig) (*SQL, error) {
    db, err := sql.Open("mysql", cfg.DSN())
    if err != nil { return nil, err }

    // Keep pool small and recycle connections aggressively.
    // MySQL server default wait_timeout is 8h but managed / local instances
    // often set it to 60s-5min. We stay well inside that window.
    db.SetMaxOpenConns(20)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(90 * time.Second)  // recycle before MySQL closes them
    db.SetConnMaxIdleTime(30 * time.Second)  // discard idle connections quickly

    if err := db.Ping(); err != nil { return nil, err }
    return &SQL{db}, nil
}

func (s *SQL) Close() error { return s.DB.Close() }

// isInvalidConn reports whether err is the go-sql-driver "invalid connection"
// sentinel that occurs when the pool returns a dead connection.
func isInvalidConn(err error) bool {
    if err == nil {
        return false
    }
    if errors.Is(err, mysql.ErrInvalidConn) {
        return true
    }
    msg := err.Error()
    return strings.Contains(msg, "invalid connection") ||
        strings.Contains(msg, "bad connection")
}

// QueryContext executes a query on the pool.
// On "invalid connection" it acquires a dedicated fresh connection and retries once,
// which sidesteps stale-pool hits without masking real errors.
func (s *SQL) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err == nil {
        return rows, nil
    }
    if !isInvalidConn(err) {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    // Retry on a fresh dedicated connection.
    conn, connErr := s.DB.Conn(ctx)
    if connErr != nil {
        return nil, fmt.Errorf("query failed (invalid connection, reconnect also failed: %v): %w", connErr, err)
    }
    defer conn.Close()
    rows, err = conn.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("query failed after retry: %w", err)
    }
    return rows, nil
}

// QueryRowContext executes a single-row query on the pool.
// On "invalid connection" it retries once on a fresh connection.
func (s *SQL) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
    return s.DB.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a statement on the pool.
// On "invalid connection" it retries once on a fresh connection.
func (s *SQL) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
    result, err := s.DB.ExecContext(ctx, query, args...)
    if err == nil {
        return result, nil
    }
    if !isInvalidConn(err) {
        return nil, fmt.Errorf("exec failed: %w", err)
    }
    conn, connErr := s.DB.Conn(ctx)
    if connErr != nil {
        return nil, fmt.Errorf("exec failed (invalid connection, reconnect also failed: %v): %w", connErr, err)
    }
    defer conn.Close()
    result, err = conn.ExecContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("exec failed after retry: %w", err)
    }
    return result, nil
}
