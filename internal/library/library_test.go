package library

import (
	"database/sql"
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
