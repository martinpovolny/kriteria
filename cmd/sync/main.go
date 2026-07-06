package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/martinpovolny/go-snip/edookit"
	"github.com/martinpovolny/kriteria/internal/api"
	"github.com/martinpovolny/kriteria/internal/store"
)

func main() {
	dbPath := flag.String("db", getenv("DB_PATH", "data/kriteria.db"), "path to SQLite database file")
	dryRun := flag.Bool("dry-run", false, "fetch from edookit but don't write to DB")
	flag.Parse()

	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	apiURL := os.Getenv("EDOOKIT_API_URL")
	apiUser := os.Getenv("EDOOKIT_API_USER")
	apiPassword := os.Getenv("EDOOKIT_API_PASSWORD")

	if apiURL == "" || apiUser == "" || apiPassword == "" {
		logger.Error("missing edookit credentials", "url", apiURL, "user", apiUser, "password_set", apiPassword != "")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	yearID, yearLabel := api.EnsureCurrentSchoolYear(st.DB(), ctx)
	logger.Info("current school year", "label", yearLabel, "id", yearID)

	client := edookit.New(apiURL, apiUser, apiPassword)

	// === Students ===

	logger.Info("fetching students from edookit", "url", apiURL)

	students, err := fetchWithBackoff(ctx, logger, client.ListStudents)
	if err != nil {
		logger.Error("failed to fetch students after retries", "err", err)
		os.Exit(1)
	}

	logger.Info("fetched students", "count", len(students))

	if *dryRun {
		for _, s := range students {
			grade := 0
			if s.CurrentGradeNum != nil {
				grade = *s.CurrentGradeNum
			}
			logger.Info("student", "person_id", s.PersonID, "name", s.Firstname+" "+s.Lastname, "grade", grade)
		}
	} else {
		var created, updated, unchanged, skipped int
		var enrollmentsEnsured int

		for _, s := range students {
			if s.CurrentGradeNum == nil || *s.CurrentGradeNum < 1 || *s.CurrentGradeNum > 9 {
				logger.Warn("student has no valid grade, skipping", "person_id", s.PersonID, "name", s.Firstname)
				skipped++
				continue
			}

			grade := *s.CurrentGradeNum
			displayName := s.Firstname + " " + s.Lastname

			action, err := upsertStudent(ctx, st.DB(), s.PersonID, displayName, grade)
			if err != nil {
				logger.Error("upsert student", "person_id", s.PersonID, "name", displayName, "err", err)
				skipped++
				continue
			}

			switch action {
			case "created":
				created++
			case "updated":
				updated++
			default:
				unchanged++
			}

			n, err := ensureEnrollments(ctx, st.DB(), s.PersonID, grade, yearID)
			if err != nil {
				logger.Error("ensure enrollments", "person_id", s.PersonID, "err", err)
			}
			enrollmentsEnsured += n
		}

		logger.Info("student sync complete",
			"created", created,
			"updated", updated,
			"unchanged", unchanged,
			"skipped", skipped,
			"enrollments_ensured", enrollmentsEnsured,
			"total_in_edookit", len(students),
		)
	}

	// === Teachers ===

	logger.Info("fetching employees from edookit", "url", apiURL)

	employees, err := fetchWithBackoff(ctx, logger, client.ListEmployees)
	if err != nil {
		logger.Error("failed to fetch employees after retries", "err", err)
		os.Exit(1)
	}

	logger.Info("fetched employees", "count", len(employees))

	if *dryRun {
		for _, e := range employees {
			email := "(no email)"
			if e.PrimaryEmail != nil {
				email = *e.PrimaryEmail
			}
			logger.Info("employee", "person_id", e.PersonID, "name", e.Firstname+" "+e.Lastname, "email", email)
		}
	} else {
		var tCreated, tUpdated, tUnchanged int

		for _, e := range employees {
			displayName := e.Firstname + " " + e.Lastname
			email := ""
			if e.PrimaryEmail != nil {
				email = *e.PrimaryEmail
			}

			action, err := upsertTeacher(ctx, st.DB(), e.PersonID, displayName, email)
			if err != nil {
				logger.Error("upsert teacher", "person_id", e.PersonID, "name", displayName, "err", err)
				continue
			}

			switch action {
			case "created":
				tCreated++
			case "updated":
				tUpdated++
			default:
				tUnchanged++
			}
		}

		logger.Info("teacher sync complete",
			"created", tCreated,
			"updated", tUpdated,
			"unchanged", tUnchanged,
			"total_in_edookit", len(employees),
		)
	}
}

type listFunc[T any] func(opts edookit.StudentDataOpts) ([]T, error)

// fetchWithBackoff calls a list function with exponential backoff.
// The edookit API is not reliable — we start with 2s delay, double on each
// retry, up to 5 attempts (2s, 4s, 8s, 16s). Total worst case ~62s.
func fetchWithBackoff[T any](ctx context.Context, logger *slog.Logger, fn listFunc[T]) ([]T, error) {
	const maxAttempts = 5
	const baseDelay = 2 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		results, err := fn(edookit.StudentDataOpts{})
		if err == nil {
			return results, nil
		}

		lastErr = err
		if attempt < maxAttempts-1 {
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			logger.Warn("edookit request failed, retrying",
				"attempt", attempt+1, "max", maxAttempts,
				"delay", delay.String(), "err", err)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

// upsertStudent creates or updates a student by person_id.
// Returns "created", "updated", or "unchanged".
func upsertStudent(ctx context.Context, db *sql.DB, personID int, displayName string, grade int) (string, error) {
	// Check if student with this person_id exists
	var existingID int64
	var existingName string
	var existingGrade int
	err := db.QueryRowContext(ctx,
		`SELECT id, display_name, current_grade FROM student WHERE person_id = ?`, personID).
		Scan(&existingID, &existingName, &existingGrade)

	if err == sql.ErrNoRows {
		// Create new student
		_, err := db.ExecContext(ctx,
			`INSERT INTO student (display_name, person_id, current_grade) VALUES (?, ?, ?)`,
			displayName, personID, grade)
		if err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}

	// Update if name or grade changed
	if existingName != displayName || existingGrade != grade {
		_, err := db.ExecContext(ctx,
			`UPDATE student SET display_name = ?, current_grade = ? WHERE id = ?`,
			displayName, grade, existingID)
		if err != nil {
			return "", err
		}
		return "updated", nil
	}
	return "unchanged", nil
}

// upsertTeacher creates or updates a teacher by edookit person_id.
// Uses "edookit:<personID>" as oauth_subject.
// Returns "created", "updated", or "unchanged".
func upsertTeacher(ctx context.Context, db *sql.DB, personID int, displayName, email string) (string, error) {
	oauthSubject := fmt.Sprintf("edookit:%d", personID)

	var existingID int64
	var existingName string
	var existingEmail string
	err := db.QueryRowContext(ctx,
		`SELECT id, display_name, email FROM teacher WHERE oauth_subject = ?`, oauthSubject).
		Scan(&existingID, &existingName, &existingEmail)

	if err == sql.ErrNoRows {
		_, err := db.ExecContext(ctx,
			`INSERT INTO teacher (oauth_subject, email, display_name, role) VALUES (?, ?, ?, 'teacher')`,
			oauthSubject, email, displayName)
		if err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}

	if existingName != displayName || existingEmail != email {
		_, err := db.ExecContext(ctx,
			`UPDATE teacher SET display_name = ?, email = ? WHERE id = ?`,
			displayName, email, existingID)
		if err != nil {
			return "", err
		}
		return "updated", nil
	}
	return "unchanged", nil
}

// ensureEnrollments auto-enrolls the student in all subjects that have criteria
// for the given grade + current school year. Returns number of new enrollments created.
func ensureEnrollments(ctx context.Context, db *sql.DB, personID int, gradeLevel int, schoolYearID int64) (int, error) {
	// Get student ID
	var studentID int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM student WHERE person_id = ?`, personID).Scan(&studentID)
	if err != nil {
		return 0, err
	}

	// Get grade ID
	var gradeID int64
	err = db.QueryRowContext(ctx,
		`SELECT id FROM grade WHERE level = ?`, gradeLevel).Scan(&gradeID)
	if err != nil {
		return 0, fmt.Errorf("grade %d not found: %w", gradeLevel, err)
	}

	// Find all subjects that have criteria for this grade
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT c.subject_id FROM criterion c WHERE c.grade_id = ?`, gradeID)
	if err != nil {
		return 0, err
	}
	var subjectIDs []int64
	for rows.Next() {
		var sid int64
		rows.Scan(&sid)
		subjectIDs = append(subjectIDs, sid)
	}
	rows.Close()

	var created int
	for _, subjectID := range subjectIDs {
		res, err := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO enrollment (student_id, subject_id, grade_id, school_year_id)
			 VALUES (?, ?, ?, ?)`,
			studentID, subjectID, gradeID, schoolYearID)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			created++
		}
	}
	return created, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
