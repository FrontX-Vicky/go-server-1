// internal/modules/test_items/repo.go
package test_items

import (
  "context"
  "database/sql"
  "errors"
  "strings"

  "server_1/internal/core/db"
)

type Repo struct {
  db1 *sql.DB
  db2 *sql.DB
}

func NewRepo() *Repo {
  return &Repo{
    db1: db.DB("DB1"),
    db2: db.DB("DB2"),
  }
}

// choose lets you pick inside your business logic without URL/body flags.
func (r *Repo) choose(name string) *sql.DB {
  if strings.EqualFold(name, "DB2") { return r.db2 }
  return r.db1
}

func (r *Repo) List(ctx context.Context, limit, offset int) ([]Item, error) {
  rows, err := r.db1.QueryContext(ctx,
    `SELECT id,name,status,created_at FROM test_items ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
  if err != nil { return nil, err }                 // ← return MySQL error as-is
  defer rows.Close()

  out := []Item{}
  for rows.Next() {
    var it Item
    if err := rows.Scan(&it.ID, &it.Name, &it.Status, &it.CreatedAt); err != nil { return nil, err }
    out = append(out, it)
  }
  return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id int64) (*Item, error) {
  var it Item
  err := r.db1.QueryRowContext(ctx,
    `SELECT id,name,status,created_at FROM test_items WHERE id=?`, id).
    Scan(&it.ID, &it.Name, &it.Status, &it.CreatedAt)
  if err != nil { return nil, err }                 // ← return MySQL error (including “table doesn't exist”)
  return &it, nil
}

func (r *Repo) Create(ctx context.Context, name string, status int) (int64, error) {
  res, err := r.db1.ExecContext(ctx, `INSERT INTO test_items(name,status) VALUES(?,?)`, strings.TrimSpace(name), status)
  if err != nil { return 0, err }                   // ← raw error
  return res.LastInsertId()
}

func (r *Repo) Update(ctx context.Context, id int64, name *string, status *int) error {
  sets := []string{}; args := []any{}
  if name != nil   { sets = append(sets, "name=?");   args = append(args, strings.TrimSpace(*name)) }
  if status != nil { sets = append(sets, "status=?"); args = append(args, *status) }
  if len(sets) == 0 { return errors.New("no fields") }
  args = append(args, id)
  _, err := r.db1.ExecContext(ctx, `UPDATE test_items SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
  return err                                         // ← raw error
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
  _, err := r.db1.ExecContext(ctx, `DELETE FROM test_items WHERE id=?`, id)
  return err                                         // ← raw error
}

/* Optional dual-DB helpers, no migrations, raw errors */

func (r *Repo) GetFromDB(ctx context.Context, dbName string, id int64) (*Item, error) {
  sqlDB := r.choose(dbName)
  var it Item
  err := sqlDB.QueryRowContext(ctx, `SELECT id,name,status,created_at FROM test_items WHERE id=?`, id).
    Scan(&it.ID, &it.Name, &it.Status, &it.CreatedAt)
  if err != nil { return nil, err }
  return &it, nil
}

func (r *Repo) ListCombined(ctx context.Context, limit, offset int) ([]Item, error) {
  type res struct{ test_items []Item; err error }
  ch1 := make(chan res, 1)
  ch2 := make(chan res, 1)

  go func() {
    rows, err := r.db1.QueryContext(ctx, `SELECT id,name,status,created_at FROM test_items ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
    if err != nil { ch1 <- res{nil, err}; return }
    defer rows.Close()
    var out []Item
    for rows.Next() {
      var it Item
      if err := rows.Scan(&it.ID, &it.Name, &it.Status, &it.CreatedAt); err != nil { ch1 <- res{nil, err}; return }
      out = append(out, it)
    }
    ch1 <- res{out, rows.Err()}
  }()
  go func() {
    rows, err := r.db2.QueryContext(ctx, `SELECT id,name,status,created_at FROM test_items ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
    if err != nil { ch2 <- res{nil, err}; return }
    defer rows.Close()
    var out []Item
    for rows.Next() {
      var it Item
      if err := rows.Scan(&it.ID, &it.Name, &it.Status, &it.CreatedAt); err != nil { ch2 <- res{nil, err}; return }
      out = append(out, it)
    }
    ch2 <- res{out, rows.Err()}
  }()

  r1, r2 := <-ch1, <-ch2
  if r1.err != nil { return nil, r1.err }           // ← whichever DB error occurred first
  if r2.err != nil { return nil, r2.err }
  return append(r1.test_items, r2.test_items...), nil
}
