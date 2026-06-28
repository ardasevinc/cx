package projects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardasevinc/cx/internal/sessions"
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

func TestFilterSessionsByCWDSameGitRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	app := filepath.Join(repo, "apps", "web")
	pkg := filepath.Join(repo, "packages", "api")
	other := filepath.Join(t.TempDir(), "other")
	for _, dir := range []string{app, pkg, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got := FilterSessionsByCWD([]sessions.Session{
		{ID: "root", CWD: repo},
		{ID: "app", CWD: app},
		{ID: "pkg", CWD: pkg},
		{ID: "other", CWD: other},
	}, app, Options{GitTimeout: time.Second})

	if ids := sessionIDs(got); ids != "root,app,pkg" {
		t.Fatalf("expected same git root sessions, got %q", ids)
	}
}

func TestFilterSessionsByScopeUsesProvidedRoots(t *testing.T) {
	scope := Scope{
		CWD: "/repo/apps/web",
		Root: Root{
			Kind: KindGit,
			Dir:  "/repo",
		},
	}
	items := []sessions.Session{
		{ID: "web", CWD: "/repo/apps/web"},
		{ID: "api", CWD: "/repo/packages/api"},
		{ID: "other", CWD: "/other"},
	}
	roots := map[string]Root{
		"/repo/apps/web": {
			Kind: KindGit,
			Dir:  "/repo",
		},
		"/repo/packages/api": {
			Kind: KindGit,
			Dir:  "/repo",
		},
		"/other": {
			Kind: KindCWD,
			Dir:  "/other",
		},
	}

	got := FilterSessionsByScope(items, scope, roots, Options{})

	if ids := sessionIDs(got); ids != "web,api" {
		t.Fatalf("expected same provided git root sessions, got %q", ids)
	}
}

func TestScopeDoesNotMatchEverythingUnderHome(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "src", "cx")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FilterSessionsByCWD([]sessions.Session{
		{ID: "home", CWD: home},
		{ID: "project", CWD: project},
	}, home, Options{Home: home})

	if ids := sessionIDs(got); ids != "home" {
		t.Fatalf("expected only exact home session, got %q", ids)
	}
}

func TestProjectScopeDoesNotMatchHomeAncestorSession(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "src", "cx")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FilterSessionsByCWD([]sessions.Session{
		{ID: "home", CWD: home},
		{ID: "project", CWD: project},
	}, project, Options{Home: home})

	if ids := sessionIDs(got); ids != "project" {
		t.Fatalf("expected only project session, got %q", ids)
	}
}

func TestProjectScopeDoesNotMatchBroadParentDirectory(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "cx")
	child := filepath.Join(project, "internal")
	for _, dir := range []string{project, child} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got := FilterSessionsByCWD([]sessions.Session{
		{ID: "parent", CWD: parent},
		{ID: "project", CWD: project},
		{ID: "child", CWD: child},
	}, project, Options{})

	if ids := sessionIDs(got); ids != "project,child" {
		t.Fatalf("expected project and child sessions, got %q", ids)
	}
}

func sessionIDs(items []sessions.Session) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return strings.Join(ids, ",")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
