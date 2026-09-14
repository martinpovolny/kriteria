package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

//go:embed adjectives.txt
var adjectivesRaw string

//go:embed nouns.txt
var nounsRaw string

var (
	adjectives []string
	nouns      []string
)

func init() {
	for _, line := range strings.Split(adjectivesRaw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			adjectives = append(adjectives, line)
		}
	}
	for _, line := range strings.Split(nounsRaw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			nouns = append(nouns, line)
		}
	}
}

// generateWordPair returns (slug, password) — each an adjective+noun pair
// concatenated, e.g. slug="modrystrom", password="tichyhrad".
// ~7800 nouns × ~2700 adjectives = ~21M combinations per pair.
// Easy to say over the phone ("modrý strom"), hard to guess.

// parentSessions holds in-memory sessions for verified parent access.
// In production this would be in DB or a session store; for now in-memory is fine.
var (
	parentSessions   = map[string]int64{} // token → parent_access.id
	parentSessionsMu sync.RWMutex
)

// generateWordPair returns (slug, password) — each an adjective+noun pair.
func generateWordPair() (string, string) {
	return genAdjNoun(), genAdjNoun()
}

func genAdjNoun() string {
	return adjectives[randInt(len(adjectives))] + nouns[randInt(len(nouns))]
}

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

// parentAccessHandler handles generating word pairs for a student.
// POST /api/parent/access
// {"student_id":1, "subject_id":2, "grade_id":3, "school_year_id":4}
// → {"slug":"jarnivanek", "password":"zelenatrave", ...}
func parentAccessHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		teacherID := teacherIDFromContext(r)
		if teacherID == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}

		var body struct {
			StudentID    int64 `json:"student_id"`
			SubjectID    int64 `json:"subject_id"`
			GradeID      int64 `json:"grade_id"`
			SchoolYearID int64 `json:"school_year_id"`
		}
		if err := decodeJSON(r, &body); err != nil || body.StudentID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student_id required"})
			return
		}

		// Generate unique slug
		var slug, password string
		var slugTaken bool
		for i := 0; i < 10; i++ {
			slug, password = generateWordPair()
			err := db.QueryRowContext(r.Context(),
				`SELECT 1 FROM parent_access WHERE slug = ? AND revoked_at IS NULL`, slug).Scan(&slugTaken)
			if err == sql.ErrNoRows {
				slugTaken = false
				break
			}
		}
		if slugTaken {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate unique slug"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash failed"})
			return
		}

		// subject_id, grade_id, school_year_id are optional scoping (nullable)
		var subjectID, gradeID, schoolYearID any
		if body.SubjectID != 0 {
			subjectID = body.SubjectID
		}
		if body.GradeID != 0 {
			gradeID = body.GradeID
		}
		if body.SchoolYearID != 0 {
			schoolYearID = body.SchoolYearID
		}

		res, err := db.ExecContext(r.Context(),
			`INSERT INTO parent_access (slug, password_hash, password_plain, student_id, subject_id, grade_id, school_year_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			slug, string(hash), password, body.StudentID, subjectID, gradeID, schoolYearID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()

		// Audit log
		db.ExecContext(r.Context(),
			`INSERT INTO audit_log (actor_type, actor_id, action, detail)
			 VALUES ('teacher', ?, 'parent_access.create', ?)`,
			teacherID,
			fmt.Sprintf(`{"parent_access_id":%d,"student_id":%d,"slug":"%s"}`, id, body.StudentID, slug))

		writeJSON(w, http.StatusOK, map[string]any{
			"id":             id,
			"slug":           slug,
			"password":       password, // returned ONCE so teacher can give it to parent
			"student_id":     body.StudentID,
			"url":            "/z/" + slug,
		})
	}
}

// regeneratedAccess is the result of replacing one parent_access row.
type regeneratedAccess struct {
	NewID     int64
	Slug      string
	Password  string
	StudentID int64
}

// regenerateAccessCode revokes exactly one existing parent_access row (by
// id) and creates a fresh replacement with a newly generated slug+password,
// same student and scoping. Scoped strictly by primary key — touches no
// other row, no other table.
func regenerateAccessCode(ctx context.Context, db *sql.DB, teacherID, oldID int64) (regeneratedAccess, error) {
	var (
		studentID                      int64
		subjectID, gradeID, schoolYear sql.NullInt64
	)
	err := db.QueryRowContext(ctx,
		`SELECT student_id, subject_id, grade_id, school_year_id
		 FROM parent_access WHERE id = ? AND revoked_at IS NULL`, oldID).
		Scan(&studentID, &subjectID, &gradeID, &schoolYear)
	if err != nil {
		return regeneratedAccess{}, err
	}

	// Generate unique slug for the replacement.
	var slug, password string
	var slugTaken bool
	for i := 0; i < 10; i++ {
		slug, password = generateWordPair()
		serr := db.QueryRowContext(ctx,
			`SELECT 1 FROM parent_access WHERE slug = ? AND revoked_at IS NULL`, slug).Scan(&slugTaken)
		if serr == sql.ErrNoRows {
			slugTaken = false
			break
		}
	}
	if slugTaken {
		return regeneratedAccess{}, fmt.Errorf("could not generate unique slug")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return regeneratedAccess{}, fmt.Errorf("hash failed: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return regeneratedAccess{}, err
	}
	defer tx.Rollback()

	// Revoke exactly this one row — scoped by primary key, nothing else.
	res, err := tx.ExecContext(ctx,
		`UPDATE parent_access SET revoked_at = datetime('now') WHERE id = ? AND revoked_at IS NULL`, oldID)
	if err != nil {
		return regeneratedAccess{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return regeneratedAccess{}, fmt.Errorf("access code was already revoked")
	}

	insertRes, err := tx.ExecContext(ctx,
		`INSERT INTO parent_access (slug, password_hash, password_plain, student_id, subject_id, grade_id, school_year_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		slug, string(hash), password, studentID, subjectID, gradeID, schoolYear)
	if err != nil {
		return regeneratedAccess{}, err
	}
	newID, _ := insertRes.LastInsertId()

	if err := tx.Commit(); err != nil {
		return regeneratedAccess{}, err
	}

	db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_type, actor_id, action, detail)
		 VALUES ('teacher', ?, 'parent_access.regenerate', ?)`,
		teacherID,
		fmt.Sprintf(`{"old_parent_access_id":%d,"new_parent_access_id":%d,"student_id":%d,"slug":"%s"}`,
			oldID, newID, studentID, slug))

	return regeneratedAccess{NewID: newID, Slug: slug, Password: password, StudentID: studentID}, nil
}

// parentAccessRegenerateHandler revokes exactly one existing parent_access
// row (by id, from the URL path) and creates a fresh replacement. Never
// touches any other access code.
// POST /api/parent/access/{id}/regenerate
func parentAccessRegenerateHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		teacherID := teacherIDFromContext(r)
		if teacherID == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}

		oldID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || oldID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		result, err := regenerateAccessCode(r.Context(), db, teacherID, oldID)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "access code not found or already revoked"})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         result.NewID,
			"slug":       result.Slug,
			"password":   result.Password,
			"student_id": result.StudentID,
			"url":        "/z/" + result.Slug,
		})
	}
}

// parentAccessRegenerateAllHandler revokes every currently-active
// parent_access row and creates a fresh replacement for each — used for a
// one-time bulk cleanup (e.g. after tightening the wordlist filter). Only
// ever touches the parent_access table, one row (by id) at a time; no other
// data is read or written.
// POST /api/parent/access/regenerate-all
func parentAccessRegenerateAllHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		teacherID := teacherIDFromContext(r)
		if teacherID == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}

		rows, err := db.QueryContext(r.Context(), `SELECT id FROM parent_access WHERE revoked_at IS NULL`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()

		type failure struct {
			ID    int64  `json:"id"`
			Error string `json:"error"`
		}
		var regenerated int
		var failed []failure
		for _, id := range ids {
			if _, err := regenerateAccessCode(r.Context(), db, teacherID, id); err != nil {
				failed = append(failed, failure{ID: id, Error: err.Error()})
				continue
			}
			regenerated++
		}
		if failed == nil {
			failed = []failure{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"regenerated": regenerated,
			"failed":      failed,
		})
	}
}

// parentVerifyHandler verifies slug+password and returns a session token.
// POST /api/parent/verify  {"slug":"jarnivanek", "password":"zelenatrave"}
// → {"token":"...", "student_id":1, "subject_id":2, ...}
func parentVerifyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var body struct {
			Slug     string `json:"slug"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil || body.Slug == "" || body.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and password required"})
			return
		}

		var (
			id                              int64
			passwordHash                    string
			studentID                       int64
			subjectID                       sql.NullInt64
			gradeID                         sql.NullInt64
			schoolYearID                    sql.NullInt64
		)
		err := db.QueryRowContext(r.Context(),
			`SELECT id, password_hash, student_id, subject_id, grade_id, school_year_id
			 FROM parent_access
			 WHERE slug = ? AND revoked_at IS NULL`, body.Slug).
			Scan(&id, &passwordHash, &studentID, &subjectID, &gradeID, &schoolYearID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "neplatný přístup"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "neplatné heslo"})
			return
		}

		// Generate session token
		token := genToken()
		parentSessionsMu.Lock()
		parentSessions[token] = id
		parentSessionsMu.Unlock()

		resp := map[string]any{
			"token":      token,
			"student_id": studentID,
		}
		if subjectID.Valid {
			resp["subject_id"] = subjectID.Int64
		}
		if gradeID.Valid {
			resp["grade_id"] = gradeID.Int64
		}
		if schoolYearID.Valid {
			resp["school_year_id"] = schoolYearID.Int64
		}

		// Audit log
		db.ExecContext(r.Context(),
			`INSERT INTO audit_log (actor_type, actor_id, action, detail)
			 VALUES ('parent', ?, 'parent.verify', ?)`,
			id, fmt.Sprintf(`{"slug":"%s"}`, body.Slug))

		writeJSON(w, http.StatusOK, resp)
	}
}

// parentEvaluationsHandler returns anonymized evaluations for the parent's kid.
// GET /api/parent/evaluations?token=xxx
// No student names, no teacher names — just criteria + levels + descriptions.
func parentEvaluationsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token required"})
			return
		}

		parentSessionsMu.RLock()
		accessID, ok := parentSessions[token]
		parentSessionsMu.RUnlock()
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
			return
		}

		// Get the parent_access record
		var (
			studentID    int64
			subjectID    sql.NullInt64
			gradeID      sql.NullInt64
			schoolYearID sql.NullInt64
		)
		err := db.QueryRowContext(r.Context(),
			`SELECT student_id, subject_id, grade_id, school_year_id
			 FROM parent_access WHERE id = ? AND revoked_at IS NULL`, accessID).
			Scan(&studentID, &subjectID, &gradeID, &schoolYearID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "access revoked"})
			return
		}

		// Get evaluations — anonymized (no student name, no teacher name)
		query := `SELECT e.criterion_id, c.code, c.name, c.category, c.subcategory,
		                 cl.level, cl.letter, cl.label, cl.description,
		                 e.set_at, e.note
		          FROM evaluation e
		          JOIN criterion c ON e.criterion_id = c.id
		          LEFT JOIN criterion_level cl ON cl.criterion_id = e.criterion_id AND cl.level = e.level
		          WHERE e.student_id = ? AND e.deleted_at IS NULL`
		args := []any{studentID}

		if subjectID.Valid && gradeID.Valid {
			query += ` AND c.subject_id = ? AND c.grade_id = ?`
			args = append(args, subjectID.Int64, gradeID.Int64)
		}
		query += ` ORDER BY c.sort_order, e.set_at DESC`

		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type evalEntry struct {
			Level       int    `json:"level"`
			Letter      string `json:"letter"`
			Label       string `json:"label"`
			Description string `json:"description"`
			SetAt       string `json:"set_at"`
			Note        string `json:"note"`
		}
		type criterionResult struct {
			CriterionID int64       `json:"criterion_id"`
			Code        string      `json:"code"`
			Name        string      `json:"name"`
			Category    string      `json:"category"`
			Subcategory string      `json:"subcategory"`
			Current     *evalEntry  `json:"current"`
			History     []evalEntry `json:"history"`
		}

		byCriterion := map[int64]*criterionResult{}
		var ordered []*criterionResult

		for rows.Next() {
			var (
				critID                                        int64
				code, name, category, subcategory             string
				setAt, note                                   string
			)
			// level/letter/label/desc may be NULL if level desc missing
			var nullLevel sql.NullInt64
			var nullLetter, nullLabel, nullDesc sql.NullString
			rows.Scan(&critID, &code, &name, &category, &subcategory,
				&nullLevel, &nullLetter, &nullLabel, &nullDesc,
				&setAt, &note)

			cr, ok := byCriterion[critID]
			if !ok {
				cr = &criterionResult{
					CriterionID: critID, Code: code, Name: name,
					Category: category, Subcategory: subcategory,
					History: []evalEntry{},
				}
				byCriterion[critID] = cr
				ordered = append(ordered, cr)
			}

			entry := evalEntry{
				Level:       int(nullLevel.Int64),
				Letter:      nullLetter.String,
				Label:       nullLabel.String,
				Description: nullDesc.String,
				SetAt:       setAt,
				Note:        note,
			}

			if cr.Current == nil {
				cr.Current = &entry
			} else {
				cr.History = append(cr.History, entry)
			}
		}

		// Also return the criteria for this subject+grade so the parent can see
		// ALL criteria, even those without evaluations yet
		if subjectID.Valid && gradeID.Valid {
			critRows, err := db.QueryContext(r.Context(),
				`SELECT id, code, name, category, subcategory
				 FROM criterion
				 WHERE subject_id = ? AND grade_id = ?
				 ORDER BY sort_order`, subjectID.Int64, gradeID.Int64)
			if err == nil {
				for critRows.Next() {
					var cid int64
					var code, name, cat, subcat string
					critRows.Scan(&cid, &code, &name, &cat, &subcat)
					if _, ok := byCriterion[cid]; !ok {
						cr := &criterionResult{
							CriterionID: cid, Code: code, Name: name,
							Category: cat, Subcategory: subcat,
							History: []evalEntry{},
						}
						ordered = append(ordered, cr)
					}
				}
				critRows.Close()
			}
		}

		if ordered == nil {
			writeJSON(w, http.StatusOK, map[string]any{"evaluations": []any{}})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"evaluations": ordered,
		})
	}
}

func genToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// parentAccessListHandler lists all parent access codes for admin/teacher.
// GET /api/parent/access?student_id=1
func parentAccessListHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		studentIDStr := r.URL.Query().Get("student_id")
		if studentIDStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student_id required"})
			return
		}
		studentID, _ := strconv.ParseInt(studentIDStr, 10, 64)

		rows, err := db.QueryContext(r.Context(),
			`SELECT pa.id, pa.slug, pa.student_id, pa.created_at, pa.revoked_at,
			        s.code, s.name, g.level, sy.label
			 FROM parent_access pa
			 LEFT JOIN subject s ON pa.subject_id = s.id
			 LEFT JOIN grade g ON pa.grade_id = g.id
			 LEFT JOIN school_year sy ON pa.school_year_id = sy.id
			 WHERE pa.student_id = ?
			 ORDER BY pa.created_at DESC`, studentID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type access struct {
			ID          int64  `json:"id"`
			Slug        string `json:"slug"`
			StudentID   int64  `json:"student_id"`
			CreatedAt   string `json:"created_at"`
			RevokedAt   string `json:"revoked_at"`
			SubjectCode string `json:"subject_code"`
			SubjectName string `json:"subject_name"`
			GradeLevel  int    `json:"grade_level"`
			SchoolYear  string `json:"school_year"`
			URL         string `json:"url"`
		}

		var items []access
		for rows.Next() {
			var a access
			var revokedAt sql.NullString
			var subjCode, subjName, schoolYear sql.NullString
			var gradeLevel sql.NullInt64
			rows.Scan(&a.ID, &a.Slug, &a.StudentID, &a.CreatedAt, &revokedAt,
				&subjCode, &subjName, &gradeLevel, &schoolYear)
			a.RevokedAt = revokedAt.String
			a.SubjectCode = subjCode.String
			a.SubjectName = subjName.String
			a.GradeLevel = int(gradeLevel.Int64)
			a.SchoolYear = schoolYear.String
			a.URL = "/z/" + a.Slug
			items = append(items, a)
		}
		if items == nil {
			items = []access{}
		}
		writeJSON(w, http.StatusOK, items)
	}
}

// Ensure json import is used
var _ = json.Marshal
