package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// teachersHandler handles GET (list) and POST (create) for teachers/directors.
// GET  /api/teachers          — list all
// POST /api/teachers          — create {"email":"...", "display_name":"...", "role":"director"}
func teachersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listTeachers(db, w, r)
		case http.MethodPost:
			createTeacher(db, w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func listTeachers(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.QueryContext(r.Context(),
		`SELECT id, oauth_subject, email, display_name, role, created_at
		 FROM teacher ORDER BY display_name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type teacher struct {
		ID           int64  `json:"id"`
		OauthSubject string `json:"oauth_subject"`
		Email        string `json:"email"`
		DisplayName  string `json:"display_name"`
		Role         string `json:"role"`
		CreatedAt    string `json:"created_at"`
	}
	var teachers []teacher
	for rows.Next() {
		var t teacher
		rows.Scan(&t.ID, &t.OauthSubject, &t.Email, &t.DisplayName, &t.Role, &t.CreatedAt)
		teachers = append(teachers, t)
	}
	if teachers == nil {
		teachers = []teacher{}
	}
	writeJSON(w, http.StatusOK, teachers)
}

func createTeacher(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if strings.TrimSpace(body.Email) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email required"})
		return
	}
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "teacher"
	}
	if role != "teacher" && role != "director" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be 'teacher' or 'director'"})
		return
	}

	// oauth_subject is derived from email for now (manual creation).
	// When OAuth is wired, the real subject will replace this.
	oauthSubject := "manual:" + strings.TrimSpace(body.Email)

	res, err := db.ExecContext(r.Context(),
		`INSERT INTO teacher (oauth_subject, email, display_name, role)
		 VALUES (?, ?, ?, ?)`,
		oauthSubject, strings.TrimSpace(body.Email),
		strings.TrimSpace(body.DisplayName), role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Audit log
	teacherID := teacherIDFromContext(r)
	if teacherID > 0 {
		db.ExecContext(r.Context(),
			`INSERT INTO audit_log (actor_type, actor_id, action, detail)
			 VALUES ('teacher', ?, 'teacher.create', ?)`,
			teacherID,
			fmt.Sprintf(`{"teacher_id":%d,"email":"%s","role":"%s"}`, id, body.Email, role))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":            id,
		"email":         strings.TrimSpace(body.Email),
		"display_name":  strings.TrimSpace(body.DisplayName),
		"role":          role,
		"oauth_subject": oauthSubject,
	})
}

// updateTeacherRoleHandler allows changing a teacher's role.
// PATCH /api/teachers/{id}/role  {"role":"director"}
func updateTeacherRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		idStr := r.PathValue("id")
		var id int64
		_, _ = fmt.Sscanf(idStr, "%d", &id)
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		var body struct {
			Role string `json:"role"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if body.Role != "teacher" && body.Role != "director" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be 'teacher' or 'director'"})
			return
		}

		_, err := db.ExecContext(r.Context(),
			`UPDATE teacher SET role = ? WHERE id = ?`, body.Role, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": id, "role": body.Role})
	}
}
