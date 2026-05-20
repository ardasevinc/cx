package projects

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardasevinc/cx/internal/sessions"
)

type Kind string

const (
	KindUnknown Kind = "unknown"
	KindChat    Kind = "chat"
	KindHome    Kind = "home"
	KindGit     Kind = "git"
	KindCWD     Kind = "cwd"
	KindMissing Kind = "missing"
)

type Root struct {
	Key         string
	DisplayName string
	Dir         string
	Kind        Kind
}

type Options struct {
	Home       string
	GitTimeout time.Duration
}

func ClassifySessions(items []sessions.Session, opts Options) map[string]Root {
	roots := make(map[string]Root)
	for _, item := range items {
		cwd := strings.TrimSpace(item.CWD)
		if cwd == "" {
			continue
		}
		if _, ok := roots[cwd]; ok {
			continue
		}
		roots[cwd] = ClassifyCWD(cwd, opts)
	}
	return roots
}

func ClassifyCWD(cwd string, opts Options) Root {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return Root{Key: "unknown", DisplayName: "unknown", Kind: KindUnknown}
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		home = absolutePath(home)
	}
	abs := absolutePath(cwd)

	if isChatPath(abs, home) {
		dir := filepath.Join(home, "Documents", "Codex")
		return Root{Key: "chats", DisplayName: "chats", Dir: dir, Kind: KindChat}
	}
	if home != "" && samePath(abs, home) {
		return Root{Key: "home", DisplayName: "home", Dir: home, Kind: KindHome}
	}

	info, err := os.Stat(abs)
	if err != nil {
		return Root{Key: "missing:" + abs, DisplayName: displayName(abs), Dir: abs, Kind: KindMissing}
	}
	if !info.IsDir() {
		return Root{Key: "cwd:" + abs, DisplayName: displayName(filepath.Dir(abs)), Dir: filepath.Dir(abs), Kind: KindCWD}
	}

	if root, ok := gitRoot(abs, opts.GitTimeout); ok {
		return Root{Key: "git:" + root, DisplayName: displayName(root), Dir: root, Kind: KindGit}
	}
	return Root{Key: "cwd:" + abs, DisplayName: displayName(abs), Dir: abs, Kind: KindCWD}
}

func gitRoot(cwd string, timeout time.Duration) (string, bool) {
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	return absolutePath(root), true
}

func isChatPath(cwd, home string) bool {
	if home == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Join(home, "Documents", "Codex"), cwd)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) >= 2 && looksLikeDate(parts[0])
}

func looksLikeDate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func absolutePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func displayName(path string) string {
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return path
	}
	return base
}

func samePath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && rel == "."
}
