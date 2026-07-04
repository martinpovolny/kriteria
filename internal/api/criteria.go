package api

import (
	"net/http"
	"os"
)

// criteriaHandler serves the JSON file produced by the importer (data/kriteria.json).
// This way the overview page works without a database — you review the JSON,
// fix the source .docx files, re-run the importer, and only load to DB once clean.
func criteriaHandler(jsonPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "kriteria.json not found. Run: go run ./cmd/importer",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}
