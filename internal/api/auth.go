package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// AuthConfig holds OAuth2 configuration for the authentication provider.
type AuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	SecretKey    string // HMAC signing key for session cookies
}

// oauthProvider wraps the OAuth2 config for Google.
func oauthProvider(cfg AuthConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

// SessionCookieName is the name of the cookie storing the session token.
const SessionCookieName = "kriteria_session"
const sessionMaxAge = 7 * 24 * 60 * 60 // 7 days in seconds

// sessionToken creates an HMAC-signed token: base64(teacherID:expiry:hmac)
func sessionToken(teacherID int64, secretKey string) (string, error) {
	expiry := time.Now().Add(sessionMaxAge * time.Second).Unix()
	payload := fmt.Sprintf("%d:%d", teacherID, expiry)
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := fmt.Sprintf("%s:%s", payload, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(token)), nil
}

// parseSessionToken validates the HMAC and returns the teacher ID, or 0 if invalid.
func parseSessionToken(token string, secretKey string) int64 {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0
	}
	parts := strings.SplitN(string(raw), ":", 3)
	if len(parts) != 3 {
		return 0
	}
	payload := parts[0] + ":" + parts[1]
	sig := parts[2]

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return 0
	}

	var teacherID int64
	var expiry int64
	fmt.Sscanf(payload, "%d:%d", &teacherID, &expiry)
	if time.Now().Unix() > expiry {
		return 0
	}
	return teacherID
}

// authLoginHandler redirects the user to the Google OAuth consent screen.
// GET /api/auth/login
func authLoginHandler(provider *oauth2.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := randString(32)
		http.SetCookie(w, &http.Cookie{
			Name:     "kriteria_oauth_state",
			Value:    state,
			Path:     "/",
			MaxAge:   600, // 10 minutes
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		url := provider.AuthCodeURL(state)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

// authCallbackHandler handles the OAuth2 callback from Google.
// GET /api/auth/callback?code=...&state=...
func authCallbackHandler(db *sql.DB, provider *oauth2.Config, authCfg AuthConfig, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify state to prevent CSRF
		stateCookie, err := r.Cookie("kriteria_oauth_state")
		if err != nil {
			http.Redirect(w, r, "/login?error=state_missing", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Query().Get("state") != stateCookie.Value {
			http.Redirect(w, r, "/login?error=state_mismatch", http.StatusTemporaryRedirect)
			return
		}
		// Clear state cookie
		http.SetCookie(w, &http.Cookie{
			Name: "kriteria_oauth_state", Value: "", Path: "/",
			MaxAge: -1,
		})

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Redirect(w, r, "/login?error=no_code", http.StatusTemporaryRedirect)
			return
		}

		// Exchange code for token
		token, err := provider.Exchange(r.Context(), code)
		if err != nil {
			logger.Error("oauth token exchange", "err", err)
			http.Redirect(w, r, "/login?error=token_exchange", http.StatusTemporaryRedirect)
			return
		}

		// Fetch user info from Google
		client := provider.Client(r.Context(), token)
		resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
		if err != nil {
			logger.Error("oauth userinfo fetch", "err", err)
			http.Redirect(w, r, "/login?error=userinfo", http.StatusTemporaryRedirect)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var userInfo struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := json.Unmarshal(body, &userInfo); err != nil {
			logger.Error("oauth userinfo parse", "err", err)
			http.Redirect(w, r, "/login?error=userinfo_parse", http.StatusTemporaryRedirect)
			return
		}

		oauthSubject := "google:" + userInfo.ID

		// Find or create teacher
		var teacherID int64
		err = db.QueryRowContext(r.Context(),
			`INSERT INTO teacher (oauth_subject, email, display_name, role)
			 VALUES (?, ?, ?, 'teacher')
			 ON CONFLICT(oauth_subject) DO UPDATE SET email=excluded.email, display_name=excluded.display_name
			 RETURNING id`, oauthSubject, userInfo.Email, userInfo.Name).Scan(&teacherID)
		if err != nil {
			db.QueryRowContext(r.Context(),
				`SELECT id FROM teacher WHERE oauth_subject = ?`, oauthSubject).Scan(&teacherID)
		}
		if teacherID == 0 {
			logger.Error("oauth: could not create/find teacher", "subject", oauthSubject, "email", userInfo.Email)
			http.Redirect(w, r, "/login?error=no_teacher", http.StatusTemporaryRedirect)
			return
		}

		// Audit log
		db.ExecContext(r.Context(),
			`INSERT INTO audit_log (actor_type, actor_id, action, detail)
			 VALUES ('teacher', ?, 'teacher.login', ?)`,
			teacherID, fmt.Sprintf(`{"oauth_subject":"%s","email":"%s"}`, oauthSubject, userInfo.Email))

		// Create session cookie
		session, _ := sessionToken(teacherID, authCfg.SecretKey)
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    session,
			Path:     "/",
			MaxAge:   sessionMaxAge,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		logger.Info("oauth login", "teacher_id", teacherID, "email", userInfo.Email)

		// Redirect to teacher UI
		http.Redirect(w, r, "/ucitel", http.StatusTemporaryRedirect)
	}
}

// authLogoutHandler clears the session cookie.
// GET /api/auth/logout
func authLogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: SessionCookieName, Value: "", Path: "/",
			MaxAge: -1,
		})
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

// authMiddleware validates the session cookie and injects teacher ID into context.
// If not authenticated, it redirects to /login for browser navigation or
// returns 401 for API calls.
func authMiddleware(db *sql.DB, secretKey string, logger *slog.Logger, next http.Handler) http.Handler {
	return authMiddlewareWithPublicPaths(db, secretKey, logger, nil, next)
}

// authMiddlewareWithPublicPaths is like authMiddleware but skips auth for
// paths listed in publicPaths (and for /z/ parent access URLs).
func authMiddlewareWithPublicPaths(db *sql.DB, secretKey string, logger *slog.Logger, publicPaths map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow public paths
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		// Allow parent access URLs (/z/:slug) — they have their own auth
		if strings.HasPrefix(r.URL.Path, "/z/") {
			next.ServeHTTP(w, r)
			return
		}
		// Allow static assets (JS, CSS, etc.)
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(SessionCookieName)
		if err == nil {
			teacherID := parseSessionToken(cookie.Value, secretKey)
			if teacherID > 0 {
				ctx := context.WithValue(r.Context(), teacherIDKey, teacherID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Not authenticated
		// For API calls (JSON), return 401
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}
		// For browser navigation, redirect to login
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})
}

// loginPageHandler serves a simple server-rendered login page.
// GET /login
func loginPageHandler(oauthEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if oauthEnabled {
			errorMsg := r.URL.Query().Get("error")
			errorHTML := ""
			if errorMsg != "" {
				errorHTML = `<div class="error">Přihlášení selhalo. Zkuste to znovu.</div>`
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, loginPageHTML, errorHTML)
		} else {
			// OAuth not configured
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(loginDisabledHTML))
		}
	}
}

// GenerateRandomKey generates a random 32-byte key for HMAC signing.
// Used when no explicit secret key is provided.
func GenerateRandomKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func randString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

const loginPageHTML = `<!DOCTYPE html>
<html lang="cs">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Přihlášení — Kriteria</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, system-ui, "Segoe UI", Roboto, sans-serif;
         background: #f5f5f5; display: flex; align-items: center; justify-content: center;
         min-height: 100vh; }
  .card { background: #fff; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);
          padding: 40px; max-width: 400px; width: 90%%; text-align: center; }
  h1 { font-size: 24px; margin-bottom: 8px; }
  .subtitle { color: #666; font-size: 14px; margin-bottom: 32px; }
  .btn { display: inline-flex; align-items: center; gap: 10px; padding: 12px 24px;
          border: 1px solid #ddd; border-radius: 8px; background: #fff; cursor: pointer;
          font-size: 15px; color: #3c4043; text-decoration: none; transition: background 0.2s; }
  .btn:hover { background: #f8f9fa; }
  .btn svg { width: 20px; height: 20px; }
  .error { background: #fee; color: #c00; padding: 10px; border-radius: 6px;
           margin-bottom: 16px; font-size: 14px; }
</style>
</head>
<body>
<div class="card">
  <h1>Kriteria</h1>
  <p class="subtitle">Kritériové hodnocení žáků</p>
  %s
  <a href="/api/auth/login" class="btn">
    <svg viewBox="0 0 24 24"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>
    Přihlásit se přes Google
  </a>
</div>
</body>
</html>`

const loginDisabledHTML = `<!DOCTYPE html>
<html lang="cs">
<head><meta charset="utf-8"><title>Kriteria</title></head>
<body>
<h1>Přihlášení není nastaveno</h1>
<p>OAuth není nakonfigurováno. Spusťte server s parametry <code>--oauth-client-id</code> a <code>--oauth-secret</code>, nebo použijte <code>--dev</code> režim.</p>
</body>
</html>`
