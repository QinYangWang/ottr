package storage

import (
	"path/filepath"
	"testing"
)

func TestAllTablesExist(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		got[n] = true
	}
	for _, want := range []string{"media", "transcript", "segment", "translation", "translation_segment", "summary", "job", "settings", "schema_migrations"} {
		if !got[want] {
			t.Errorf("missing table %q (have %v)", want, got)
		}
	}
}
