package tests

import (
    "testing"

    "progressdb/pkg/utils"
)

func TestUtils_GenIDAndSlugAndSplit(t *testing.T) {
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
}

