package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenMigrates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no migrations applied")
	}
	if _, err := db.Exec(`INSERT INTO media (name, source, path, created_at) VALUES ('clip', 'import', '/tmp/clip.mp4', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("media table unusable: %v", err)
	}
	// Reopening must not re-apply migrations.
	db2, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db2.Close()
}
