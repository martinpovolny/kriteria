package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/martinpovolny/kriteria/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dbPath := flag.String("db", "data/kriteria.db", "path to SQLite database file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	db := st.DB()

	// 1. Create a school year
	var yearID int64
	err = db.QueryRowContext(ctx,
		`INSERT OR IGNORE INTO school_year (label) VALUES ('2025/2026')
		 RETURNING id`).Scan(&yearID)
	if err != nil {
		db.QueryRowContext(ctx, `SELECT id FROM school_year WHERE label='2025/2026'`).Scan(&yearID)
	}
	fmt.Println("school year:", yearID)

	// 2. Create test students
	names := []string{"Anna Nováková", "Tomáš Dvořák", "Ema Černá", "Jakub Procházka",
		"Lída Horáková", "Matěj Kratochvíl", "Tereza Bílá", "Filip Veselý"}
	studentIDs := make([]int64, 0, len(names))
	for _, name := range names {
		var id int64
		err := db.QueryRowContext(ctx,
			`INSERT INTO student (display_name) VALUES (?)
			 RETURNING id`, name).Scan(&id)
		if err != nil {
			db.QueryRowContext(ctx, `SELECT id FROM student WHERE display_name=?`, name).Scan(&id)
		}
		studentIDs = append(studentIDs, id)
	}
	fmt.Println("students:", len(studentIDs))

	// 3. Enroll students in different grades so the UI shows different lists:
	//    Grade 1: Anna, Tomáš, Ema, Jakub  (Mat 1 + AJ 1 + CJ 1)
	//    Grade 2: Lída, Matěj              (Mat 2 + AJ 2)
	//    Grade 3: Tereza, Filip            (Mat 3 + AJ 3)
	type enrollmentSpec struct {
		studentIdx int
		subject    string
		grade      int
	}
	specs := []enrollmentSpec{
		// Grade 1
		{0, "Mat", 1}, {1, "Mat", 1}, {2, "Mat", 1}, {3, "Mat", 1},
		{0, "AJ", 1}, {1, "AJ", 1}, {2, "AJ", 1}, {3, "AJ", 1},
		{0, "CJ", 1}, {1, "CJ", 1}, {2, "CJ", 1}, {3, "CJ", 1},
		// Grade 2
		{4, "Mat", 2}, {5, "Mat", 2},
		{4, "AJ", 2}, {5, "AJ", 2},
		// Grade 3
		{6, "Mat", 3}, {7, "Mat", 3},
		{6, "AJ", 3}, {7, "AJ", 3},
	}

	for _, spec := range specs {
		if spec.studentIdx >= len(studentIDs) {
			continue
		}
		var subjectID, gradeID int64
		db.QueryRowContext(ctx, `SELECT id FROM subject WHERE code=?`, spec.subject).Scan(&subjectID)
		db.QueryRowContext(ctx, `SELECT id FROM grade WHERE level=?`, spec.grade).Scan(&gradeID)
		db.ExecContext(ctx,
			`INSERT OR IGNORE INTO enrollment (student_id, subject_id, grade_id, school_year_id)
			 VALUES (?, ?, ?, ?)`,
			studentIDs[spec.studentIdx], subjectID, gradeID, yearID)
	}
	fmt.Println("enrolled: 4 in grade 1, 2 in grade 2, 2 in grade 3")

	// 4. Get dev teacher (with role)
	var teacherID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO teacher (oauth_subject, email, display_name, role)
		 VALUES ('dev:local', 'dev@localhost', 'Učitel (dev)', 'teacher')
		 ON CONFLICT(oauth_subject) DO UPDATE SET email=excluded.email
		 RETURNING id`).Scan(&teacherID)
	if err != nil {
		db.QueryRowContext(ctx, `SELECT id FROM teacher WHERE oauth_subject='dev:local'`).Scan(&teacherID)
	}
	// Add a director
	db.ExecContext(ctx,
		`INSERT OR IGNORE INTO teacher (oauth_subject, email, display_name, role)
		 VALUES ('dev:director', 'director@localhost', 'Ředitel (dev)', 'director')`)
	fmt.Println("teacher:", teacherID, "(teacher role)")
	fmt.Println("director: created (dev:director)")

	// 5. Add some sample evaluations for 3 students in Mat grade 1
	var matSubjectID, matGradeID int64
	db.QueryRowContext(ctx, `SELECT id FROM subject WHERE code='Mat'`).Scan(&matSubjectID)
	db.QueryRowContext(ctx, `SELECT id FROM grade WHERE level=1`).Scan(&matGradeID)

	// Get first 5 criteria for Mat grade 1
	rows, err := db.QueryContext(ctx,
		`SELECT id FROM criterion WHERE subject_id=? AND grade_id=? ORDER BY sort_order LIMIT 5`,
		matSubjectID, matGradeID)
	if err != nil {
		logger.Error("query criteria", "err", err)
		os.Exit(1)
	}
	var critIDs []int64
	for rows.Next() {
		var cid int64
		rows.Scan(&cid)
		critIDs = append(critIDs, cid)
	}
	rows.Close()

	// Add evaluations for first 3 students
	for i := 0; i < 3 && i < len(studentIDs); i++ {
		for j, cid := range critIDs {
			// Random level 2-4, with first student getting higher levels
			level := rand.Intn(3) + 2 // 2..4
			if i == 0 {
				level = 4 // Anna gets all Ú
			}
			db.ExecContext(ctx,
				`INSERT INTO evaluation (student_id, criterion_id, teacher_id, level, note)
				 VALUES (?, ?, ?, ?, ?)`,
				studentIDs[i], cid, teacherID, level, "")
			_ = j
		}
	}
	fmt.Println("sample evaluations added for 3 students")

	// 5b. Historical evaluations for Anna Nováková (index 0) in Mat grade 1
	// This creates a progression over time so the timeline shows arrows/history.
	now := time.Now()
	type histEval struct {
		criterionIdx int
		level        int
		monthsAgo    int
		note         string
	}
	history := []histEval{
		// 3 months ago — mostly J (1) or Č (2)
		{0, 1, 3, ""}, {1, 2, 3, ""}, {2, 1, 3, ""}, {3, 2, 3, ""}, {4, 1, 3, ""},
		// 2 months ago — mostly Č (2) or T (3)
		{0, 2, 2, ""}, {1, 3, 2, ""}, {2, 2, 2, ""}, {3, 3, 2, ""}, {4, 2, 2, ""},
		// 1 month ago — mostly T (3), one still Č
		{0, 3, 1, ""}, {1, 4, 1, ""}, {2, 3, 1, ""}, {3, 4, 1, ""}, {4, 3, 1, ""},
	}
	for _, h := range history {
		dt := now.AddDate(0, -h.monthsAgo, 0).Format("2006-01-02 10:00:00")
		db.ExecContext(ctx,
			`INSERT INTO evaluation (student_id, criterion_id, teacher_id, level, note, set_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			studentIDs[0], critIDs[h.criterionIdx], teacherID, h.level, h.note, dt)
	}
	fmt.Println("historical evaluations for Anna: 15 entries (3 months of progress)")

	// 6. Generate parent access codes for ALL students (with plaintext for printing)
	for _, sid := range studentIDs {
		// Generate a simple slug from the student name for testing
		var name string
		db.QueryRowContext(ctx, `SELECT display_name FROM student WHERE id=?`, sid).Scan(&name)
		// Use the API's word generator by inserting directly with random words
		slug := genSimpleSlug(name)
		password := genSimplePassword()
		hash, _ := bcryptGenerate(password)
		db.ExecContext(ctx,
			`INSERT OR IGNORE INTO parent_access (slug, password_hash, password_plain, student_id)
			 VALUES (?, ?, ?, ?)`,
			slug, hash, password, sid)
	}
	fmt.Println("parent access codes generated for all students")
	// Print a sample
	var firstSlug, firstPw string
	db.QueryRowContext(ctx,
		`SELECT slug, password_plain FROM parent_access WHERE student_id=?`, studentIDs[0]).Scan(&firstSlug, &firstPw)
	fmt.Printf("sample: /z/%s password: %s\n", firstSlug, firstPw)

	fmt.Println("\nDone! Run: go run ./cmd/kriteria")
}

func bcryptGenerate(s string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(s), bcrypt.DefaultCost)
	return string(hash), err
}

func genSimpleSlug(name string) string {
	// Simple: take first 5 chars of first name, lowercase, no diacritics
	s := strings.ToLower(strings.Fields(name)[0])
	s = strings.Map(func(r rune) rune {
		switch r {
		case 'á': return 'a'; case 'é': return 'e'; case 'í': return 'i'
		case 'ó': return 'o'; case 'ú': return 'u'; case 'ů': return 'u'
		case 'ý': return 'y'; case 'ř': return 'r'; case 'š': return 's'
		case 'č': return 'c'; case 'ž': return 'z'; case 'ň': return 'n'
		case 'ě': return 'e'; case 'ť': return 't'; case 'ď': return 'd'
		}
		return r
	}, s)
	if len(s) > 5 { s = s[:5] }
	return s + "test"
}

func genSimplePassword() string {
	words := []string{"les", "hora", "reka", "most", "hrad", "strom", "kvet", "list", "kmen", "pupen"}
	adj := []string{"jarni", "letni", "zimni", "stary", "novy", "maly", "velky", "cisty"}
	rand.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
	rand.Shuffle(len(adj), func(i, j int) { adj[i], adj[j] = adj[j], adj[i] })
	return adj[0] + words[0]
}
