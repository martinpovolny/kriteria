package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/martinpovolny/kriteria/internal/store"
	"github.com/martinpovolny/kriteria/internal/web"
)

// BuildInfo is injected from main via -ldflags.
type BuildInfo struct {
	Commit    string
	BuildTime string
}

// NewMux wires all routes onto a single http.Handler.
func NewMux(logger *slog.Logger, st *store.Store, build BuildInfo, jsonPath string, devMode bool, authCfg AuthConfig) http.Handler {
	mux := http.NewServeMux()

	// --- Public routes ---
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"commit":     build.Commit,
			"build_time": build.BuildTime,
		})
	})

	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"commit":     build.Commit,
			"build_time": build.BuildTime,
		})
	})

	// Login page (server-rendered)
	oauthEnabled := authCfg.ClientID != "" && authCfg.ClientSecret != ""
	mux.HandleFunc("GET /login", loginPageHandler(oauthEnabled))

	// Auth routes (only when OAuth is configured)
	if oauthEnabled {
		provider := oauthProvider(authCfg)
		mux.HandleFunc("GET /api/auth/login", authLoginHandler(provider))
		mux.HandleFunc("GET /api/auth/callback", authCallbackHandler(st.DB(), provider, authCfg, logger))
		mux.HandleFunc("GET /api/auth/logout", authLogoutHandler())
	}

	// Criteria overview (from JSON file, not DB)
	mux.HandleFunc("GET /api/criteria", criteriaHandler(jsonPath))

	// Subjects & grades & school years
	mux.HandleFunc("GET /api/subjects", subjectsHandler(st.DB()))
	mux.HandleFunc("GET /api/grades", gradesHandler(st.DB()))
	mux.HandleFunc("GET /api/school-years", schoolYearsHandler(st.DB()))
	mux.HandleFunc("GET /api/school-years/current", currentSchoolYearHandler(st.DB()))
	mux.HandleFunc("POST /api/school-years", createSchoolYearHandler(st.DB()))

	// Criteria by subject+grade (from DB, includes criterion IDs for evaluations)
	mux.HandleFunc("GET /api/criteria/{subjectCode}/{gradeLevel}", criteriaBySubjectGrade(st.DB()))

	// Students & enrollments
	mux.HandleFunc("GET /api/students", studentsHandler(st.DB()))
	mux.HandleFunc("POST /api/students", studentsHandler(st.DB()))
	mux.HandleFunc("GET /api/enrollments", enrollmentHandler(st.DB()))
	mux.HandleFunc("POST /api/enrollments", enrollmentHandler(st.DB()))

	// Teachers & directors
	mux.HandleFunc("GET /api/teachers", teachersHandler(st.DB()))
	mux.HandleFunc("POST /api/teachers", teachersHandler(st.DB()))
	mux.HandleFunc("POST /api/teachers/{id}/role", updateTeacherRoleHandler(st.DB()))

	// Evaluations (append-only, audit trail)
	mux.HandleFunc("GET /api/evaluations", evaluationsHandler(st.DB()))
	mux.HandleFunc("POST /api/evaluations", evaluationsHandler(st.DB()))

	// Audit trail (who, what, when)
	mux.HandleFunc("GET /api/audit", auditHandler(st.DB()))

	// Access codes (for director to print)
	mux.HandleFunc("GET /api/access-codes", accessCodesHandler(st.DB()))
	mux.HandleFunc("GET /api/access-codes-by-class", accessCodesByClassHandler(st.DB()))

	// Director: students with enrollments + audit trail
	mux.HandleFunc("GET /api/director/students", directorStudentsHandler(st.DB()))

	// Parent access (anonymized)
	mux.HandleFunc("POST /api/parent/access", parentAccessHandler(st.DB()))
	mux.HandleFunc("GET /api/parent/access", parentAccessListHandler(st.DB()))
	mux.HandleFunc("POST /api/parent/verify", parentVerifyHandler(st.DB()))
	mux.HandleFunc("GET /api/parent/evaluations", parentEvaluationsHandler(st.DB()))

	// --- Overview pages (standalone HTML, not part of React) ---
	mux.HandleFunc("GET /prehled", overviewHandler)
	mux.HandleFunc("GET /prehled-pristupu", accessPrintHandler)

	// --- Frontend catch-all (SPA) ---
	// Must be registered last. Serves embedded assets and falls back to
	// index.html for client-side routes.
	uiFS, err := fs.Sub(web.StaticFiles, "dist")
	if err != nil {
		// dist/ may not exist yet during early development; serve a
		// placeholder so the binary still runs.
		logger.Warn("embedded dist not found, serving placeholder UI", "err", err)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(placeholderHTML))
		})
	} else {
		fileServer := http.FileServer(http.FS(uiFS))
		mux.Handle("/", spaHandler{fs: fileServer, fsys: uiFS})
	}

	// Public paths that don't require authentication
	publicPaths := map[string]bool{
		"/":                         true,
		"/login":                    true,
		"/prehled":                  true,
		"/api/auth/login":           true,
		"/api/auth/callback":        true,
		"/api/criteria":              true,
		"/api/school-years/current":  true,
		"/api/parent/verify":         true,
		"/api/parent/evaluations":   true,
	}

	var handler http.Handler = mux
	if devMode {
		handler = devTeacherMiddleware(st.DB(), handler)
	} else {
		// Production: always apply auth middleware.
		// If OAuth isn't configured, login page shows an error message.
		handler = authMiddlewareWithPublicPaths(st.DB(), authCfg.SecretKey, logger, publicPaths, handler)
	}
	return loggingMiddleware(logger, handler)
}

// spaHandler serves static assets and falls back to index.html for any path
// that is not a real file (client-side routing).
type spaHandler struct {
	fs    http.Handler
	fsys  fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if _, err := fs.Stat(h.fsys, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			// Not a real file — serve the SPA shell so client router can handle it.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			h.fs.ServeHTTP(w, r2)
			return
		}
	}
	h.fs.ServeHTTP(w, r)
}

// loggingMiddleware logs every request in a structured (JSON-ish) line.
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const placeholderHTML = `<!DOCTYPE html>
<html lang="cs"><head><meta charset="utf-8"><title>Kriteria</title></head>
<body><h1>Kriteria</h1><p>Frontend ještě nebyl sestaven. Spusťte <code>make ui-build</code>.</p></body></html>`
