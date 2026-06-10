package sessions

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoadIndexesSessionsByUpdatedTime(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "alice")
	t.Setenv("HOME", home)
	codexHome := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "05", "19")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeRollout(t, filepath.Join(sessionDir, "rollout-old.jsonl"), `{"type":"session_meta","payload":{"id":"old","timestamp":"2026-05-19T10:00:00Z","cwd":"/home/alice/src/cx","source":"cli","thread_source":"user"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions ignored"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"build session picker"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working on it"}]}}
`)
	writeRollout(t, filepath.Join(sessionDir, "rollout-new.jsonl"), `{"type":"session_meta","payload":{"id":"new","timestamp":"2026-05-19T11:00:00Z","cwd":"`+filepath.Join(home, "Documents", "Codex", "2026-05-19", "codex-hey")+`","source":"cli","thread_source":"user"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"resume vpn thread"}]}}
`)

	sessions, err := Load(Options{CodexHome: codexHome, LoadForkParents: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "new" {
		t.Fatalf("expected newest file first, got %q", sessions[0].ID)
	}
	if sessions[1].Title != "build session picker" {
		t.Fatalf("unexpected title: %q", sessions[1].Title)
	}
	if sessions[0].Project != "chats" {
		t.Fatalf("expected Codex document cwd to be grouped as chats, got %q", sessions[0].Project)
	}
}

func TestFilterMatchesMetadataAndTranscript(t *testing.T) {
	all := []Session{
		{ID: "a", Title: "Build picker", CWD: "/tmp/cx", SearchText: buildSearchText(Session{ID: "a", Title: "Build picker", CWD: "/tmp/cx"})},
		{ID: "b", Title: "VPN notes", CWD: "/tmp/vpn", SearchText: buildSearchText(Session{ID: "b", Title: "VPN notes", CWD: "/tmp/vpn"})},
	}

	filtered := Filter(all, "pick")
	if len(filtered) != 1 || filtered[0].ID != "a" {
		t.Fatalf("expected picker match, got %#v", filtered)
	}
}

func TestProjectNameSpecialCases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		cwd  string
		want string
	}{
		{cwd: home, want: "home"},
		{cwd: filepath.Join(home, "Documents", "Codex", "2026-05-19", "chat"), want: "chats"},
		{cwd: filepath.Join(home, "programming", "open-source", "cx"), want: "cx"},
	}

	for _, tt := range tests {
		if got := projectName(tt.cwd); got != tt.want {
			t.Fatalf("projectName(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}

func TestLoadUsesStateDBWhenAvailable(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(codexHome, "state_5.sqlite")
	sql := `
create table threads (
  id text primary key,
  rollout_path text not null,
  created_at integer not null,
  updated_at integer not null,
  source text not null,
  model_provider text not null,
  cwd text not null,
  title text not null,
  sandbox_policy text not null,
  approval_mode text not null,
  tokens_used integer not null default 0,
  has_user_event integer not null default 0,
  archived integer not null default 0,
  archived_at integer,
  git_sha text,
  git_branch text,
  git_origin_url text,
  cli_version text not null default '',
  first_user_message text not null default '',
  agent_nickname text,
  agent_role text,
  memory_mode text not null default 'enabled',
  model text,
  reasoning_effort text,
  agent_path text,
  created_at_ms integer,
  updated_at_ms integer,
  thread_source text,
  preview text not null default ''
);
insert into threads (id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, sandbox_policy, approval_mode, tokens_used, archived, created_at_ms, updated_at_ms, thread_source, preview)
values ('db-thread', '/tmp/db-thread.jsonl', 1, 2, 'cli', 'openai', '/tmp/cx', 'DB title', '{}', 'never', 42, 0, 1000, 2000, 'user', 'DB preview');
`
	execSQL(t, dbPath, sql)

	sessions, err := Load(Options{CodexHome: codexHome})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "db-thread" || sessions[0].Title != "DB title" || sessions[0].TokensUsed != 42 {
		t.Fatalf("unexpected db session: %#v", sessions[0])
	}
}

func TestLoadUsesSessionIndexThreadNameWhenDBTitleIsFirstMessage(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(codexHome, "state_5.sqlite")
	sql := `
create table threads (
  id text primary key,
  rollout_path text not null,
  created_at integer not null,
  updated_at integer not null,
  source text not null,
  model_provider text not null,
  cwd text not null,
  title text not null,
  sandbox_policy text not null,
  approval_mode text not null,
  tokens_used integer not null default 0,
  has_user_event integer not null default 0,
  archived integer not null default 0,
  archived_at integer,
  git_sha text,
  git_branch text,
  git_origin_url text,
  cli_version text not null default '',
  first_user_message text not null default '',
  agent_nickname text,
  agent_role text,
  memory_mode text not null default 'enabled',
  model text,
  reasoning_effort text,
  agent_path text,
  created_at_ms integer,
  updated_at_ms integer,
  thread_source text,
  preview text not null default ''
);
insert into threads (id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, sandbox_policy, approval_mode, tokens_used, archived, created_at_ms, updated_at_ms, first_user_message, thread_source, preview)
	values ('named-thread', '/tmp/named-thread.jsonl', 1, 2, 'cli', 'openai', '/tmp/cx', 'start the billing audit', '{}', 'never', 42, 0, 1000, 2000, 'start the billing audit', 'user', 'start the billing audit');
`
	execSQL(t, dbPath, sql)
	writeRollout(t, filepath.Join(codexHome, "session_index.jsonl"), `{"id":"named-thread","thread_name":"Investigate issue","updated_at":"2026-05-12T19:18:49Z"}
{"id":"named-thread","thread_name":"Production billing audit","updated_at":"2026-05-12T19:59:18Z"}
`)

	sessions, err := Load(Options{CodexHome: codexHome, LoadForkParents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "Production billing audit" {
		t.Fatalf("expected session-index title, got %q", sessions[0].Title)
	}
	if !strings.Contains(sessions[0].SearchText, "production billing audit") {
		t.Fatalf("expected custom title to be searchable: %q", sessions[0].SearchText)
	}
}

func TestLoadInheritsParentThreadNameForFork(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "05", "19")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	forkPath := filepath.Join(sessionDir, "rollout-fork.jsonl")
	writeRollout(t, forkPath, `{"type":"session_meta","payload":{"id":"fork-thread","forked_from_id":"parent-thread","timestamp":"2026-05-19T19:25:46Z","cwd":"/tmp/cx","source":"cli","thread_source":"user"}}
`)

	dbPath := filepath.Join(codexHome, "state_5.sqlite")
	sql := `
create table threads (
  id text primary key,
  rollout_path text not null,
  created_at integer not null,
  updated_at integer not null,
  source text not null,
  model_provider text not null,
  cwd text not null,
  title text not null,
  sandbox_policy text not null,
  approval_mode text not null,
  tokens_used integer not null default 0,
  has_user_event integer not null default 0,
  archived integer not null default 0,
  archived_at integer,
  git_sha text,
  git_branch text,
  git_origin_url text,
  cli_version text not null default '',
  first_user_message text not null default '',
  agent_nickname text,
  agent_role text,
  memory_mode text not null default 'enabled',
  model text,
  reasoning_effort text,
  agent_path text,
  created_at_ms integer,
  updated_at_ms integer,
  thread_source text,
  preview text not null default ''
);
insert into threads (id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, sandbox_policy, approval_mode, archived, created_at_ms, updated_at_ms, first_user_message, thread_source, preview)
values ('fork-thread', '` + forkPath + `', 1, 2, 'cli', 'openai', '/tmp/cx', 'start the billing audit', '{}', 'never', 0, 1000, 2000, 'start the billing audit', 'user', 'start the billing audit');
`
	execSQL(t, dbPath, sql)
	writeRollout(t, filepath.Join(codexHome, "session_index.jsonl"), `{"id":"parent-thread","thread_name":"Production billing audit","updated_at":"2026-05-12T19:59:18Z"}
`)

	sessions, err := Load(Options{CodexHome: codexHome, LoadForkParents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "Production billing audit" {
		t.Fatalf("expected inherited parent title, got %q", sessions[0].Title)
	}
	if sessions[0].ParentID != "parent-thread" {
		t.Fatalf("expected parent id, got %q", sessions[0].ParentID)
	}
}

func writeRollout(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, path string, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}
