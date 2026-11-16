package authprivacy

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Parallel feature branches reserve adjacent migration numbers. Goose applies
// only versions above the ledger, so a duplicated number would silently leave
// one branch's schema unreachable on databases migrated past it. Fail at merge
// time instead of at deployment. Historical gaps (0032 is intentionally
// absent) are allowed; only unique, parseable version prefixes are required.
func TestMigrationVersionsAreUnique(t *testing.T) {
	files, e := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if e != nil || len(files) == 0 {
		t.Fatal("migration files absent", e)
	}
	byVersion := make(map[int]string, len(files))
	for _, file := range files {
		name := filepath.Base(file)
		prefix, _, ok := strings.Cut(name, "_")
		version, e := strconv.Atoi(prefix)
		if !ok || e != nil || version <= 0 {
			t.Fatalf("migration %s has no numeric version prefix", name)
		}
		if first, exists := byVersion[version]; exists {
			t.Fatalf("migration version %d is used twice: %s and %s", version, first, name)
		}
		byVersion[version] = name
	}
}
