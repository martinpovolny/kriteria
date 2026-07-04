# AGENTS.md

Notes for AI agents (and humans) working on this project.

## Documentation

- **[docs/SPEC.md](docs/SPEC.md)** — Living specification: implemented features, pending work, design decisions. Update whenever you implement or change something.
- **[docs/go-embedded-ui-pattern.md](docs/go-embedded-ui-pattern.md)** — Architecture pattern for the single-binary Go + embedded Vite/React frontend. Includes NFRs, repository layout, and build checklist.

## Non-Functional Requirements

The project started with these constraints (from session start):

- Single Go binary with embedded Vite/React frontend (`//go:embed`) — see [docs/go-embedded-ui-pattern.md](docs/go-embedded-ui-pattern.md) for full NFRs
- SQLite via `modernc.org/sqlite` (pure Go, no CGO), DB file in `data/`
- UI in Czech
- Teachers authenticate via Google OAuth (dev mode uses mock teacher)
- Parents get anonymized access: URL slug + password word pair, no student names visible
- Evaluation levels 1-4 (J/Č/T/Ú), each with criterion-specific text description
- Evaluation table is append-only (audit trail: who/what/when)
- Criteria scoped per (subject, grade/ročník), not per school year
- Auth deferred for local debug (`--dev` flag, on by default)
- `.env` for secrets (OAuth credentials, session secret, edookit API)
- Word pair generation from Czech morphological dictionary (MorphoDiTa, ÚFAL UK)

## Build & Run

```
make build                              # build Go binary + embedded UI
go run ./cmd/kriteria                    # run in dev mode (mock teacher)
go run ./cmd/kriteria -dev=false         # run with OAuth (needs .env)
go run ./cmd/importer --load             # reimport .docx → JSON + DB
go run ./cmd/seed                        # seed test data
go run ./cmd/sync                        # sync students from edookit
go run ./cmd/gen-overview                # generate static prehled.html
```

## Rebuild Cycle

```
cd ui && npm run build && cd .. && make build
```

## Reseed

```
kill $(pgrep -f "bin/kriteria")  # must stop server first
rm -f data/kriteria.db
go run ./cmd/importer --load
go run ./cmd/seed
```

## Key Paths

- Server: port 8088 (`-addr=:8088`)
- DB: `data/kriteria.db`
- JSON: `data/kriteria.json`
- Source .docx: `data/kriteria/` (8 subjects x 5 grades = 108 files)
- Wordlists: `internal/api/nouns.txt`, `internal/api/adjectives.txt`
- Wordlist generator: `cmd/gen-wordlist/gen_wordlist.py`

## Lint / Typecheck

No linter configured yet. TypeScript types checked via `npm run build` in `ui/`.
Go builds checked via `go build ./...`.
