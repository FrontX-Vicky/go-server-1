package db

import (
    "database/sql"
    "time"

    _ "github.com/go-sql-driver/mysql"
)

type SQL struct{ *sql.DB }

type DBConfig interface { DSN() string }

func Connect(cfg DBConfig) (*SQL, error) {
    db, err := sql.Open("mysql", cfg.DSN())
    if err != nil { return nil, err }
    // Pool tuning: lower lifetime & add idle time to recycle connections regularly.
    db.SetMaxOpenConns(40)              // slightly reduced to mitigate saturation until metrics guide tuning
    db.SetMaxIdleConns(10)              // keep a modest idle buffer
    db.SetConnMaxLifetime(5 * time.Minute)  // recycle well before MySQL wait_timeout (default 8h, but managed DBs can be much lower)
    db.SetConnMaxIdleTime(2 * time.Minute)  // discard idle connections quickly to avoid stale pool hits
    if err := db.Ping(); err != nil { return nil, err }
    return &SQL{db}, nil
}

func (s *SQL) Close() error { return s.DB.Close() }
