package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
)

// criteriaBySubjectGrade returns criteria with levels for a given subject + grade.
// GET /api/criteria/{subjectCode}/{gradeLevel}
func criteriaBySubjectGrade(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subjectCode := r.PathValue("subjectCode")
		gradeLevelStr := r.PathValue("gradeLevel")
		if subjectCode == "" || gradeLevelStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subject and grade required"})
			return
		}
		gradeLevel, err := strconv.Atoi(gradeLevelStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid grade"})
			return
		}

		// Get subject + grade IDs
		var subjectID, gradeID int64
		err = db.QueryRowContext(r.Context(),
			`SELECT id FROM subject WHERE code = ?`, subjectCode).Scan(&subjectID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subject not found"})
			return
		}
		err = db.QueryRowContext(r.Context(),
			`SELECT id FROM grade WHERE level = ?`, gradeLevel).Scan(&gradeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "grade not found"})
			return
		}

		rows, err := db.QueryContext(r.Context(),
			`SELECT id, code, name, category, subcategory, ovu_code, sort_order
			 FROM criterion
			 WHERE subject_id = ? AND grade_id = ?
			 ORDER BY sort_order`, subjectID, gradeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		// First pass: collect all criteria (close rows before querying levels)
		type level struct {
			Level       int    `json:"level"`
			Letter      string `json:"letter"`
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		type criterion struct {
			ID          int64   `json:"id"`
			Code        string  `json:"code"`
			Name        string  `json:"name"`
			Category    string  `json:"category"`
			Subcategory string  `json:"subcategory"`
			OVUCode     string  `json:"ovu_code"`
			SortOrder   int     `json:"sort_order"`
			Levels      []level `json:"levels"`
		}

		var criteria []criterion
		for rows.Next() {
			var c criterion
			rows.Scan(&c.ID, &c.Code, &c.Name, &c.Category, &c.Subcategory, &c.OVUCode, &c.SortOrder)
			criteria = append(criteria, c)
		}
		rows.Close()

		// Second pass: load levels for each criterion
		for i := range criteria {
			lvRows, err := db.QueryContext(r.Context(),
				`SELECT level, letter, label, description
				 FROM criterion_level
				 WHERE criterion_id = ?
				 ORDER BY level DESC`, criteria[i].ID)
			if err != nil {
				continue
			}
			for lvRows.Next() {
				var lv level
				lvRows.Scan(&lv.Level, &lv.Letter, &lv.Label, &lv.Description)
				criteria[i].Levels = append(criteria[i].Levels, lv)
			}
			lvRows.Close()
		}

		if criteria == nil {
			criteria = []criterion{}
		}
		writeJSON(w, http.StatusOK, criteria)
	}
}

// evaluationsHandler handles getting and setting evaluations.
// GET  /api/evaluations?student_id=1&subject_id=2&grade_id=3
//      → returns current level + history for each criterion
// POST /api/evaluations
//      {"student_id":1, "criterion_id":2, "level":4, "note":"..."}
//      → append-only: creates a new evaluation record
func evaluationsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getEvaluations(db, w, r)
		case http.MethodPost:
			setEvaluation(db, w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func getEvaluations(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	studentIDStr := r.URL.Query().Get("student_id")
	subjectIDStr := r.URL.Query().Get("subject_id")
	gradeIDStr := r.URL.Query().Get("grade_id")
	if studentIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student_id required"})
		return
	}
	studentID, _ := strconv.ParseInt(studentIDStr, 10, 64)

	// Base query: all evaluations for this student, optionally filtered by subject+grade
	query := `SELECT e.id, e.criterion_id, c.code, c.name, c.category, c.subcategory,
	                 e.level, e.teacher_id, t.display_name, e.set_at, e.note
	          FROM evaluation e
	          JOIN criterion c ON e.criterion_id = c.id
	          JOIN teacher t ON e.teacher_id = t.id`
	args := []any{studentID}

	if subjectIDStr != "" && gradeIDStr != "" {
		subjectID, _ := strconv.ParseInt(subjectIDStr, 10, 64)
		gradeID, _ := strconv.ParseInt(gradeIDStr, 10, 64)
		query += ` JOIN criterion c2 ON e.criterion_id = c2.id
		           WHERE e.student_id = ? AND c2.subject_id = ? AND c2.grade_id = ?`
		args = append(args, subjectID, gradeID)
	} else {
		query += ` WHERE e.student_id = ?`
	}
	query += ` ORDER BY c.sort_order, e.set_at DESC`

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	// Group by criterion: latest = current, rest = history
	type histEntry struct {
		Level       int    `json:"level"`
		TeacherName string `json:"teacher_name"`
		SetAt       string `json:"set_at"`
		Note        string `json:"note"`
	}
	type criterionEval struct {
		CriterionID int64        `json:"criterion_id"`
		Code        string       `json:"code"`
		Name        string       `json:"name"`
		Category    string       `json:"category"`
		Subcategory string       `json:"subcategory"`
		Current     *histEntry   `json:"current"`
		History     []histEntry  `json:"history"`
	}

	byCriterion := map[int64]*criterionEval{}
	var ordered []*criterionEval

	for rows.Next() {
		var (
			id                                               int64
			critID                                           int64
			code, name, category, subcategory                string
			level                                            int
			teacherID                                        int64
			teacherName, setAt, note                         string
		)
		rows.Scan(&id, &critID, &code, &name, &category, &subcategory,
			&level, &teacherID, &teacherName, &setAt, &note)

		ce, ok := byCriterion[critID]
		if !ok {
			ce = &criterionEval{
				CriterionID: critID, Code: code, Name: name,
				Category: category, Subcategory: subcategory,
			}
			byCriterion[critID] = ce
			ordered = append(ordered, ce)
		}

		entry := histEntry{
			Level: level, TeacherName: teacherName,
			SetAt: setAt, Note: note,
		}

		if ce.Current == nil {
			ce.Current = &entry
		} else {
			ce.History = append(ce.History, entry)
		}
	}

	if ordered == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, ordered)
}

func setEvaluation(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	teacherID := teacherIDFromContext(r)
	if teacherID == 0 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body struct {
		StudentID   int64  `json:"student_id"`
		CriterionID int64  `json:"criterion_id"`
		Level       int    `json:"level"`
		Note        string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.StudentID == 0 || body.CriterionID == 0 || body.Level < 1 || body.Level > 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student_id, criterion_id, level (1-4) required"})
		return
	}

	res, err := db.ExecContext(r.Context(),
		`INSERT INTO evaluation (student_id, criterion_id, teacher_id, level, note)
		 VALUES (?, ?, ?, ?, ?)`,
		body.StudentID, body.CriterionID, teacherID, body.Level, body.Note)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Log to audit trail
	db.ExecContext(r.Context(),
		`INSERT INTO audit_log (actor_type, actor_id, action, detail)
		 VALUES ('teacher', ?, 'evaluation.set', ?)`,
		teacherID,
		fmt.Sprintf(`{"evaluation_id":%d,"student_id":%d,"criterion_id":%d,"level":%d}`,
			id, body.StudentID, body.CriterionID, body.Level))

	writeJSON(w, http.StatusOK, map[string]any{
		"id":            id,
		"student_id":    body.StudentID,
		"criterion_id":  body.CriterionID,
		"level":         body.Level,
		"note":          body.Note,
		"teacher_id":    teacherID,
	})
}
