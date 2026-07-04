# Kriteria

Kritériové hodnocení žáků — criteria-based student evaluation system for Czech primary schools (ZŠ).

Teachers enter level assessments (J/Č/T/Ú) per criterion. Parents access results anonymously via generated word-pair URLs. Directors manage students, access codes, and view audit trails.

## Quick Start

```bash
# 1. Build the frontend + Go binary
make build

# 2. Import criteria from .docx files into DB
go run ./cmd/importer --load

# 3. (Optional) Seed test data
go run ./cmd/seed

# 4. Run the server (dev mode, no OAuth needed)
./bin/kriteria
```

Open http://localhost:8088

## Modes

### Dev Mode (default)

Mock teacher, no login required:

```bash
./bin/kriteria
```

### Production Mode (with Google OAuth)

1. Create OAuth credentials at https://console.cloud.google.com/apis/credentials
2. Set redirect URI to `http://localhost:8088/api/auth/callback`
3. Copy `.env.example` to `.env` and fill in credentials
4. Set `DEV_MODE=false` and `SESSION_SECRET` (use `openssl rand -base64 32`)
5. Run:

```bash
./bin/kriteria -dev=false
```

## Student Sync from Edookit

Sync students from the edookit school system. Set `EDOOKIT_*` vars in `.env` first.

```bash
go run ./cmd/sync              # sync students + auto-enroll
go run ./cmd/sync --dry-run    # preview without writing
```

Students are matched by `person_id` (edookit PersonId) — safe to re-run, no data lost on re-import.

## Build Commands

```bash
make build                    # build Go binary + embedded UI
cd ui && npm run dev          # frontend dev server (HMR, proxies API to :8088)
go run ./cmd/kriteria          # run server in dev mode
go run ./cmd/importer --load   # reimport .docx → JSON + DB
go run ./cmd/seed              # seed test data
go run ./cmd/sync              # sync students from edookit
go run ./cmd/gen-overview      # generate static prehled.html
```

## Rebuild After UI Changes

```bash
cd ui && npm run build && cd .. && make build
```

## Reseed Database

```bash
kill $(pgrep -f "bin/kriteria")  # must stop server first
rm -f data/kriteria.db
go run ./cmd/importer --load
go run ./cmd/seed
```

## Documentation

- [docs/SPEC.md](docs/SPEC.md) — Living specification: features, design decisions, pending work
- [docs/go-embedded-ui-pattern.md](docs/go-embedded-ui-pattern.md) — Architecture pattern for single-binary Go + embedded Vite/React
- [AGENTS.md](AGENTS.md) — Notes for AI agents and developers

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.22+ (single binary) |
| Frontend | Vite + React + TypeScript + Tailwind CSS v4 |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| Auth | Google OAuth2 (dev mode uses mock teacher) |
| Student sync | Edookit API via `github.com/martinpovolny/go-snip/edookit` |
| Word generation | MorphoDiTa (ÚFAL UK) morphological analyzer |
