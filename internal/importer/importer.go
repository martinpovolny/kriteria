// Package importer parses .docx criteria and rubric files into a JSON file
// for review. With the Load flag set, it also inserts into the SQLite database.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/martinpovolny/kriteria/internal/docx"
)

// Config controls what the importer does.
type Config struct {
	DataDir  string // path to data/ (containing kriteria/)
	JSONPath string // where to write the JSON file (e.g. data/kriteria.json)
	Load     bool   // if true, also insert into the database
}

// Run walks the data directory, parses all kriteria and rubriky .docx files,
// writes the result as JSON, and optionally inserts into the database.
func Run(ctx context.Context, db *sql.DB, cfg Config, logger *slog.Logger) (*Stats, error) {
	imp := &importer{db: db, logger: logger, cfg: cfg}
	return imp.run(ctx)
}

// Stats summarises the import results.
type Stats struct {
	Subjects     int
	Grades       int
	Criteria     int
	LevelDescs   int
	FilesParsed  int
	FilesSkipped int
	MismatchLog  []string
}

type importer struct {
	db     *sql.DB
	logger *slog.Logger
	cfg    Config
	stats  Stats
}

func (imp *importer) run(ctx context.Context) (*Stats, error) {
	kriteriaRoot := filepath.Join(imp.cfg.DataDir, "kriteria")

	// Phase 1: parse kriteria files → build in-memory model
	subjects, err := imp.parseKriteriaFiles(ctx, kriteriaRoot)
	if err != nil {
		return nil, err
	}

	// Phase 2: parse rubriky files → attach level descriptions
	if err := imp.parseRubrikyFiles(ctx, kriteriaRoot, subjects); err != nil {
		return nil, err
	}

	// Phase 3: write JSON (always — this is the review artifact)
	jsonData := buildJSON(subjects)
	if err := imp.writeJSON(jsonData); err != nil {
		return nil, err
	}

	// Count stats from parsed data (not from DB insert)
	imp.stats.Subjects = len(jsonData.Subjects)
	for _, s := range jsonData.Subjects {
		imp.stats.Grades += len(s.Grades)
		for _, g := range s.Grades {
			imp.stats.Criteria += len(g.Criteria)
		}
	}

	imp.logger.Info("wrote JSON", "path", imp.cfg.JSONPath,
		"subjects", imp.stats.Subjects, "criteria", imp.stats.Criteria)

	// Phase 4: load into database (only if requested)
	if imp.cfg.Load {
		if imp.db == nil {
			return nil, fmt.Errorf("--load requested but no database handle")
		}
		if err := imp.insert(ctx, subjects); err != nil {
			return nil, err
		}
	}

	return &imp.stats, nil
}


// ---------------------------------------------------------------------------
// Data model (in-memory)
// ---------------------------------------------------------------------------

type subjectData struct {
	Code     string          // "AJ", "Mat", ...
	Name     string          // "Anglický jazyk"
	Grades   map[int]*gradeData
}

type gradeData struct {
	Level     int                       // 1..5
	Criteria  map[string]*criterionData // keyed by code "K1", "K2", ...
}

type criterionData struct {
	Code        string
	Name        string
	Category    string
	Subcategory string
	OVUCode     string
	SortOrder   int
	Levels      map[int]*levelData // keyed by level 1..4
}

type levelData struct {
	Level       int    // 4=Ú, 3=T, 2=Č, 1=J
	Letter      string // "Ú", "T", "Č", "J"
	Label       string // "úplně osvojeno"
	Description string
}

// ---------------------------------------------------------------------------
// JSON output model
// ---------------------------------------------------------------------------

type jsonSubject struct {
	Code   string     `json:"code"`
	Name   string     `json:"name"`
	Grades []jsonGrade `json:"grades"`
}

type jsonGrade struct {
	Level    int              `json:"level"`
	Criteria []jsonCriterion  `json:"criteria"`
}

type jsonCriterion struct {
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Subcategory string       `json:"subcategory"`
	OVUCode     string       `json:"ovu_code"`
	SortOrder   int          `json:"sort_order"`
	Levels      []jsonLevel  `json:"levels"`
}

type jsonLevel struct {
	Level       int    `json:"level"`
	Letter      string `json:"letter"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type jsonFile struct {
	Subjects []jsonSubject `json:"subjects"`
}

func buildJSON(subjects map[string]*subjectData) jsonFile {
	var out jsonFile
	for _, subj := range subjects {
		js := jsonSubject{Code: subj.Code, Name: subj.Name}
		// Sort grades by level
		gradeLevels := make([]int, 0, len(subj.Grades))
		for gl := range subj.Grades {
			gradeLevels = append(gradeLevels, gl)
		}
		sortInts(gradeLevels)
		for _, gl := range gradeLevels {
			gd := subj.Grades[gl]
			jg := jsonGrade{Level: gd.Level}
			// Criteria sorted by sort_order
			codes := make([]string, 0, len(gd.Criteria))
			for code := range gd.Criteria {
				codes = append(codes, code)
			}
			sortStrings(codes)
			// Actually we need sort by SortOrder, not by code
			// Rebuild by collecting and sorting by sort_order
			var sortedCrit []*criterionData
			for _, cd := range gd.Criteria {
				sortedCrit = append(sortedCrit, cd)
			}
			sortByOrder(sortedCrit)
			for _, cd := range sortedCrit {
				jc := jsonCriterion{
					Code: cd.Code, Name: cd.Name, Category: cd.Category,
					Subcategory: cd.Subcategory, OVUCode: cd.OVUCode,
					SortOrder: cd.SortOrder,
				}
				// Levels sorted descending (Ú=4 first)
				for lv := 4; lv >= 1; lv-- {
					if ld, ok := cd.Levels[lv]; ok {
						jc.Levels = append(jc.Levels, jsonLevel{
							Level: ld.Level, Letter: ld.Letter,
							Label: ld.Label, Description: ld.Description,
						})
					}
				}
				jg.Criteria = append(jg.Criteria, jc)
			}
			js.Grades = append(js.Grades, jg)
		}
		out.Subjects = append(out.Subjects, js)
	}
	return out
}

func (imp *importer) writeJSON(data jsonFile) error {
	f, err := os.Create(imp.cfg.JSONPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", imp.cfg.JSONPath, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(data)
}

// --- small sort helpers (avoid pulling sort package for clarity) ---

func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortByOrder(s []*criterionData) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].SortOrder < s[j-1].SortOrder; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 1: Parse kriteria files
// ---------------------------------------------------------------------------

var (
	reTitleKriteria = regexp.MustCompile(`Kritéria\s+hodnocení\s+–\s+(.+?)\s+–\s+(\d+)\.\s+ročník`)
	reCriterionCode = regexp.MustCompile(`^(K\d+)\s+(.+)$`)
	reOVU           = regexp.MustCompile(`^OVU:\s*(.+)$`)
)

func (imp *importer) parseKriteriaFiles(ctx context.Context, root string) (map[string]*subjectData, error) {
	subjects := make(map[string]*subjectData)

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read data dir %s: %w", root, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subjectCode := entry.Name()
		if subjectCode == "puvodni" {
			continue
		}

		subjectDir := filepath.Join(root, subjectCode)
		files, err := os.ReadDir(subjectDir)
		if err != nil {
			imp.logger.Warn("skip unreadable dir", "dir", subjectDir, "err", err)
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".docx") {
				continue
			}
			nameLower := strings.ToLower(f.Name())
			// Match "kriteria" or "kritéria" (with accent) + "v2"
			if (!strings.Contains(nameLower, "kriteria") && !strings.Contains(nameLower, "kritéria")) || !strings.Contains(nameLower, "v2") {
				continue
			}

			path := filepath.Join(subjectDir, f.Name())
			if _, err := imp.parseOneKriteria(path, subjectCode, subjects); err != nil {
				imp.logger.Warn("parse kriteria file", "file", path, "err", err)
				continue
			}
			imp.stats.FilesParsed++
		}
	}

	return subjects, nil
}

func (imp *importer) parseOneKriteria(path, subjectCode string, subjects map[string]*subjectData) (*subjectData, error) {
	blocks, err := docx.Read(path)
	if err != nil {
		return nil, err
	}

	var subjectName string
	var gradeLevel int

	// Find title in first few Heading1 paragraphs
	for _, b := range blocks {
		if !b.IsParagraph() {
			continue
		}
		p := b.Paragraph
		if p.Style == "Heading1" || (subjectName == "" && p.Text != "") {
			m := reTitleKriteria.FindStringSubmatch(p.Text)
			if m != nil {
				subjectName = m[1]
				fmt.Sscanf(m[2], "%d", &gradeLevel)
				break
			}
		}
	}

	if subjectName == "" || gradeLevel == 0 {
		return nil, fmt.Errorf("could not parse title (subject/grade) from %s", path)
	}

	subj := subjects[subjectCode]
	if subj == nil {
		subj = &subjectData{Code: subjectCode, Name: subjectName, Grades: make(map[int]*gradeData)}
		subjects[subjectCode] = subj
	}
	// Update name in case first encounter was from a file that didn't parse
	if subj.Name == "" {
		subj.Name = subjectName
	}

	gd := subj.Grades[gradeLevel]
	if gd == nil {
		gd = &gradeData{Level: gradeLevel, Criteria: make(map[string]*criterionData)}
		subj.Grades[gradeLevel] = gd
	}

	var category, subcategory string
	var sortOrder int
	var lastCriterion *criterionData

	for _, b := range blocks {
		if !b.IsParagraph() {
			continue
		}
		p := b.Paragraph
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}

		switch p.Style {
		case "Heading2":
			category = text
			subcategory = ""
		case "Heading3":
			subcategory = text
		default:
			// Criterion line: "K1  <text>"
			if m := reCriterionCode.FindStringSubmatch(text); m != nil {
				code := m[1]
				cd := &criterionData{
					Code:        code,
					Name:        strings.TrimSpace(m[2]),
					Category:    category,
					Subcategory: subcategory,
					SortOrder:   sortOrder,
				}
				gd.Criteria[code] = cd
				lastCriterion = cd
				sortOrder++
				continue
			}
			// OVU line
			if m := reOVU.FindStringSubmatch(text); m != nil && lastCriterion != nil {
				lastCriterion.OVUCode = strings.TrimSpace(m[1])
				continue
			}
		}
	}

	imp.logger.Info("parsed kriteria", "file", filepath.Base(path),
		"subject", subjectName, "grade", gradeLevel, "criteria", len(gd.Criteria))

	return subj, nil
}

// ---------------------------------------------------------------------------
// Phase 2: Parse rubriky files
// ---------------------------------------------------------------------------

var (
	reTitleRubrikyGrade = regexp.MustCompile(`(\d+)\.\s+ročník`)
	reRubrikyCriterion  = regexp.MustCompile(`^K(\d+)\s*[–-]\s+(.+)$`)
	reRubrikyCriterion2 = regexp.MustCompile(`^Kritérium\s+(\d+)[\s:]+(.+)$`)
	reZnění             = regexp.MustCompile(`^(?:Znění(?:\s+kritéria)?:|Vaše\s+znění.*?:)\s*(.+)$`)
	reOVURubriky        = regexp.MustCompile(`^(?:OVU|Navázáno\s+na\s+OVU):\s*(.+)$`)
)

func (imp *importer) parseRubrikyFiles(ctx context.Context, root string, subjects map[string]*subjectData) error {
	for _, subj := range subjects {
		subjectDir := filepath.Join(root, subj.Code)
		files, err := os.ReadDir(subjectDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".docx") {
				continue
			}
			nameLower := strings.ToLower(f.Name())
			if !strings.Contains(nameLower, "rubriky") {
				continue
			}

			path := filepath.Join(subjectDir, f.Name())
			if err := imp.parseOneRubriky(path, subj); err != nil {
				imp.logger.Warn("parse rubriky file", "file", path, "err", err)
				continue
			}
			imp.stats.FilesParsed++
		}
	}
	return nil
}

func (imp *importer) parseOneRubriky(path string, subj *subjectData) error {
	blocks, err := docx.Read(path)
	if err != nil {
		return err
	}

	// Find grade from title
	var gradeLevel int
	for _, b := range blocks {
		if !b.IsParagraph() {
			continue
		}
		if m := reTitleRubrikyGrade.FindStringSubmatch(b.Paragraph.Text); m != nil {
			fmt.Sscanf(m[1], "%d", &gradeLevel)
			break
		}
	}
	if gradeLevel == 0 {
		return fmt.Errorf("could not find grade in rubriky title")
	}

	gd := subj.Grades[gradeLevel]
	if gd == nil {
		imp.stats.MismatchLog = append(imp.stats.MismatchLog,
			fmt.Sprintf("rubriky for %s grade %d has no matching kriteria file", subj.Code, gradeLevel))
		return nil
	}

	// Walk blocks, pairing criterion headings with the next table
	var currentCode string
	for _, b := range blocks {
		// Handle table blocks: if we have a current criterion, parse levels
		if b.IsTable() && currentCode != "" {
			levels := parseLevelTable(b.Table)
			if len(levels) > 0 {
				cd := gd.Criteria[currentCode]
				if cd != nil {
					cd.Levels = levels
					imp.stats.LevelDescs += len(levels)
				} else {
					imp.stats.MismatchLog = append(imp.stats.MismatchLog,
						fmt.Sprintf("rubriky %s grade %d: code %s not found in kriteria",
							subj.Code, gradeLevel, currentCode))
				}
			}
			currentCode = ""
			continue
		}

		if !b.IsParagraph() {
			continue
		}
		text := strings.TrimSpace(b.Paragraph.Text)
		if text == "" {
			continue
		}

		// Match criterion heading: "K1 – ..." or "Kritérium 1: ..."
		code := matchCriterionHeading(text)
		if code != "" {
			currentCode = code
			continue
		}
	}

	// Count criteria that are missing level descriptions
	for code, cd := range gd.Criteria {
		if len(cd.Levels) == 0 {
			imp.stats.MismatchLog = append(imp.stats.MismatchLog,
				fmt.Sprintf("no rubriky levels for %s grade %d %s", subj.Code, gradeLevel, code))
		}
	}

	imp.logger.Info("parsed rubriky", "file", filepath.Base(path),
		"subject", subj.Code, "grade", gradeLevel)
	return nil
}

func matchCriterionHeading(text string) string {
	if m := reRubrikyCriterion.FindStringSubmatch(text); m != nil {
		return "K" + m[1]
	}
	if m := reRubrikyCriterion2.FindStringSubmatch(text); m != nil {
		return "K" + m[1]
	}
	// Bare "K1" or "K12" (PV style, heading is just the code)
	if m := regexp.MustCompile(`^K(\d+)$`).FindStringSubmatch(text); m != nil {
		return "K" + m[1]
	}
	return ""
}

// levelLetterToNumber maps J/Č/T/Ú to 1/2/3/4.
var levelLetterToNumber = map[string]int{
	"Ú": 4, "U": 4,
	"T": 3,
	"Č": 2, "C": 2,
	"J": 1,
}

func parseLevelTable(tbl *docx.Table) map[int]*levelData {
	levels := make(map[int]*levelData)

	for _, row := range tbl.Rows {
		if len(row.Cells) < 2 {
			continue
		}
		labelCell := strings.TrimSpace(row.Cells[0].Text)
		descCell := strings.TrimSpace(row.Cells[1].Text)

		// Skip header row
		if strings.Contains(strings.ToLower(labelCell), "úroveň") {
			continue
		}
		// Skip empty rows
		if labelCell == "" && descCell == "" {
			continue
		}

		// Extract the level letter from the first non-space character
		letter := firstNonSpace(labelCell)
		levelNum, ok := levelLetterToNumber[letter]
		if !ok {
			continue
		}

		// Extract the label text: everything after the letter
		// Handle both "Ú – úplně osvoji" and "Ú\núplně osvojeno"
		label := extractLabel(labelCell, letter)

		levels[levelNum] = &levelData{
			Level:       levelNum,
			Letter:      letter,
			Label:       label,
			Description: descCell,
		}
	}

	return levels
}

func firstNonSpace(s string) string {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '–' || r == '-' {
			continue
		}
		return string(r)
	}
	return ""
}

func extractLabel(cellText, letter string) string {
	// Remove the letter and surrounding separators
	s := strings.TrimSpace(cellText)
	s = strings.TrimPrefix(s, letter)
	s = strings.TrimSpace(s)
	// Remove leading dash (both en-dash and hyphen)
	s = strings.TrimPrefix(s, "–")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimSpace(s)
	// If there's a newline, take the second line (Mat style: "Ú\núplně osvojeno")
	if strings.Contains(s, "\n") {
		parts := strings.SplitN(s, "\n", 2)
		second := strings.TrimSpace(parts[1])
		if second != "" {
			return second
		}
		s = strings.TrimSpace(parts[0])
	}
	return s
}

// ---------------------------------------------------------------------------
// Phase 3: Insert into database
// ---------------------------------------------------------------------------

func (imp *importer) insert(ctx context.Context, subjects map[string]*subjectData) error {
	tx, err := imp.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert grades 1-5 (idempotent)
	for g := 1; g <= 5; g++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO grade (level) VALUES (?)`, g); err != nil {
			return fmt.Errorf("insert grade %d: %w", g, err)
		}
	}

	// Preload grade IDs
	gradeIDs := make(map[int]int64)
	for g := 1; g <= 5; g++ {
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM grade WHERE level = ?`, g).Scan(&id); err != nil {
			return fmt.Errorf("select grade %d: %w", g, err)
		}
		gradeIDs[g] = id
	}

	for _, subj := range subjects {
		// Insert subject
		res, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO subject (code, name) VALUES (?, ?)`,
			subj.Code, subj.Name)
		if err != nil {
			return fmt.Errorf("insert subject %s: %w", subj.Code, err)
		}
		var subjectID int64
		if n, _ := res.RowsAffected(); n > 0 {
			subjectID, _ = res.LastInsertId()
		} else {
			if err := tx.QueryRowContext(ctx, `SELECT id FROM subject WHERE code = ?`, subj.Code).Scan(&subjectID); err != nil {
				return fmt.Errorf("select subject %s: %w", subj.Code, err)
			}
		}

		for gradeLevel, gd := range subj.Grades {
			gradeID := gradeIDs[gradeLevel]

			for _, cd := range gd.Criteria {
				// Insert criterion
				res, err := tx.ExecContext(ctx,
					`INSERT OR IGNORE INTO criterion
					 (subject_id, grade_id, code, name, category, subcategory, ovu_code, sort_order)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					subjectID, gradeID, cd.Code, cd.Name,
					cd.Category, cd.Subcategory, cd.OVUCode, cd.SortOrder)
				if err != nil {
					return fmt.Errorf("insert criterion %s %s %s: %w",
						subj.Code, cd.Code, cd.Name, err)
				}

				var critID int64
				if n, _ := res.RowsAffected(); n > 0 {
					critID, _ = res.LastInsertId()
				} else {
					if err := tx.QueryRowContext(ctx,
						`SELECT id FROM criterion WHERE subject_id = ? AND grade_id = ? AND code = ?`,
						subjectID, gradeID, cd.Code).Scan(&critID); err != nil {
						return fmt.Errorf("select criterion: %w", err)
					}
				}

				// Insert level descriptions
				for level, ld := range cd.Levels {
					_, err := tx.ExecContext(ctx,
						`INSERT OR IGNORE INTO criterion_level
						 (criterion_id, level, letter, label, description)
						 VALUES (?, ?, ?, ?, ?)`,
						critID, level, ld.Letter, ld.Label, ld.Description)
					if err != nil {
						return fmt.Errorf("insert level: %w", err)
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
