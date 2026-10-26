package library

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ottr/internal/storage"
)

func openTest(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &Service{}
	s.Open(db)
	return s, db
}

func insert(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO media (name, source, path, duration_ms, created_at)
		VALUES (?, 'import', '/tmp/' || ?, 61000, ?)`, name, name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestListAndSearch(t *testing.T) {
	s, db := openTest(t)
	insert(t, db, "standup")
	insert(t, db, "demo")

	all, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}
	if all[0].Name != "demo" {
		t.Errorf("want newest first, got %q", all[0].Name)
	}
	if all[0].DurationMs != 61000 {
		t.Errorf("duration lost: %d", all[0].DurationMs)
	}

	got, err := s.List("stand")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "standup" {
		t.Fatalf("search: %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	s, _ := openTest(t)
	if _, err := s.Get(42); err == nil {
		t.Fatal("want error")
	}
}

// Files in the recordings folder that no record holds are reported, and can be
// deleted without touching the library's rows.
func TestOrphansAndDeleteFiles(t *testing.T) {
	s, db := openTest(t)
	dir := t.TempDir()
	also := filepath.Join(dir, "nested")
	if err := os.MkdirAll(also, 0o755); err != nil {
		t.Fatal(err)
	}

	kept := filepath.Join(dir, "kept.mp4")
	orphan := filepath.Join(dir, "orphan.mp4")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{kept, orphan, other} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO media (name, source, path, created_at)
		VALUES ('kept', 'recording', ?, ?)`, kept, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	orphans, err := s.Orphans(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0] != orphan {
		t.Fatalf("orphans: %v (want just %s)", orphans, orphan)
	}

	if err := s.DeleteFiles(orphans); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("the orphan file is still there: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("a file the library holds was deleted: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("a file Ottr did not write was deleted: %v", err)
	}
	if orphans, err = s.Orphans(dir); err != nil || len(orphans) != 0 {
		t.Errorf("orphans after deleting them: %v, %v", orphans, err)
	}

	// A missing folder is no error: nothing was recorded yet.
	if orphans, err := s.Orphans(filepath.Join(dir, "absent")); err != nil || len(orphans) != 0 {
		t.Errorf("missing folder: %v, %v", orphans, err)
	}
	if orphans, err := s.Orphans(""); err != nil || len(orphans) != 0 {
		t.Errorf("no folder: %v, %v", orphans, err)
	}
}
