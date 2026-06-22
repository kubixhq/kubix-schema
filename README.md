<div align="center">

<img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License" />
<img src="https://img.shields.io/badge/status-early%20development-orange.svg" alt="Status" />
<img src="https://img.shields.io/badge/go-1.22+-00ADD8.svg" alt="Go Version" />
<img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs Welcome" />

# kubix-schema

**Schema visibility for your PostgreSQL database.**

Live ERD, migration history, and diff between any two schema versions — served as a REST API.

[Getting Started](#getting-started) · [Endpoints](#endpoints) · [Configuration](#configuration) · [Contributing](#contributing)

</div>

---

## What it does

`kubix-schema` connects to your PostgreSQL database and exposes three endpoints:

| Endpoint | What it returns |
|----------|----------------|
| `GET /api/schema/erd` | All tables, columns, primary keys, foreign keys |
| `GET /api/schema/migrations` | Migration history (Flyway, Liquibase, Prisma) |
| `GET /api/schema/diff` | Diff between two schema snapshots |

Part of the [Kubix](https://github.com/kubixhq/kubix) observability platform.

---

## Getting started

**Requirements:** Go 1.22+ or Docker

### With Docker

```bash
docker run \
  -e DB_HOST=localhost \
  -e DB_PORT=5432 \
  -e DB_NAME=your_database \
  -e DB_USER=your_user \
  -e DB_PASSWORD=your_password \
  -p 8080:8080 \
  ghcr.io/kubixhq/kubix-schema:latest
```

### With Docker Compose

```yaml
services:
  kubix-schema:
    image: ghcr.io/kubixhq/kubix-schema:latest
    ports:
      - "8080:8080"
    environment:
      DB_HOST: your_db_host
      DB_PORT: 5432
      DB_NAME: your_database
      DB_USER: your_user
      DB_PASSWORD: your_password
```

### From source

```bash
git clone https://github.com/kubixhq/kubix-schema.git
cd kubix-schema
cp .env.example .env
go run ./cmd/server
```

---

## Endpoints

### `GET /api/schema/erd`

Returns all tables, columns, and relationships in your database.

```bash
curl http://localhost:8080/api/schema/erd
```

```json
{
  "tables": [
    {
      "name": "users",
      "schema": "public",
      "columns": [
        {
          "name": "id",
          "type": "uuid",
          "nullable": false,
          "default": "gen_random_uuid()"
        },
        {
          "name": "email",
          "type": "varchar",
          "nullable": false,
          "default": null
        }
      ],
      "primary_keys": ["id"],
      "foreign_keys": []
    }
  ]
}
```

---

### `GET /api/schema/migrations`

Returns migration history. Auto-detects Flyway, Liquibase, or Prisma.

```bash
curl http://localhost:8080/api/schema/migrations
```

```json
{
  "tool": "flyway",
  "migrations": [
    {
      "version": "1",
      "description": "create users table",
      "applied_at": "2024-01-15T10:30:00Z",
      "status": "success",
      "checksum": 123456789
    }
  ]
}
```

---

### `GET /api/schema/diff?from=1&to=2`

Returns the diff between two schema snapshots.

```bash
# Compare two snapshots
curl "http://localhost:8080/api/schema/diff?from=1&to=2"

# Compare snapshot against live schema
curl "http://localhost:8080/api/schema/diff?from=1&to=current"
```

```json
{
  "added": {
    "tables": ["audit_logs"],
    "columns": {
      "users": ["last_login_at"]
    }
  },
  "removed": {
    "tables": [],
    "columns": {}
  },
  "modified": {
    "columns": {
      "users.username": {
        "from": "varchar(50)",
        "to": "varchar(255)"
      }
    }
  }
}
```

**How snapshots work:**

```bash
# Before migration — save snapshot
go run ./cmd/server   # saves snapshots/1.json on startup

# After migration — save next snapshot
go run ./cmd/server   # saves snapshots/2.json on startup

# Compare
curl "localhost:8080/api/schema/diff?from=1&to=2"
```

---

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | ✅ | — | PostgreSQL host |
| `DB_PORT` | — | `5432` | PostgreSQL port |
| `DB_NAME` | ✅ | — | Database name |
| `DB_USER` | ✅ | — | Database user |
| `DB_PASSWORD` | ✅ | — | Database password |
| `DB_SSL_MODE` | — | `disable` | `disable` / `require` |
| `MIGRATION_TOOL` | — | `auto` | `flyway` / `liquibase` / `prisma` / `auto` |
| `SERVER_PORT` | — | `8080` | HTTP server port |

---

## Supported migration tools

| Tool | Detection | Table |
|------|-----------|-------|
| Flyway | Auto | `flyway_schema_history` |
| Liquibase | Auto | `databasechangelog` |
| Prisma | Auto | `_prisma_migrations` |

Set `MIGRATION_TOOL=auto` (default) and kubix-schema detects which tool you use automatically.

---

## Development

```bash
# Run tests
go test ./...

# Run with race detector
go test -race ./...

# Run integration tests (requires PostgreSQL)
go test -tags=integration ./...

# Build binary
go build -o kubix-schema ./cmd/server

# Build Docker image
docker build -t kubix-schema .
```

---

## Contributing

See the org-wide [CONTRIBUTING.md](https://github.com/kubixhq/kubix/blob/main/CONTRIBUTING.md) for guidelines.

Good first issues are tagged [`good first issue`](https://github.com/kubixhq/kubix-schema/issues?q=is%3Aissue+label%3A%22good+first+issue%22).

---

## License

Apache 2.0 — see [LICENSE](./LICENSE) for details.

---

<div align="center">
Part of <a href="https://github.com/kubixhq/kubix">Kubix</a> — built in public by <a href="https://github.com/kubixhq">kubixhq</a>
</div>