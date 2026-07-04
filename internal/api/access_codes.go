package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

// accessCodesHandler returns all active parent access codes with student names
// and plaintext password — for the director to print and distribute.
// NOTE: passwords are only known at generation time (stored as bcrypt hash).
// This endpoint returns the slug+url only; if the password was not captured
// at creation, it cannot be recovered. For codes generated via the API
// (createStudent / parent/access), the password is returned once.
//
// GET /api/access-codes
func accessCodesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT pa.id, pa.slug, pa.password_plain, pa.student_id, s.display_name,
			        pa.created_at, pa.revoked_at,
			        sub.code, sub.name, g.level, sy.label
			 FROM parent_access pa
			 JOIN student s ON pa.student_id = s.id
			 LEFT JOIN subject sub ON pa.subject_id = sub.id
			 LEFT JOIN grade g ON pa.grade_id = g.id
			 LEFT JOIN school_year sy ON pa.school_year_id = sy.id
			 WHERE pa.revoked_at IS NULL
			 ORDER BY s.display_name, pa.created_at`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type code struct {
			ID          int64  `json:"id"`
			Slug        string `json:"slug"`
			Password    string `json:"password"`
			URL         string `json:"url"`
			StudentID   int64  `json:"student_id"`
			StudentName string `json:"student_name"`
			CreatedAt   string `json:"created_at"`
			RevokedAt   string `json:"revoked_at"`
			Active      bool   `json:"active"`
			SubjectCode string `json:"subject_code"`
			SubjectName string `json:"subject_name"`
			GradeLevel  int    `json:"grade_level"`
			SchoolYear  string `json:"school_year"`
			Scope       string `json:"scope"`
		}

		var codes []code
		for rows.Next() {
			var c code
			var revokedAt sql.NullString
			var subjCode, subjName, schoolYear, password sql.NullString
			var gradeLevel sql.NullInt64
			rows.Scan(&c.ID, &c.Slug, &password, &c.StudentID, &c.StudentName,
				&c.CreatedAt, &revokedAt,
				&subjCode, &subjName, &gradeLevel, &schoolYear)
			c.Password = password.String
			c.URL = "/z/" + c.Slug
			c.RevokedAt = revokedAt.String
			c.Active = revokedAt.String == ""
			c.SubjectCode = subjCode.String
			c.SubjectName = subjName.String
			c.GradeLevel = int(gradeLevel.Int64)
			c.SchoolYear = schoolYear.String
			if c.SubjectCode != "" {
				c.Scope = c.SubjectCode + " " + strconv.Itoa(c.GradeLevel) + ".r"
				if c.SchoolYear != "" {
					c.Scope += " " + c.SchoolYear
				}
			} else {
				c.Scope = "vše"
			}
			codes = append(codes, c)
		}
		if codes == nil {
			codes = []code{}
		}
		writeJSON(w, http.StatusOK, codes)
	}
}

// accessCodesByClassHandler returns access codes grouped by grade (class).
// Each student appears once (with their most recent active access code),
// grouped by their enrolled grade. For the director's print page.
// GET /api/access-codes-by-class
func accessCodesByClassHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT s.id, s.display_name,
			        MAX(e.grade_id) as grade_id,
			        MAX(g.level) as grade_level,
			        pa.slug, pa.password_plain
			 FROM student s
			 JOIN enrollment e ON e.student_id = s.id
			 JOIN grade g ON e.grade_id = g.id
			 LEFT JOIN parent_access pa ON pa.student_id = s.id AND pa.revoked_at IS NULL
			 GROUP BY s.id
			 ORDER BY grade_level, s.display_name`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type studentAccess struct {
			StudentID   int64  `json:"student_id"`
			StudentName string `json:"student_name"`
			GradeLevel  int    `json:"grade_level"`
			Slug        string `json:"slug"`
			Password    string `json:"password"`
			URL         string `json:"url"`
		}

		byGrade := map[int][]studentAccess{}
		var gradeOrder []int

		for rows.Next() {
			var sa studentAccess
			var slug, password sql.NullString
			var gradeID sql.NullInt64
			var gradeLevel sql.NullInt64
			rows.Scan(&sa.StudentID, &sa.StudentName, &gradeID, &gradeLevel, &slug, &password)
			sa.GradeLevel = int(gradeLevel.Int64)
			sa.Slug = slug.String
			sa.Password = password.String
			if sa.Slug != "" {
				sa.URL = "/z/" + sa.Slug
			}

			if _, exists := byGrade[sa.GradeLevel]; !exists {
				gradeOrder = append(gradeOrder, sa.GradeLevel)
			}
			byGrade[sa.GradeLevel] = append(byGrade[sa.GradeLevel], sa)
		}

		// Sort grade order
		for i := 1; i < len(gradeOrder); i++ {
			for j := i; j > 0 && gradeOrder[j] < gradeOrder[j-1]; j-- {
				gradeOrder[j], gradeOrder[j-1] = gradeOrder[j-1], gradeOrder[j]
			}
		}

		type classGroup struct {
			GradeLevel int             `json:"grade_level"`
			Students   []studentAccess `json:"students"`
		}

		var groups []classGroup
		for _, g := range gradeOrder {
			students := byGrade[g]
			if students == nil {
				students = []studentAccess{}
			}
			groups = append(groups, classGroup{GradeLevel: g, Students: students})
		}
		if groups == nil {
			groups = []classGroup{}
		}

		writeJSON(w, http.StatusOK, groups)
	}
}
