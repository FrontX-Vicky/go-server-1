package db

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

var (
	regMu sync.RWMutex
	reg   = map[string]*SQL{}
)

// Register stores a named connection. Name is case-insensitive.
func Register(name string, conn *SQL) {
	n := strings.ToUpper(strings.TrimSpace(name))
	regMu.Lock()
	defer regMu.Unlock()
	reg[n] = conn
}

// Get returns a connection and whether it exists.
func Get(name string) (*SQL, bool) {
	n := strings.ToUpper(strings.TrimSpace(name))
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := reg[n]
	return c, ok
}

// Must returns a connection or panics with a clear message.
func Must(name string) *SQL {
	if c, ok := Get(name); ok && c != nil {
		return c
	}
	panic(fmt.Sprintf("db: connection %q not found; check DB_NAMES/.env", name))
}

// DB returns the raw *sql.DB for a registered name ("DB1","DB2"...), or panics if missing.
func DB(name string) *sql.DB {
	c := Must(name)
	if c == nil || c.DB == nil {
		panic(fmt.Sprintf("db: connection %q not initialized", name))
	}
	return c.DB
}

// DBx returns the *SQL wrapper (with retry helpers) for a registered name, or panics if missing.
func DBx(name string) *SQL {
	c := Must(name)
	if c == nil {
		panic(fmt.Sprintf("db: connection %q not initialized", name))
	}
	return c
}

// CloseAll closes every registered connection.
func CloseAll() {
	regMu.Lock()
	defer regMu.Unlock()
	for k, c := range reg {
		if c != nil {
			_ = c.Close()
		}
		delete(reg, k)
	}
}
