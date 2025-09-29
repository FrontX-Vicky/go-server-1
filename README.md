# server_1 starter (API + Worker)

## Quick start (local)
1) `cp configs/app.example.env .env` and edit DB1/DB2 credentials.
2) `go mod tidy`
3) `go run cmd/api/main.go`
4) `curl http://127.0.0.1:5000/api/v1/items`

## Endpoints
- GET    /api/v1/items
- POST   /api/v1/items        {"name":"Test","status":1}
- GET    /api/v1/items/:id
- PUT    /api/v1/items/:id    {"name":"New Name","status":0}
- DELETE /api/v1/items/:id

## Deploy (symlinked releases)
- Ensure `/var/www/go-workspace/server_1/{releases,shared,current}` exists.
- Put your `.env` in `/var/www/go-workspace/server_1/shared/.env`.
- Run `bash scripts/release.sh` to build, switch symlink, and restart systemd.
