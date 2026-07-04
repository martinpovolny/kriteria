package api

import (
	"context"
	"database/sql"
	"net/http"
)

type contextKey string

const teacherIDKey contextKey = "teacher_id"

// teacherIDFromContext returns the authenticated teacher's DB id, or 0 if none.
func teacherIDFromContext(r *http.Request) int64 {
	if v, ok := r.Context().Value(teacherIDKey).(int64); ok {
		return v
	}
	return 0
}

// devTeacherMiddleware injects a mock teacher into the context for local
// development without OAuth. The teacher row is created on first use.
func devTeacherMiddleware(db *sql.DB, next http.Handler) http.Handler {
	var teacherID int64

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if teacherID == 0 {
			// Create or find the dev teacher
			err := db.QueryRowContext(r.Context(),
				`INSERT INTO teacher (oauth_subject, email, display_name, role)
				 VALUES ('dev:local', 'dev@localhost', 'Učitel (dev)', 'teacher')
				 ON CONFLICT(oauth_subject) DO UPDATE SET email=excluded.email
				 RETURNING id`).Scan(&teacherID)
			if err != nil {
				// Fallback: select existing
				db.QueryRowContext(r.Context(),
					`SELECT id FROM teacher WHERE oauth_subject = 'dev:local'`).Scan(&teacherID)
			}
		}

		ctx := context.WithValue(r.Context(), teacherIDKey, teacherID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
