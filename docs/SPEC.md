# Kriteria — Living Specification

> **Last updated:** 2026-06-30
> **Status:** Active development

Kritériové hodnocení žáků — a criteria-based student evaluation system for Czech primary schools (ZŠ).

Teachers enter level assessments (J/Č/T/Ú) per criterion. Parents access results anonymously via generated word-pair URLs. Directors manage students, access codes, and view audit trails.

---

## Architecture

| Layer | Technology |
|---|---|
| Backend | Go 1.22+ (single binary) |
| Frontend | Vite + React + TypeScript + Tailwind CSS v4 |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| Embedding | `//go:embed` of built React app into Go binary |
| Auth | Google OAuth2 (dev mode uses mock teacher) |
| Word generation | MorphoDiTa (ÚFAL UK) morphological analyzer |

### Project Structure

```
cmd/
  kriteria/        server entrypoint
  importer/        .docx → JSON + DB importer
  seed/            test data seeder
  gen-overview/    static HTML overview generator
  gen-wordlist/    Python script for MorphoDiTa wordlist generation
internal/
  api/             HTTP handlers, middleware, auth
  docx/            .docx parser (Office Open XML)
  importer/        import logic
  store/           SQLite open/migrate
  web/             embedded frontend assets
ui/
  src/             React app
data/
  kriteria/        source .docx files (108 files, 8 subjects × 5 grades)
  kriteria.json    generated review artifact
  kriteria.db      SQLite database
```

---

## Implemented Features

### ✅ Project Scaffold
- Go module, Vite+React+TS+Tailwind v4, Makefile
- Single binary with embedded UI via `//go:embed`
- `.gitignore` for build artifacts, DB, secrets, `.env`

### ✅ Database Schema (10 tables)
- `teacher` — with `role` column (`teacher` | `director`), `oauth_subject` for OAuth
- `subject`, `grade` (1–9 for ZŠ), `school_year`
- `criterion` — scoped per (subject, grade), with category/subcategory/ovu_code
- `criterion_level` — 4 levels per criterion (J/Č/T/Ú), each with description text
- `student`, `enrollment` — student enrolled in (subject, grade, school_year)
- `parent_access` — slug + bcrypt password hash + `password_plain` for printing
- `evaluation` — append-only, with teacher_id, level, set_at, note
- `audit_log` — generic audit trail for non-evaluation actions
- Indexed: `(student_id, criterion_id, set_at DESC)`, `(teacher_id, set_at DESC)`

### ✅ Criteria Import
- `.docx` parser reads Office Open XML (paragraphs with styles + tables)
- 74 .docx files parsed → 792 criteria, 3168 level descriptions
- 8 subjects: AJ, CAS, CJ, Inf, Mat, PV, TV, UMT
- 5 grades (1–5), all criteria have all 4 levels (no gaps)
- JSON review artifact + `--load` flag for DB insertion

### ✅ API
| Endpoint | Method | Description |
|---|---|---|
| `/healthz`, `/api/version` | GET | Health check, build info |
| `/api/subjects` | GET | All subjects with available grades |
| `/api/grades` | GET | All grade levels |
| `/api/school-years` | GET | All school years |
| `/api/school-years/current` | GET | Current school year (auto-created on startup) |
| `/api/school-years` | POST | Create school year |
| `/api/criteria` | GET | All criteria (from JSON file) |
| `/api/criteria/{subjectCode}/{gradeLevel}` | GET | Criteria with levels for subject+grade |
| `/api/students` | GET, POST | List (with filters) / create students |
| `/api/enrollments` | GET, POST | List / create enrollments |
| `/api/evaluations` | GET, POST | Get evaluations (current+history) / set evaluation (append-only) |
| `/api/audit` | GET | Audit trail of evaluation changes |
| `/api/access-codes` | GET | Access codes for printing |
| `/api/access-codes-by-class` | GET | Access codes grouped by class |
| `/api/parent/access` | GET, POST | List / generate parent access |
| `/api/parent/verify` | POST | Verify slug+password → session token |
| `/api/parent/evaluations` | GET | Anonymized evaluations for parent |
| `/api/teachers` | GET, POST | List / create teachers |
| `/api/teachers/{id}/role` | POST | Update teacher role |
| `/api/director/students` | GET | All students with enrollments (director view) |
| `/api/auth/login` | GET | OAuth login redirect to Google |
| `/api/auth/callback` | GET | OAuth callback handler |
| `/api/auth/logout` | GET | Clear session, redirect to home |
| `/prehled` | GET | Criteria overview page (standalone HTML) |
| `/prehled-pristupu` | GET | Access codes print page (standalone HTML) |

### ✅ Authentication & Authorization
- Google OAuth2 via `golang.org/x/oauth2`
- Session cookies: HMAC-signed (teacher_id:expiry:signature)
- Dev mode (`--dev`, default): mock teacher injected, no login needed
- Production (`-dev=false`): unauthenticated → redirect to `/login`
- Public paths (no auth): `/`, `/login`, `/prehled`, `/z/:slug`, `/api/criteria`, `/api/parent/verify`, `/api/parent/evaluations`, static assets
- `.env` for secrets (loaded via `github.com/joho/godotenv`)
- Login page: server-rendered HTML with "Sign in with Google" button

### ✅ Teacher UI (`/ucitel`)
- Pick ročník → předmět (filtered) → student list (filtered by enrollment + school year)
- Criteria displayed grouped by category/subcategory
- Level buttons: J (red), Č (yellow), T (blue), Ú (green) — color-coded
- Current level highlighted, level description shown
- School year selector (defaults to current year)
- Add student inline
- Logout link

### ✅ Parent UI (`/z/{slug}`)
- Password login (word pair)
- Anonymized results — no student name, no teacher name
- Current level + history per criterion
- All criteria shown (even without evaluations yet)

### ✅ Director Print Page (`/prehled-pristupu`)
- Cards with student name, URL, password
- Grouped by class (grade level)
- Page break between classes for printing
- Print button

### ✅ Word Pair Generation
- Generator script: `cmd/gen-wordlist/gen_wordlist.py`
- Uses MorphoDiTa REST API (ÚFAL UK) to classify Czech words by POS
- Downloads Czech frequency list (OpenSubtitles 2018, 50k words)
- Disables morphological guesser (only dictionary words)
- Filters: 4–10 chars, diacritics stripped, deduplicated, ambiguous words removed
- Output: `internal/api/nouns.txt` (7,812 nouns), `internal/api/adjectives.txt` (2,666 adjectives)
- ~20.8M combinations per pair (adjective+noun)
- Example: slug=`napajenysvedomi`, password=`vyresenypirko`

### ✅ School Year Support
- `currentSchoolYearLabel()`: Czech school year (Sep 1–Aug 31)
- Current year auto-created on server startup
- `GET /api/school-years/current` endpoint
- Teacher UI: school year selector, defaults to current year
- Student list filtered by school year via enrollment

### ✅ Edookit Sync (Students + Teachers)
- `cmd/sync/main.go` — syncs students from edookit API
- Reads `EDOOKIT_API_URL`, `EDOOKIT_API_USER`, `EDOOKIT_API_PASSWORD` from `.env`
- Uses `github.com/martinpovolny/go-snip/edookit` client
- `person_id` (edookit PersonId) is the persistent key — survives deletions and re-creations
- Upsert by `person_id`: creates if new, updates if name/grade changed, skips if unchanged
- Auto-enrolls each student in all subjects that have criteria for their `current_grade`
- Students with grade 0 (preschool/kindergarten) are skipped
- Exponential backoff: 5 attempts, 2s/4s/8s/16s/32s (edookit API is not reliable)
- `--dry-run` flag to preview without writing to DB
- Idempotent: safe to re-run (upsert + `INSERT OR IGNORE` for enrollments)
- 54 students synced from 85 in edookit (31 skipped — grade 0)
- `ListEmployees` syncs teachers into `teacher` table via `edookit:<PersonID>` as `oauth_subject`
- Idempotent upsert by `oauth_subject`: creates if new, updates if name/email changed
- Teacher sync runs alongside student sync in a single `go run ./cmd/sync` pass

### ✅ Dev Mode
- `--dev` flag (on by default)
- Mock teacher middleware injects `dev:local` teacher
- No OAuth needed for local development
- Seed data: 8 students, 3 grades, sample evaluations, access codes

---

## Pending Work

### 🔲 Director UI (`/reditel`)
- Students list with enrollments (API exists: `/api/director/students`)
- Audit log viewer (API exists: `/api/audit`)
- Teacher management (API exists: `/api/teachers`)
- **Status:** In progress — API and types ready, UI not built yet

### 🔲 Enrollment Management UI
- Assign students to subjects/grades/years
- API exists (`/api/enrollments`), no UI yet
- Auto-enrollment happens via edookit sync, but manual adjustments may be needed

### ✅ Progress-over-Time View
- `ProgressTimeline` component: horizontal timeline of level changes per criterion
- Colored dots (J=red, Č=yellow, T=blue, Ú=green) with dates, arrows showing progression
- Summary stats: count evaluated/total, average level
- Level distribution bar: visual breakdown of J/Č/T/Ú proportions
- Per-criterion cards: border colored by current level, scrollable timeline
- **Parent view** (`/z/{slug}`): "Postup v čase" tab, anonymized (no teacher names)
- **Teacher view** (`/ucitel`): "Postup v čase" toggle for selected student, shows teacher names
- **Director view** (`/reditel`): "Postup v čase" tab with student selector, shows teacher names

### 🔲 Parent Access Revocation
- Schema has `revoked_at` column
- No API/UI to revoke access yet

### 🔲 Teacher Scoping
- Teachers should see only their students (relation to be established)
- Currently all teachers see all students

### 🔲 Production Hardening
- TLS termination (reverse proxy or built-in)
- Rate limiting on parent verify endpoint
- Persistent parent sessions (currently in-memory, lost on restart)
- `SESSION_SECRET` should be set explicitly in production

---

## Design Decisions

### SQLite Concurrency
- `SetMaxOpenConns(1)` to avoid nested query deadlocks with pure-Go SQLite driver

### Criteria Source
- `/api/criteria` serves from JSON file (not DB) so review works without DB load
- Importer writes JSON always; DB load only with `--load` flag

### Teacher Workflow
- Teacher picks ročník first, then předmět (filtered to subjects available for that grade)
- Students filtered by enrollment in selected subject+grade+school_year

### Parent Access
- `password_plain` stored in DB for director printing (bcrypt hash used for auth)
- Access codes auto-generated when student is created
- Word pairs: adjective+noun concatenated (e.g. `modrystrom`)

### Evaluation Storage
- Append-only: never UPDATE or DELETE
- Latest entry = current level, older entries = history
- Single table gives both audit trail and progress-over-time

### School Year
- Computed from current date: Sep–Dec → `YYYY/YYYY+1`, Jan–Aug → `YYYY-1/YYYY`
- Auto-created on server startup if missing

### Auth Middleware
- Dev mode: mock teacher for all requests
- Production: HMAC-signed session cookies, public paths bypass auth
- Parent access URLs (`/z/:slug`) are always public (own auth via word pair)

---

## Build & Run

```
make build                              # build Go binary + embedded UI
go run ./cmd/kriteria                    # run in dev mode (mock teacher)
go run ./cmd/kriteria -dev=false         # run with OAuth (needs .env)
go run ./cmd/importer --load             # reimport .docx → JSON + DB
go run ./cmd/seed                        # seed test data
go run ./cmd/gen-overview                # generate static prehled.html
python3 cmd/gen-wordlist/gen_wordlist.py # regenerate wordlists
```

### Reseed

```
kill $(pgrep -f "bin/kriteria")  # must stop server first
rm -f data/kriteria.db
go run ./cmd/importer --load
go run ./cmd/seed
```

### OAuth Setup

1. Create OAuth credentials at https://console.cloud.google.com/apis/credentials
2. Set redirect URI to `http://localhost:8088/api/auth/callback`
3. Copy `.env.example` to `.env` and fill in credentials
4. Set `DEV_MODE=false` and `SESSION_SECRET` (use `openssl rand -base64 32`)
5. Run `./bin/kriteria`
