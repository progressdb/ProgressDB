package state

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	artifactOnce sync.Once
	artifactRoot string
)

// ArtifactRoot returns the base directory for runtime/test artifacts when
// configured via environment variables. It resolves the first non-empty value
// of PROGRESSDB_ARTIFACT_ROOT or TEST_ARTIFACTS_ROOT and normalizes it to an
// absolute path. Callers fall back to legacy defaults when the result is empty.
func ArtifactRoot() string {
	artifactOnce.Do(func() {
		candidates := []string{
			os.Getenv("PROGRESSDB_ARTIFACT_ROOT"),
			os.Getenv("TEST_ARTIFACTS_ROOT"),
		}
		for _, c := range candidates {
			if strings.TrimSpace(c) == "" {
				continue
			}
			if abs, err := filepath.Abs(c); err == nil {
				artifactRoot = abs
			} else {
				artifactRoot = c
			}
			break
		}
	})
	return artifactRoot
}

// ArtifactPath joins the artifact root with the provided path elements. It
// returns an empty string when no artifact root is configured.
func ArtifactPath(elem ...string) string {
	root := ArtifactRoot()
	if root == "" {
		return ""
	}
	parts := append([]string{root}, elem...)
	return filepath.Join(parts...)
}
