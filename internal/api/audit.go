package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

// auditHandler returns a flat list of all evaluation changes (who, what, when).
// GET /api/audit?limit=50
// GET /api/audit?student_id=1&limit=100
func auditHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		studentIDStr := r.URL.Query().Get("student_id")
		limit := 50
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 500 {
			limit = n
		}

		query := `SELECT e.id, e.set_at, e.level,
		         s.id, s.display_name,
		         c.code, c.name, c.category, c.subcategory,
		         sub.code, sub.name,
		         g.level,
		         t.display_name,
		         e.note
	          FROM evaluation e
	          JOIN student s ON e.student_id = s.id
	          JOIN criterion c ON e.criterion_id = c.id
	          JOIN subject sub ON c.subject_id = sub.id
	          JOIN grade g ON c.grade_id = g.id
	          JOIN teacher t ON e.teacher_id = t.id`
		args := []any{}

		if studentIDStr != "" {
			query += ` WHERE e.student_id = ?`
			args = append(args, studentIDStr)
		}
		query += ` ORDER BY e.set_at DESC LIMIT ?`
		args = append(args, limit)

		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type entry struct {
			ID            int64  `json:"id"`
			SetAt         string `json:"set_at"`
			Level         int    `json:"level"`
			StudentID     int64  `json:"student_id"`
			StudentName   string `json:"student_name"`
			CriterionCode string `json:"criterion_code"`
			CriterionName string `json:"criterion_name"`
			Category      string `json:"category"`
			Subcategory   string `json:"subcategory"`
			SubjectCode   string `json:"subject_code"`
			SubjectName   string `json:"subject_name"`
			GradeLevel    int    `json:"grade_level"`
			TeacherName   string `json:"teacher_name"`
			Note          string `json:"note"`
		}

		var entries []entry
		for rows.Next() {
			var e entry
			rows.Scan(&e.ID, &e.SetAt, &e.Level,
				&e.StudentID, &e.StudentName,
				&e.CriterionCode, &e.CriterionName, &e.Category, &e.Subcategory,
				&e.SubjectCode, &e.SubjectName,
				&e.GradeLevel,
				&e.TeacherName,
				&e.Note)
			entries = append(entries, e)
		}
		if entries == nil {
			entries = []entry{}
		}
		writeJSON(w, http.StatusOK, entries)
	}
}
