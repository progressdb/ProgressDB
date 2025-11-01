package tests

import (
	"testing"

	"progressdb/pkg/api/router"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/timeutil"
)

func TestUtils_Suite(t *testing.T) {
	t.Run("BasicHelpers", func(t *testing.T) {
		// Test basic utility functions
		nowFunc := timeutil.Now().UTC().String()
		id := keys.GenMessageKey(nowFunc, "test", 0)
		if id == "" {
			t.Fatalf("expected GenMessageKey to produce a value")
		}

		tid := keys.GenThreadKey(nowFunc)
		if tid == "" {
			t.Fatalf("expected GenThreadKey to produce a value")
		}

		slug := MakeSlug("Hello World!", "xyz")
		if slug == "" {
			t.Fatalf("expected MakeSlug to produce a value")
		}

		parts := SplitPath("/a/b/c/")
		if len(parts) != 3 {
			t.Fatalf("expected SplitPath to return 3 segments; got %d", len(parts))
		}
	})

	t.Run("JSONHelpers", func(t *testing.T) {
		vals := []string{"{\"a\":1}", "{\"b\":2}"}
		raws := router.ToRawMessages(vals)
		if len(raws) != 2 {
			t.Fatalf("expected 2 raw messages; got %d", len(raws))
		}
		if string(raws[0]) != vals[0] {
			t.Fatalf("unexpected raw[0]: %s", string(raws[0]))
		}
	})
}
