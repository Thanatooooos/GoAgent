package config

import (
	"os"
	"path/filepath"

	"github.com/subosito/gotenv"
)

func init() {
	for _, candidate := range envCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			_ = gotenv.OverLoad(candidate)
			return
		}
	}
}

func envCandidates() []string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		return []string{".env"}
	}

	candidates := make([]string, 0, 6)
	dir := wd
	for i := 0; i < 6; i++ {
		candidates = append(candidates, filepath.Join(dir, ".env"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return candidates
}
