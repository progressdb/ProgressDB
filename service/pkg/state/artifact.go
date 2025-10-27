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

func ArtifactRoot() string {
	artifactOnce.Do(func() {
		candidates := []string{
			os.Getenv("PROGRESSDB_ARTIFACT_ROOT"),
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

func ArtifactPath(elem ...string) string {
	root := ArtifactRoot()
	if root == "" {
		return ""
	}
	parts := append([]string{root}, elem...)
	return filepath.Join(parts...)
}
