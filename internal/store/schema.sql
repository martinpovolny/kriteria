-- Kriteria schema v1
-- Criteria-based student evaluation with audit trail and temporal tracking.
--
-- Data model:
--   subject (Anglický jazyk, Matematika, ...)
--     └── grade (ročník 1–5) — criteria are scoped per (subject, grade)
--           └── criterion (K1, K2, ...) — belongs to (subject, grade), has a category
--                 └── criterion_level (1=J, 2=Č, 3=T, 4=Ú) — 4 levels, each with
--                     criterion-specific description text
--
--   student — enrolled in (subject, grade, school_year)
--     └── evaluation (append-only) — who/what/when: teacher_id, level, set_at
--     └── parent_access — slug+password for anonymized parent viewing

-- === Teachers & Directors =================================================
-- Authenticated via Google/Microsoft (oauth_subject). The role field
-- distinguishes teachers (enter evaluations) from directors (manage students,
-- generate access codes, view audit trail). For now both are created manually
-- or via API; OAuth wiring comes later.
CREATE TABLE IF NOT EXISTS teacher (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    oauth_subject TEXT    NOT NULL UNIQUE,   -- "google:1234567890" | "microsoft:uuid"
    email         TEXT    NOT NULL,
    display_name  TEXT    NOT NULL DEFAULT '',
    role          TEXT    NOT NULL DEFAULT 'teacher',  -- "teacher" | "director"
    created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- === Subjects ============================================================
CREATE TABLE IF NOT EXISTS subject (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    code  TEXT    NOT NULL UNIQUE,           -- "AJ", "Mat", "CJ", ...
    name  TEXT    NOT NULL UNIQUE            -- "Anglický jazyk", "Matematika", ...
);

-- === Grades (ročník) =====================================================
-- Criteria are scoped per (subject, grade). Grade 1 = first ročník, etc.
CREATE TABLE IF NOT EXISTS grade (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    level INTEGER NOT NULL UNIQUE CHECK (level BETWEEN 1 AND 9)  -- 1–9 for ZŠ
);

-- === School years (školní rok) ===========================================
-- The academic year during which evaluations are entered. A student in grade 1
-- during 2025/2026 will be in grade 2 during 2026/2027.
CREATE TABLE IF NOT EXISTS school_year (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    label TEXT    NOT NULL UNIQUE            -- "2025/2026"
);

-- === Criteria ============================================================
-- Each criterion belongs to exactly one (subject, grade) and has 4 levels.
-- Categories can be 1 or 2 levels deep (area / sub-area). Some subjects use
-- only area (AJ), others use both (CJ: "Jazyková výchova" → "Hlásky a písmena").
CREATE TABLE IF NOT EXISTS criterion (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_id   INTEGER NOT NULL REFERENCES subject(id) ON DELETE CASCADE,
    grade_id     INTEGER NOT NULL REFERENCES grade(id)   ON DELETE CASCADE,
    code         TEXT    NOT NULL,           -- "K1", "K2", ... (stable within subject+grade)
    name         TEXT    NOT NULL,           -- criterion text, 1st person (student)
    category     TEXT    NOT NULL DEFAULT '',-- e.g. "Poslech a porozumění"
    subcategory  TEXT    NOT NULL DEFAULT '',-- e.g. "Mluvení" (empty if no sub-level)
    ovu_code     TEXT    NOT NULL DEFAULT '',-- curriculum linkage, e.g. "MAT-MAT-001-ZV5-001"
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (subject_id, grade_id, code)
);

-- Level descriptions per criterion. Levels are always 1..4.
-- The letter labels J/Č/T/Ú come from the "Vysvědčení jinak" framework.
CREATE TABLE IF NOT EXISTS criterion_level (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    criterion_id INTEGER NOT NULL REFERENCES criterion(id) ON DELETE CASCADE,
    level        INTEGER NOT NULL CHECK (level BETWEEN 1 AND 4),
    letter       TEXT    NOT NULL,           -- "J", "Č", "T", "Ú"
    label        TEXT    NOT NULL,           -- "ještě neosvojeno", "částečně osvojeno", ...
    description  TEXT    NOT NULL,           -- criterion-specific text, 1st person
    UNIQUE (criterion_id, level)
);

-- === Students ============================================================
CREATE TABLE IF NOT EXISTS student (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    display_name  TEXT    NOT NULL,           -- visible to teachers only
    person_id     INTEGER UNIQUE,            -- edookit PersonId (persistent across re-imports; NULL for manual)
    current_grade INTEGER,                    -- edookit CurrentGradeNum (1–5); NULL if not synced
    created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Migration: add columns if they don't exist (idempotent for existing DBs)
-- SQLite doesn't support IF NOT EXISTS on ALTER TABLE ADD COLUMN, so we use
-- a pragma-based check in Go code (store.go migrate function).

-- Enrollment: a student is in a grade during a school year, taking a subject.
-- (student, grade, school_year) is the natural key for a class cohort; subject
-- is included because criteria differ per subject.
CREATE TABLE IF NOT EXISTS enrollment (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    student_id    INTEGER NOT NULL REFERENCES student(id)     ON DELETE CASCADE,
    subject_id    INTEGER NOT NULL REFERENCES subject(id)     ON DELETE CASCADE,
    grade_id      INTEGER NOT NULL REFERENCES grade(id)       ON DELETE CASCADE,
    school_year_id INTEGER NOT NULL REFERENCES school_year(id) ON DELETE CASCADE,
    UNIQUE (student_id, subject_id, grade_id, school_year_id)
);

-- === Parent access (anonymized) ==========================================
-- A generated URL slug + password word pair. Slug is part of the short URL
-- (hodnoceni.hmpf.cz/<slug>); password_word is the second generated word.
-- Each access maps to exactly one student. Optional scoping by subject+grade
-- +school_year lets a parent see only one enrollment's evaluations.
CREATE TABLE IF NOT EXISTS parent_access (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    slug           TEXT    NOT NULL UNIQUE,         -- "jarnivanek"
    password_hash  TEXT    NOT NULL,                -- bcrypt/argon2 of password_word
    password_plain TEXT,                             -- plaintext (for director to print; not used for auth)
    student_id     INTEGER NOT NULL REFERENCES student(id) ON DELETE CASCADE,
    -- Optional scoping: if all three are set, parent sees only this enrollment.
    -- If all NULL, parent sees all evaluations for the student.
    subject_id     INTEGER REFERENCES subject(id),
    grade_id       INTEGER REFERENCES grade(id),
    school_year_id INTEGER REFERENCES school_year(id),
    created_at     TEXT    NOT NULL DEFAULT (datetime('now')),
    revoked_at     TEXT                             -- NULL = active, non-NULL = revoked
);

-- === Evaluations (append-only, audit trail, temporal) ====================
-- This is the core table. It is APPEND-ONLY: we never UPDATE or DELETE an
-- evaluation. Every entry records WHO set it (teacher_id), WHAT level (1..4),
-- for which student+criterion, and WHEN (set_at). To get the *current* level
-- for a criterion, take the row with the latest set_at. To track progress,
-- list rows in chronological order. This gives us both the audit trail and
-- the progress-over-time view from a single table.
CREATE TABLE IF NOT EXISTS evaluation (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    student_id    INTEGER NOT NULL REFERENCES student(id)   ON DELETE CASCADE,
    criterion_id  INTEGER NOT NULL REFERENCES criterion(id) ON DELETE CASCADE,
    teacher_id    INTEGER NOT NULL REFERENCES teacher(id)   ON DELETE RESTRICT,
    level         INTEGER NOT NULL CHECK (level BETWEEN 1 AND 4),
    set_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    note          TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_evaluation_student_criterion_time
    ON evaluation (student_id, criterion_id, set_at DESC);

CREATE INDEX IF NOT EXISTS idx_evaluation_teacher
    ON evaluation (teacher_id, set_at DESC);

-- === Audit log ===========================================================
-- A generic audit log for non-evaluation actions (login, access creation,
-- revocation, criteria edits, etc.). Evaluations are self-auditing via the
-- evaluation table above.
CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_type  TEXT    NOT NULL,            -- "teacher" | "parent" | "system"
    actor_id    INTEGER,                     -- teacher.id or parent_access.id
    action      TEXT    NOT NULL,            -- e.g. "teacher.login", "parent_access.create"
    detail      TEXT    NOT NULL DEFAULT '', -- JSON blob with context
    occurred_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_log_actor
    ON audit_log (actor_type, actor_id, occurred_at DESC);
