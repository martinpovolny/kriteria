package api

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

// currentSchoolYearLabel returns the label for the current Czech school year.
// School year runs September 1 to August 31. E.g. June 2026 → "2025/2026",
// October 2026 → "2026/2027".
func currentSchoolYearLabel() string {
	now := time.Now()
	year := now.Year()
	if now.Month() >= time.September {
		return formatSchoolYear(year, year+1)
	}
	return formatSchoolYear(year-1, year)
}

func formatSchoolYear(start, end int) string {
	return itoa(start) + "/" + itoa(end)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// subjectsHandler returns all subjects with their available grades.
// GET /api/subjects
func subjectsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// First query: get all subjects (close rows before querying grades)
		rows, err := db.QueryContext(r.Context(),
			`SELECT id, code, name FROM subject ORDER BY code`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		type gradeInfo struct {
			ID    int `json:"id"`
			Level int `json:"level"`
		}
		type subjectInfo struct {
			ID     int         `json:"id"`
			Code   string      `json:"code"`
			Name   string      `json:"name"`
			Grades []gradeInfo `json:"grades"`
		}

		var subjects []subjectInfo
		for rows.Next() {
			var s subjectInfo
			if err := rows.Scan(&s.ID, &s.Code, &s.Name); err != nil {
				continue
			}
			subjects = append(subjects, s)
		}
		rows.Close()

		// Second query: get all grade levels per subject in one query
		grRows, err := db.QueryContext(r.Context(),
			`SELECT DISTINCT c.subject_id, g.id, g.level
			 FROM criterion c
			 JOIN grade g ON c.grade_id = g.id
			 ORDER BY c.subject_id, g.level`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for grRows.Next() {
			var subjID, gradeID, level int
			if grRows.Scan(&subjID, &gradeID, &level) != nil {
				continue
			}
			for i := range subjects {
				if subjects[i].ID == subjID {
					subjects[i].Grades = append(subjects[i].Grades, gradeInfo{ID: gradeID, Level: level})
					break
				}
			}
		}
		grRows.Close()

		if subjects == nil {
			subjects = []subjectInfo{}
		}
		writeJSON(w, http.StatusOK, subjects)
	}
}

// currentSchoolYearHandler returns the current school year, creating it if missing.
// GET /api/school-years/current
func currentSchoolYearHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, label := EnsureCurrentSchoolYear(db, r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"id":    id,
			"label": label,
		})
	}
}

// EnsureCurrentSchoolYear inserts the current school year if it doesn't exist yet
// and returns (id, label).
func EnsureCurrentSchoolYear(db *sql.DB, ctx context.Context) (int64, string) {
	label := currentSchoolYearLabel()
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM school_year WHERE label = ?`, label).Scan(&id)
	if err == nil {
		return id, label
	}
	res, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO school_year (label) VALUES (?)`, label)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			id, _ = res.LastInsertId()
			return id, label
		}
	}
	db.QueryRowContext(ctx, `SELECT id FROM school_year WHERE label = ?`, label).Scan(&id)
	return id, label
}

// schoolYearsHandler returns all school years.
// GET /api/school-years
func schoolYearsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT id, label FROM school_year ORDER BY label DESC`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type year struct {
			ID    int    `json:"id"`
			Label string `json:"label"`
		}
		var years []year
		for rows.Next() {
			var y year
			rows.Scan(&y.ID, &y.Label)
			years = append(years, y)
		}

		if years == nil {
			years = []year{}
		}
		writeJSON(w, http.StatusOK, years)
	}
}

// createSchoolYearHandler creates a new school year.
// POST /api/school-years  {"label": "2025/2026"}
func createSchoolYearHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Label string `json:"label"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if body.Label == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label required"})
			return
		}

		res, err := db.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO school_year (label) VALUES (?)`, body.Label)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		var id int64
		if n, _ := res.RowsAffected(); n > 0 {
			id, _ = res.LastInsertId()
		} else {
			db.QueryRowContext(r.Context(),
				`SELECT id FROM school_year WHERE label = ?`, body.Label).Scan(&id)
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": id, "label": body.Label})
	}
}
