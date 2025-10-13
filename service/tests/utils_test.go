package tests

// Objectives (from docs/tests.md):
// 1. Validate utility helpers: ID generation, slug generation, path splitting, JSON helpers.
// 2. Cover boundary and error cases for helper functions.

import (
	"testing"

	"progressdb/pkg/utils"
)

func TestUtils_Suite(t *testing.T) {
	// Subtest: Validate ID/slug/path helpers produce expected non-empty outputs and correct splitting.
	t.Run("GenID_Slug_Split", func(t *testing.T) {
		id := utils.GenID()
		if id == "" {
			t.Fatalf("expected GenID to produce a value")
		}
		tid := utils.GenThreadID()
		if tid == "" {
			t.Fatalf("expected GenThreadID to produce a value")
		}
		slug := utils.MakeSlug("Hello World!", "xyz")
		if slug == "" {
			t.Fatalf("expected MakeSlug to produce a value")
		}
		parts := utils.SplitPath("/a/b/c/")
		if len(parts) != 3 {
			t.Fatalf("expected SplitPath to return 3 segments; got %d", len(parts))
		}
		_ = id
		_ = tid
		_ = slug
		_ = parts
	})

	// Subtest: Ensure JSON helper converts strings to RawMessage correctly.
	t.Run("JSONHelpers", func(t *testing.T) {
		vals := []string{"{\"a\":1}", "{\"b\":2}"}
		raws := utils.ToRawMessages(vals)
		if len(raws) != 2 {
			t.Fatalf("expected 2 raw messages; got %d", len(raws))
		}
		if string(raws[0]) != vals[0] {
			t.Fatalf("unexpected raw[0]: %s", string(raws[0]))
		}
	})
}
