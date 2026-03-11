# Apex Mobile Check Deposit

Mobile check deposit system for brokerage accounts. Go backend, Postgres, React + TypeScript frontend.

## Quick Start

```bash
# Prerequisites: Docker, Go 1.25+, Node 20+

# Start all services (Postgres, API, VSS stub, Settlement stub, React)
make dev

# Verify health
curl http://localhost:8080/health   # API
curl http://localhost:8081/health   # Vendor Service Stub
curl http://localhost:8082/health   # Settlement Bank Stub
# http://localhost:5173             # React app

# Stop
make down

# Reset database (drop + migrate + seed)
make reset
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| API | 8080 | Go API server |
| VSS | 8081 | Vendor Service Stub |
| Settlement | 8082 | Settlement Bank Stub |
| React | 5173 | Frontend (Vite dev server) |
| Postgres | 5433 | Database |

## Development

```bash
make dev       # Start all services
make down      # Stop all services
make logs      # Tail all logs
make reset     # Drop DB, re-run migrations + seed
make test      # Go unit tests
make test-e2e  # Playwright E2E tests
```

## Documentation

- [Product Requirements](docs/prd.md)
- [Sprint Plan](docs/sprint_plan.md)
