package api

import (
	"database/sql"
	"net/http"
)

// directorStudentsHandler returns all students with their enrollments grouped.
// GET /api/director/students
func directorStudentsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT s.id, s.display_name, s.created_at
			 FROM student s ORDER BY s.display_name`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		type enrollment struct {
			SubjectCode string `json:"subject_code"`
			SubjectName string `json:"subject_name"`
			GradeLevel  int    `json:"grade_level"`
			SchoolYear  string `json:"school_year"`
		}
		type studentWithEnrollments struct {
			ID          int64        `json:"id"`
			DisplayName string       `json:"display_name"`
			CreatedAt   string       `json:"created_at"`
			Enrollments []enrollment `json:"enrollments"`
		}

		students := []studentWithEnrollments{}
		studentIdx := map[int64]int{}

		for rows.Next() {
			var s studentWithEnrollments
			rows.Scan(&s.ID, &s.DisplayName, &s.CreatedAt)
			s.Enrollments = []enrollment{}
			studentIdx[s.ID] = len(students)
			students = append(students, s)
		}
		rows.Close()

		enrRows, err := db.QueryContext(r.Context(),
			`SELECT e.student_id, s.code, s.name, g.level, sy.label
			 FROM enrollment e
			 JOIN subject s ON e.subject_id = s.id
			 JOIN grade g ON e.grade_id = g.id
			 JOIN school_year sy ON e.school_year_id = sy.id
			 ORDER BY sy.label DESC, s.code`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer enrRows.Close()

		for enrRows.Next() {
			var studentID int64
			var e enrollment
			enrRows.Scan(&studentID, &e.SubjectCode, &e.SubjectName, &e.GradeLevel, &e.SchoolYear)
			if idx, ok := studentIdx[studentID]; ok {
				students[idx].Enrollments = append(students[idx].Enrollments, e)
			}
		}

		writeJSON(w, http.StatusOK, students)
	}
}
