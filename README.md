
# server_1 (Gin API + Worker)

A lean Go backend with:
- **Gin** HTTP API
- **Worker** process (for Redis listeners / background jobs)
- **Multi‑DB** support (DB1, DB2) selectable **inside code**
- Direct build into a single **live** folder
- **Single `.env`** at the project root

---

## Repo layout

```
server_1/
├─ cmd/
│  ├─ api/                 # HTTP entrypoint (Gin)
│  └─ worker/              # Background worker entrypoint
├─ internal/
│  ├─ bootstrap/           # process bootstrap for api
│  ├─ core/
│  │  ├─ config/           # env & multi-DB config
│  │  ├─ db/               # mysql client + registry
│  │  └─ httpx/            # response helpers
│  ├─ router/              # HTTP routing only
│  ├─ modules/
│  │  └─ items/            # demo CRUD module (no migrations)
│  ├─ events/              # (future) event types/listeners
│  └─ worker/              # worker bootstrap (skeleton)
├─ deployments/
│  └─ systemd/             # systemd unit templates
├─ scripts/
│  └─ release.sh           # build-in-place, restart services
├─ go.mod
└─ .env                    # single env for API + Worker (project root)
```

---

## Requirements

- Go **1.21+**
- MySQL reachable for **DB1** and **DB2**
- (Optional) systemd on the server
- (Optional) Nginx if serving behind reverse proxy

---

## Environment (single .env at project root)

```
APP_ENV=dev
SERVER_ADDR=127.0.0.1
SERVER_PORT=5000
BASE_PATH=/

# register multiple DBs you want available in code
DB_NAMES=DB1,DB2

# DB1
DB1_HOST=127.0.0.1
DB1_PORT=3306
DB1_USER=root
DB1_PASS=pass
DB1_NAME=appdb
DB1_PARAMS=charset=utf8mb4&parseTime=True&loc=Local

# DB2
DB2_HOST=127.0.0.1
DB2_PORT=3306
DB2_USER=central_user
DB2_PASS=central_pass
DB2_NAME=pf_central
DB2_PARAMS=charset=utf8mb4&parseTime=True&loc=Local
```

> Both API and Worker read the **same `.env`** at **project root**.

---

## Local run

```bash
cp configs/app.example.env .env   # then edit credentials
go mod tidy
go run cmd/api/main.go
# Health
curl http://127.0.0.1:5000/health
# Demo CRUD
curl http://127.0.0.1:5000/api/v1/items
```

### Demo endpoints
- `GET    /api/v1/items`
- `POST   /api/v1/items        {"name":"Test","status":1}`
- `GET    /api/v1/items/:id`
- `PUT    /api/v1/items/:id    {"name":"New Name","status":0}`
- `DELETE /api/v1/items/:id`

**Error behaviour:** no migrations, no schema checks. If tables are missing, responses include real MySQL errors (e.g., *Error 1146: Table 'appdb.items' doesn't exist*). Only `sql.ErrNoRows` becomes **404 Not Found**.

---

## Multi‑DB usage (inside code)

- All DBs from `DB_NAMES` are connected on boot and registered as `"DB1"`, `"DB2"`, etc.
- Repos can use **both** DBs in one function. Example pattern used in `items`:
  - Default operations hit **DB1**.
  - `GetFromDB(ctx, "DB2", id)` shows how to query **DB2** when needed.
  - `ListCombined` demonstrates reading **both DB1 and DB2 in parallel** and merging.

No DB selection is exposed via URL/body; selection happens **inside business logic**.

---

## Build & deploy (server)

We build **directly into a live folder** and restart services.

Paths:
- Project root: `/var/www/go-workspace/server_1`
- Live binaries: `/var/www/go-workspace/server_1/live/{app,app-worker}`
- Env file: `/var/www/go-workspace/server_1/.env`

### 1) Update script
```
bash scripts/release.sh
```

What it does:
- Builds `app` and `app-worker` into `live/`
- Restarts systemd units

### 2) Systemd units (names you use)

- API: `/etc/systemd/system/go-server-1.service`
- Worker: `/etc/systemd/system/go-server-1-worker.service`

Both units use:
- `WorkingDirectory=/var/www/go-workspace/server_1/live`
- `ExecStart=/var/www/go-workspace/server_1/live/app` (or `app-worker`)
- `EnvironmentFile=/var/www/go-workspace/server_1/.env`

Enable + start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable go-server-1.service go-server-1-worker.service
sudo systemctl restart go-server-1.service go-server-1-worker.service
```

Check:
```bash
sudo systemctl status go-server-1.service go-server-1-worker.service
journalctl -u go-server-1.service -f
```

---

## Worker & listeners (future)

- Worker skeleton is in `cmd/worker` + `internal/worker`.
- Add Redis/Kafka listeners under `internal/events/listeners/{redis,kafka}/…`.
- Start them from `internal/worker/app.go` (you can run multiple goroutines, or multiple systemd instances if you adopt templated units later).

---

## Reverse proxy (optional)
If fronted by Nginx, map `/go` → `127.0.0.1:5000` and keep buffering on for JSON APIs. TLS termination stays in Nginx.

---

## Troubleshooting

- **DB not connected**: ensure `DB_NAMES` include the names you’re using, and credentials are correct.
- **“table doesn’t exist”**: expected if you haven’t created schema; the API returns raw MySQL error.
- **Port in use**: change `SERVER_PORT` or stop the other process using it.
- **Systemd won’t start**: check `EnvironmentFile` path and permissions; run `journalctl -xe` for details.

---

## Next steps

- Add your real modules (controllers + services + repos).
- Wire WebSockets under `internal/modules/ws` and mount at `/api/v1/ws`.
- Add Redis Stream listeners for DB change events (project already structured for it).
