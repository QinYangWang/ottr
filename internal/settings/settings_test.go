package settings

import (
	"path/filepath"
	"testing"

	"ottr/internal/storage"
)

func TestGetAndSet(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := &Service{}
	s.Open(db)
	if got := s.Get(Theme); got != "" {
		t.Errorf("a key never set reads %q", got)
	}
	if err := s.Set(Theme, "dark"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(Theme); got != "dark" {
		t.Errorf("stored %q, read %q", "dark", got)
	}
	// Setting the same key again replaces it rather than failing.
	if err := s.Set(Theme, "light"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(Theme); got != "light" {
		t.Errorf("replaced key reads %q", got)
	}

	// Before Open there is nothing to read or write.
	empty := &Service{}
	if err := empty.Set(Theme, "dark"); err == nil {
		t.Error("setting a key without a database reported no error")
	}
	if got := empty.Get(Theme); got != "" {
		t.Errorf("reading without a database gave %q", got)
	}
}
