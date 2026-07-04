package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/martinpovolny/kriteria/internal/importer"
	"github.com/martinpovolny/kriteria/internal/store"
)

func main() {
	dataDir := flag.String("data", "data", "path to the data directory (containing kriteria/)")
	jsonPath := flag.String("json", "data/kriteria.json", "output path for the JSON file")
	dbPath := flag.String("db", "data/kriteria.db", "path to SQLite database file (used with --load)")
	load := flag.Bool("load", false, "also load parsed data into the database")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := context.Background()

	cfg := importer.Config{
		DataDir:  *dataDir,
		JSONPath: *jsonPath,
		Load:     *load,
	}

	var db *sql.DB
	if *load {
		st, err := store.Open(ctx, *dbPath)
		if err != nil {
			logger.Error("failed to open store", "err", err)
			os.Exit(1)
		}
		defer st.Close()
		db = st.DB()
	}

	stats, err := importer.Run(ctx, db, cfg, logger)
	if err != nil {
		logger.Error("import failed", "err", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== Import Summary ===")
	fmt.Printf("Files parsed:       %d\n", stats.FilesParsed)
	fmt.Printf("Subjects:           %d\n", stats.Subjects)
	fmt.Printf("Grades:             %d\n", stats.Grades)
	fmt.Printf("Criteria:           %d\n", stats.Criteria)
	fmt.Printf("Level descriptions: %d\n", stats.LevelDescs)
	fmt.Printf("JSON written to:    %s\n", *jsonPath)
	if *load {
		fmt.Printf("Database loaded:    %s\n", *dbPath)
	}
	if len(stats.MismatchLog) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(stats.MismatchLog))
		for _, m := range stats.MismatchLog {
			fmt.Printf("  ! %s\n", m)
		}
	}
}
