package projects

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClassifyChatPath(t *testing.T) {
	home := t.TempDir()
	chat := filepath.Join(home, "Documents", "Codex", "2026-05-20", "chat-001")
	if err := os.MkdirAll(chat, 0o755); err != nil {
		t.Fatal(err)
	}
	root := ClassifyCWD(chat, Options{Home: home})

	if root.Kind != KindChat || root.Key != "chats" || root.DisplayName != "chats" {
		t.Fatalf("unexpected chat root: %#v", root)
	}
}

func TestClassifyHomePath(t *testing.T) {
	home := t.TempDir()
	root := ClassifyCWD(home, Options{Home: home})

	if root.Kind != KindHome || root.DisplayName != "home" {
		t.Fatalf("unexpected home root: %#v", root)
	}
}

func TestClassifyNestedGitPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	nested := filepath.Join(repo, "internal", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	root := ClassifyCWD(nested, Options{GitTimeout: time.Second})

	if root.Kind != KindGit || root.Dir != repo || root.DisplayName != filepath.Base(repo) {
		t.Fatalf("unexpected git root: %#v", root)
	}
}

func TestClassifyMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	root := ClassifyCWD(missing, Options{})

	if root.Kind != KindMissing || root.Dir != missing {
		t.Fatalf("unexpected missing root: %#v", root)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
