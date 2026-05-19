package launcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanChatCreatesDatedNamedDirectory(t *testing.T) {
	home := t.TempDir()
	plan, err := PlanChat("Codex hey fam", Options{
		Home: home,
		Now:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, "Documents", "Codex", "2026-05-20", "codex-hey-fam")
	if plan.Dir != want {
		t.Fatalf("dir = %q, want %q", plan.Dir, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("expected created directory: info=%v err=%v", info, err)
	}
}

func TestPlanChatUsesCollisionFreeNamedDirectory(t *testing.T) {
	home := t.TempDir()
	existing := filepath.Join(home, "Documents", "Codex", "2026-05-20", "codex-hey-fam")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanChat("Codex hey fam", Options{
		Home: home,
		Now:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, "Documents", "Codex", "2026-05-20", "codex-hey-fam-2")
	if plan.Dir != want {
		t.Fatalf("dir = %q, want %q", plan.Dir, want)
	}
}

func TestPlanChatAutonumbersUnnamedDirectory(t *testing.T) {
	home := t.TempDir()
	existing := filepath.Join(home, "Documents", "Codex", "2026-05-20", "chat-001")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanChat("", Options{
		Home: home,
		Now:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, "Documents", "Codex", "2026-05-20", "chat-002")
	if plan.Dir != want {
		t.Fatalf("dir = %q, want %q", plan.Dir, want)
	}
}

func TestProjectDirRequiresExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := ProjectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("dir = %q, want %q", got, dir)
	}
}
