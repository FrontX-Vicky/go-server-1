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
    db.SetMaxOpenConns(50)
    db.SetMaxIdleConns(10)
    db.SetConnMaxLifetime(60 * time.Minute)
    if err := db.Ping(); err != nil { return nil, err }
    return &SQL{db}, nil
}

func (s *SQL) Close() error { return s.DB.Close() }
