package api

import (
	"database/sql"
	"net/http"
)

// meHandler returns the currently authenticated teacher/director.
// GET /api/me
func meHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teacherID := teacherIDFromContext(r)
		if teacherID == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}

		var displayName, email, role string
		err := db.QueryRowContext(r.Context(),
			`SELECT display_name, email, role FROM teacher WHERE id = ?`, teacherID).
			Scan(&displayName, &email, &role)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":           teacherID,
			"display_name": displayName,
			"email":        email,
			"role":         role,
		})
	}
}
