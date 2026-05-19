package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type Options struct {
	Home string
	Now  time.Time
}

type ChatPlan struct {
	Dir string
}

func PlanChat(name string, opts Options) (ChatPlan, error) {
	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return ChatPlan{}, err
		}
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	dateDir := filepath.Join(home, "Documents", "Codex", now.Local().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		return ChatPlan{}, err
	}

	var dir string
	var err error
	if strings.TrimSpace(name) == "" {
		dir, err = nextNumberedDir(dateDir)
	} else {
		dir, err = nextNamedDir(dateDir, slugify(name))
	}
	if err != nil {
		return ChatPlan{}, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return ChatPlan{}, err
	}
	return ChatPlan{Dir: dir}, nil
}

func ProjectDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("project directory is required")
	}
	expanded, err := expandHome(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", expanded)
	}
	return filepath.Abs(expanded)
}

func nextNumberedDir(parent string) (string, error) {
	for i := 1; i < 10_000; i++ {
		dir := filepath.Join(parent, fmt.Sprintf("chat-%03d", i))
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			return dir, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no available chat directory under %s", parent)
}

func nextNamedDir(parent, slug string) (string, error) {
	if slug == "" {
		return nextNumberedDir(parent)
	}
	candidates := []string{slug}
	for i := 2; i < 10_000; i++ {
		candidates = append(candidates, fmt.Sprintf("%s-%d", slug, i))
	}
	for _, candidate := range candidates {
		dir := filepath.Join(parent, candidate)
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			return dir, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no available directory for %q under %s", slug, parent)
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
