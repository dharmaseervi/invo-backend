package database

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(dsn string) {

	path, _ := filepath.Abs("migrations") // ✅ ensures full absolute path
	sourceURL := fmt.Sprintf("file://%s", path)

	log.Println("🔄 Running migrations from:", sourceURL)

	m, err := migrate.New(
		"file://migrations", // path relative to project root
		dsn,
	)
	if err != nil {
		log.Fatalf("Migration setup failed: %v", err)
	}

	if err := m.Up(); err != nil && err.Error() != "no change" {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("✅ Database migrated successfully!")
}
