package indexer

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRebuildIndexesTranscriptSearchAndPreview(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "06", "10")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(sessionDir, "rollout-indexed.jsonl")
	writeFile(t, rolloutPath, `{"type":"session_meta","payload":{"id":"indexed-thread","timestamp":"2026-06-10T10:00:00Z","cwd":"/tmp/cx","source":"cli","thread_source":"user"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"find the transcript cache needle"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"the answer contains observable sqlite indexing"}]}}
`)
	createStateDB(t, filepath.Join(codexHome, "state_5.sqlite"), rolloutPath, "indexed-thread")

	cachePath := filepath.Join(root, "cache", "index.sqlite")
	result, err := Rebuild(Options{CodexHome: codexHome, CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status.TotalSessions != 1 || result.Status.IndexedSessions != 1 || result.Status.ChunkCount != 2 {
		t.Fatalf("unexpected status: %#v", result.Status)
	}

	hits, err := Search(Options{CodexHome: codexHome, CachePath: cachePath}, "needle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].SessionID != "indexed-thread" {
		t.Fatalf("expected indexed search hit, got %#v", hits)
	}

	preview, err := LoadPreview(Options{CodexHome: codexHome, CachePath: cachePath}, "indexed-thread", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "indexed" || len(preview.Lines) != 2 {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	refreshed, err := Refresh(Options{CodexHome: codexHome, CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.IndexedCount != 0 || refreshed.SkippedCount != 1 {
		t.Fatalf("expected unchanged refresh to skip session, got indexed=%d skipped=%d", refreshed.IndexedCount, refreshed.SkippedCount)
	}
}

func TestRebuildMarksMissingRollout(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	createStateDB(t, filepath.Join(codexHome, "state_5.sqlite"), filepath.Join(root, "missing.jsonl"), "missing-thread")

	status, err := Rebuild(Options{CodexHome: codexHome, CachePath: filepath.Join(root, "cache.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status.MissingSessions != 1 {
		t.Fatalf("expected missing session count, got %#v", status.Status)
	}
}

func TestDoctorReportsEmptyCache(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}

	status, err := Doctor(Options{CodexHome: codexHome, CachePath: filepath.Join(root, "cache.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Problems) == 0 {
		t.Fatalf("expected doctor problem for empty cache")
	}
}

func createStateDB(t *testing.T, path, rolloutPath, id string) {
	t.Helper()
	statement := `
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
values (?, ?, 1, 2, 'cli', 'openai', '/tmp/cx', 'Index fixture', '{}', 'never', 42, 0, 1000, 2000, 'find the transcript cache needle', 'user', 'Index preview');
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	for _, part := range strings.Split(statement, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "insert into") {
			if _, err := db.Exec(part, id, rolloutPath); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := db.Exec(part); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
