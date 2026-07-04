package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// studentsHandler handles GET (list) and POST (create) for students.
// GET  /api/students                 — list all students
// POST /api/students                 — create a student {"display_name": "..."}
func studentsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listStudents(db, w, r)
		case http.MethodPost:
			createStudent(db, w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func listStudents(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Optional filters: subject_id, grade_id, school_year_id
	// When subject_id + grade_id are provided, return only students enrolled
	// in that subject+grade (+optional school_year).
	subjectIDStr := r.URL.Query().Get("subject_id")
	gradeIDStr := r.URL.Query().Get("grade_id")
	schoolYearIDStr := r.URL.Query().Get("school_year_id")

	var (
		query string
		args  []any
	)

	if subjectIDStr != "" && gradeIDStr != "" {
		query = `SELECT DISTINCT s.id, s.display_name, s.created_at
		         FROM student s
		         JOIN enrollment e ON e.student_id = s.id
		         WHERE e.subject_id = ? AND e.grade_id = ?`
		args = append(args, subjectIDStr, gradeIDStr)
		if schoolYearIDStr != "" {
			query += ` AND e.school_year_id = ?`
			args = append(args, schoolYearIDStr)
		}
		query += ` ORDER BY s.display_name`
	} else {
		query = `SELECT id, display_name, created_at FROM student ORDER BY display_name`
	}

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type student struct {
		ID          int64  `json:"id"`
		DisplayName string `json:"display_name"`
		CreatedAt   string `json:"created_at"`
	}
	var students []student
	for rows.Next() {
		var s student
		rows.Scan(&s.ID, &s.DisplayName, &s.CreatedAt)
		students = append(students, s)
	}
	if students == nil {
		students = []student{}
	}
	writeJSON(w, http.StatusOK, students)
}

func createStudent(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if strings.TrimSpace(body.DisplayName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name required"})
		return
	}

	res, err := db.ExecContext(r.Context(),
		`INSERT INTO student (display_name) VALUES (?)`, strings.TrimSpace(body.DisplayName))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Auto-generate a parent access code (no subject/grade/year scoping —
	// parent sees all evaluations for this student)
	slug, password := generateWordPair()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash failed"})
		return
	}
	_, err = db.ExecContext(r.Context(),
		`INSERT INTO parent_access (slug, password_hash, password_plain, student_id) VALUES (?, ?, ?, ?)`,
		slug, string(hash), password, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"display_name": strings.TrimSpace(body.DisplayName),
		"access_slug":  slug,
		"access_url":   "/z/" + slug,
		"access_password": password,
	})
}

// enrollmentHandler handles GET (list) and POST (create) for enrollments.
// GET  /api/enrollments?student_id=1
// POST /api/enrollments  {"student_id":1,"subject_id":2,"grade_id":3,"school_year_id":4}
func enrollmentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listEnrollments(db, w, r)
		case http.MethodPost:
			createEnrollment(db, w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func listEnrollments(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	studentIDStr := r.URL.Query().Get("student_id")
	if studentIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student_id required"})
		return
	}
	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid student_id"})
		return
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT e.id, e.student_id, s.code, s.name, g.level, sy.label
		 FROM enrollment e
		 JOIN subject s ON e.subject_id = s.id
		 JOIN grade g ON e.grade_id = g.id
		 JOIN school_year sy ON e.school_year_id = sy.id
		 WHERE e.student_id = ?
		 ORDER BY sy.label DESC, s.code`, studentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type enr struct {
		ID          int64  `json:"id"`
		StudentID   int64  `json:"student_id"`
		SubjectCode string `json:"subject_code"`
		SubjectName string `json:"subject_name"`
		GradeLevel  int    `json:"grade_level"`
		SchoolYear  string `json:"school_year"`
	}
	var enrollments []enr
	for rows.Next() {
		var e enr
		rows.Scan(&e.ID, &e.StudentID, &e.SubjectCode, &e.SubjectName, &e.GradeLevel, &e.SchoolYear)
		enrollments = append(enrollments, e)
	}
	if enrollments == nil {
		enrollments = []enr{}
	}
	writeJSON(w, http.StatusOK, enrollments)
}

func createEnrollment(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var body struct {
		StudentID    int64 `json:"student_id"`
		SubjectID    int64 `json:"subject_id"`
		GradeID      int64 `json:"grade_id"`
		SchoolYearID int64 `json:"school_year_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.StudentID == 0 || body.SubjectID == 0 || body.GradeID == 0 || body.SchoolYearID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "all fields required"})
		return
	}

	// Get grade level for response
	var gradeLevel int
	db.QueryRowContext(r.Context(), `SELECT level FROM grade WHERE id = ?`, body.GradeID).Scan(&gradeLevel)

	res, err := db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO enrollment (student_id, subject_id, grade_id, school_year_id)
		 VALUES (?, ?, ?, ?)`,
		body.StudentID, body.SubjectID, body.GradeID, body.SchoolYearID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var id int64
	if n, _ := res.RowsAffected(); n > 0 {
		id, _ = res.LastInsertId()
	} else {
		db.QueryRowContext(r.Context(),
			`SELECT id FROM enrollment WHERE student_id=? AND subject_id=? AND grade_id=? AND school_year_id=?`,
			body.StudentID, body.SubjectID, body.GradeID, body.SchoolYearID).Scan(&id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          id,
		"student_id":  body.StudentID,
		"subject_id":  body.SubjectID,
		"grade_id":    body.GradeID,
		"grade_level": gradeLevel,
		"school_year_id": body.SchoolYearID,
	})
}

// gradesHandler returns all grade levels that have criteria.
// GET /api/grades
func gradesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT DISTINCT g.id, g.level FROM grade g ORDER BY g.level`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type grade struct {
			ID    int `json:"id"`
			Level int `json:"level"`
		}
		var grades []grade
		for rows.Next() {
			var g grade
			rows.Scan(&g.ID, &g.Level)
			grades = append(grades, g)
		}
		if grades == nil {
			grades = []grade{}
		}
		writeJSON(w, http.StatusOK, grades)
	}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	return dec.Decode(v)
}
