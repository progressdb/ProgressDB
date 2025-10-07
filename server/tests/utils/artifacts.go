package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var (
	baseRootOnce sync.Once
	baseRoot     string
	baseErr      error

	testRoots sync.Map
	dirSeq    uint64
)

var sanitizeRegexp = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func computeBaseRoot() (string, error) {
	for _, candidate := range []string{
		os.Getenv("PROGRESSDB_ARTIFACT_ROOT"),
		os.Getenv("TEST_ARTIFACTS_ROOT"),
	} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return candidate, nil
		}
		return abs, nil
	}
	// fallback to repo-local tests/artifacts directory
	repoRel := filepath.Join("..", "..", "tests", "artifacts")
	abs, err := filepath.Abs(repoRel)
	if err != nil {
		return repoRel, nil
	}
	return abs, nil
}

func baseArtifactsRoot() (string, error) {
	baseRootOnce.Do(func() {
		baseRoot, baseErr = computeBaseRoot()
		if baseErr == nil {
			baseErr = os.MkdirAll(baseRoot, 0o755)
		}
	})
	return baseRoot, baseErr
}

func sanitizeTestName(name string) string {
	cleaned := sanitizeRegexp.ReplaceAllString(name, "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		cleaned = "test"
	}
	return cleaned
}

func setEnvWithCleanup(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// TestArtifactsRoot returns (and memoizes) the artifact directory assigned to
// the current test. The directory is created if missing, environment variables
// are updated, and a cleanup handler removes the memoized entry when the test
// completes. Callers can assume the path is absolute.
func TestArtifactsRoot(t *testing.T) string {
	t.Helper()
	if v, ok := testRoots.Load(t); ok {
		return v.(string)
	}
	base, err := baseArtifactsRoot()
	if err != nil {
		t.Fatalf("resolve artifact root: %v", err)
	}
	name := sanitizeTestName(t.Name())
	dir := filepath.Join(base, name)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		t.Fatalf("reset artifact dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	setEnvWithCleanup(t, "PROGRESSDB_ARTIFACT_ROOT", dir)
	setEnvWithCleanup(t, "TEST_ARTIFACTS_ROOT", dir)
	testRoots.Store(t, dir)
	t.Cleanup(func() {
		testRoots.Delete(t)
	})
	return dir
}

// NewArtifactsDir creates a unique subdirectory under the test's artifact root
// using the provided prefix. The directory is created and returned.
func NewArtifactsDir(t *testing.T, prefix string) string {
	t.Helper()
	root := TestArtifactsRoot(t)
	id := atomic.AddUint64(&dirSeq, 1)
	name := fmt.Sprintf("%s-%03d", sanitizeTestName(prefix), id)
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create artifact subdir: %v", err)
	}
	return path
}
