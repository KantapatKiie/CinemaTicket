# Cinema Ticket Booking System

## Stack

- Backend: Go + Gin
- Frontend: Vue 3 + Vite
- Database: MongoDB
- Lock and Queue: Redis
- Realtime: WebSocket

## Run

### Prerequisites

- Docker Desktop
- Docker Compose V2

### macOS / Linux

```bash
chmod +x scripts/run-all.sh
./scripts/run-all.sh
```

### Windows (PowerShell or CMD)

```bat
scripts\\run-all.bat
```

### Direct command (all platforms)

```bash
docker compose up --build
```

Frontend: http://localhost:5173

Backend: http://localhost:8080

Health: http://localhost:8080/health

## Main APIs

- `POST /auth/mock`
- `GET /shows/:showID/seats`
- `POST /shows/:showID/seats/:seatID/lock`
- `DELETE /shows/:showID/seats/:seatID/lock`
- `POST /shows/:showID/seats/:seatID/confirm`
- `GET /ws/shows/:showID`
- `GET /admin/bookings`
- `GET /admin/audit-logs`

## Authentication

- Development mode: use `POST /auth/mock` to get JWT token
- Production mode: set `FIREBASE_PROJECT_ID` and send Firebase ID Token in `Authorization: Bearer <token>`
- If `FIREBASE_PROJECT_ID` is empty, backend uses mock JWT flow for local development
- Role is read from token claim `role` (`USER` by default, `ADMIN` for admin APIs)

## Booking Flow

1. Login to get token from `POST /auth/mock`
2. Lock seat with `POST /shows/:showID/seats/:seatID/lock`
3. Confirm booking with `POST /shows/:showID/seats/:seatID/confirm`
4. If lock is held by another user, lock request is rejected

## Lock Strategy

- Key: `lock:seat:{show_id}:{seat_id}`
- Method: `SET key user_id NX EX 300`
- Prevents duplicate lock and double booking

## Security

- Role: `USER` and `ADMIN`
- `GET /admin/bookings` is only for `ADMIN`
- `GET /admin/audit-logs` is only for `ADMIN`

## Admin Dashboard

Frontend page in role `ADMIN` can view bookings and filter by movie/date using API query

## Audit and Event

- Every critical action is stored in Mongo collection `audit_logs`
- Booking success event is published through Redis pub/sub `booking_events`
- Event consumer writes notification records into Mongo collection `notifications`

## Bonus Files

- Postman collection: `postman/CinemaTicket.postman_collection.json`
- Backend test cases: `backend/cmd/server/main_test.go`

## Additional Docs

- Structure: `docs/STRUCTURE.md`

## CI

- GitHub Actions workflow: `.github/workflows/ci.yml`
- Runs backend test/build, frontend build, and docker compose config validation on push and pull request
