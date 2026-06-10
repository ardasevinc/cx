package indexer

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ardasevinc/cx/internal/sessions"
)

const (
	schemaVersion       = 1
	maxLineBytes        = 512 * 1024
	maxChunkRunes       = 16 * 1024
	maxIndexedRunes     = 768 * 1024
	defaultSearchLimit  = 80
	defaultPreviewLimit = 16
)

type Options struct {
	Home      string
	CodexHome string
	CachePath string
}

type Status struct {
	CachePath         string   `json:"cache_path"`
	CodexHome         string   `json:"codex_home"`
	StateDBPath       string   `json:"state_db_path"`
	SchemaVersion     int      `json:"schema_version"`
	FTSAvailable      bool     `json:"fts_available"`
	TotalSessions     int      `json:"total_sessions"`
	IndexedSessions   int      `json:"indexed_sessions"`
	PendingSessions   int      `json:"pending_sessions"`
	FailedSessions    int      `json:"failed_sessions"`
	MissingSessions   int      `json:"missing_sessions"`
	TruncatedSessions int      `json:"truncated_sessions"`
	ChunkCount        int      `json:"chunk_count"`
	CacheBytes        int64    `json:"cache_bytes"`
	LatestIndexedAt   string   `json:"latest_indexed_at"`
	Problems          []string `json:"problems,omitempty"`
}

type RebuildResult struct {
	Status       Status        `json:"status"`
	Elapsed      time.Duration `json:"elapsed"`
	IndexedCount int           `json:"indexed_count"`
	SkippedCount int           `json:"skipped_count"`
}

type SearchResult struct {
	SessionID string
	Role      string
	Kind      string
	Snippet   string
	Score     int
}

type Preview struct {
	SessionID string
	Status    string
	Error     string
	Truncated bool
	Lines     []sessions.Line
}

type transcriptChunk struct {
	Role string
	Kind string
	Text string
}

type rolloutItem struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type responseItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content []responseChunk `json:"content"`
}

type responseChunk struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type eventMsg struct {
	Msg string `json:"msg"`
}

type paths struct {
	codexHome string
	cachePath string
}

func ResolvePaths(opts Options) (string, string, error) {
	paths, err := resolvePaths(opts)
	if err != nil {
		return "", "", err
	}
	return paths.codexHome, paths.cachePath, nil
}

func CurrentStatus(opts Options) (Status, error) {
	paths, err := resolvePaths(opts)
	if err != nil {
		return Status{}, err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return Status{}, err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return Status{}, err
	}
	return readStatus(db, paths)
}

func Rebuild(opts Options) (RebuildResult, error) {
	start := time.Now()
	paths, err := resolvePaths(opts)
	if err != nil {
		return RebuildResult{}, err
	}
	items, err := sessions.Load(sessions.Options{CodexHome: paths.codexHome})
	if err != nil {
		return RebuildResult{}, err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return RebuildResult{}, err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return RebuildResult{}, err
	}
	indexed, skipped, err := rebuildSessions(db, items, true)
	if err != nil {
		return RebuildResult{}, err
	}
	status, err := readStatus(db, paths)
	if err != nil {
		return RebuildResult{}, err
	}
	return RebuildResult{Status: status, Elapsed: time.Since(start), IndexedCount: indexed, SkippedCount: skipped}, nil
}

func Refresh(opts Options) (RebuildResult, error) {
	start := time.Now()
	paths, err := resolvePaths(opts)
	if err != nil {
		return RebuildResult{}, err
	}
	items, err := sessions.Load(sessions.Options{CodexHome: paths.codexHome})
	if err != nil {
		return RebuildResult{}, err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return RebuildResult{}, err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return RebuildResult{}, err
	}
	indexed, skipped, err := rebuildSessions(db, items, false)
	if err != nil {
		return RebuildResult{}, err
	}
	status, err := readStatus(db, paths)
	if err != nil {
		return RebuildResult{}, err
	}
	return RebuildResult{Status: status, Elapsed: time.Since(start), IndexedCount: indexed, SkippedCount: skipped}, nil
}

func Vacuum(opts Options) error {
	paths, err := resolvePaths(opts)
	if err != nil {
		return err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return err
	}
	_, err = db.Exec(`vacuum`)
	return err
}

func Search(opts Options, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 3 {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	paths, err := resolvePaths(opts)
	if err != nil {
		return nil, err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return nil, err
	}
	fts, _ := ftsAvailable(db)
	if fts {
		results, err := searchFTS(db, query, limit)
		if err == nil {
			return results, nil
		}
	}
	return searchLike(db, query, limit)
}

func LoadPreview(opts Options, sessionID, query string, limit int) (Preview, error) {
	if limit <= 0 {
		limit = defaultPreviewLimit
	}
	paths, err := resolvePaths(opts)
	if err != nil {
		return Preview{}, err
	}
	db, err := openCache(paths.cachePath)
	if err != nil {
		return Preview{}, err
	}
	defer func() {
		_ = db.Close()
	}()
	if err := migrate(db); err != nil {
		return Preview{}, err
	}
	preview := Preview{SessionID: sessionID}
	err = db.QueryRow(`select index_status, index_error, case when index_status = 'truncated' then 1 else 0 end from sessions where session_id = ?`, sessionID).Scan(&preview.Status, &preview.Error, &preview.Truncated)
	if errors.Is(err, sql.ErrNoRows) {
		preview.Status = "missing-cache"
		return preview, nil
	}
	if err != nil {
		return Preview{}, err
	}

	rows, err := db.Query(`
select role, text
from chunks
where session_id = ?
order by ordinal desc
limit ?`, sessionID, limit)
	if err != nil {
		return Preview{}, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var reversed []sessions.Line
	for rows.Next() {
		var line sessions.Line
		if err := rows.Scan(&line.Role, &line.Text); err != nil {
			return Preview{}, err
		}
		reversed = append(reversed, line)
	}
	if err := rows.Err(); err != nil {
		return Preview{}, err
	}
	for i := len(reversed) - 1; i >= 0; i-- {
		preview.Lines = append(preview.Lines, reversed[i])
	}
	if strings.TrimSpace(query) != "" {
		hits, err := searchLikeSession(db, sessionID, query, 6)
		if err == nil && len(hits) > 0 {
			preview.Lines = hits
		}
	}
	return preview, nil
}

func Doctor(opts Options) (Status, error) {
	status, err := CurrentStatus(opts)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(status.StateDBPath); err != nil {
		status.Problems = append(status.Problems, "state db unreadable: "+err.Error())
	}
	if status.TotalSessions == 0 {
		status.Problems = append(status.Problems, "cache has no indexed session metadata")
	}
	if status.FailedSessions > 0 {
		status.Problems = append(status.Problems, fmt.Sprintf("%d sessions failed indexing", status.FailedSessions))
	}
	if status.TruncatedSessions > 0 {
		status.Problems = append(status.Problems, fmt.Sprintf("%d sessions are truncated", status.TruncatedSessions))
	}
	if !status.FTSAvailable {
		status.Problems = append(status.Problems, "FTS5 unavailable; transcript search uses slower fallback")
	}
	return status, nil
}

func resolvePaths(opts Options) (paths, error) {
	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return paths{}, err
		}
	}
	codexHome := opts.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	cachePath := opts.CachePath
	if cachePath == "" {
		cacheRoot, err := os.UserCacheDir()
		if err != nil {
			cacheRoot = filepath.Join(home, ".cache")
		}
		cachePath = filepath.Join(cacheRoot, "cx", "index.sqlite")
	}
	return paths{codexHome: codexHome, cachePath: cachePath}, nil
}

func openCache(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path, "cache=shared&_pragma=busy_timeout(250)"))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`pragma journal_mode = wal; pragma busy_timeout = 250;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteDSN(path, rawQuery string) string {
	return "file:" + filepath.ToSlash(filepath.Clean(path)) + "?" + rawQuery
}

func migrate(db *sql.DB) error {
	statements := []string{
		`create table if not exists schema_meta (key text primary key, value text not null)`,
		`insert into schema_meta (key, value) values ('schema_version', '1') on conflict(key) do update set value = excluded.value`,
		`create table if not exists sessions (
			session_id text primary key,
			title text not null default '',
			preview text not null default '',
			cwd text not null default '',
			project text not null default '',
			source text not null default '',
			path text not null default '',
			parent_id text not null default '',
			created_at_ms integer not null default 0,
			updated_at_ms integer not null default 0,
			turns integer not null default 0,
			tokens_used integer not null default 0,
			indexed_size integer not null default 0,
			indexed_mtime_ns integer not null default 0,
			indexed_updated_at_ms integer not null default 0,
			indexed_at_ms integer not null default 0,
			index_status text not null default 'pending',
			index_error text not null default ''
		)`,
		`create index if not exists sessions_updated_idx on sessions(updated_at_ms desc)`,
		`create index if not exists sessions_project_idx on sessions(project, updated_at_ms desc)`,
		`create table if not exists chunks (
			id integer primary key autoincrement,
			session_id text not null,
			ordinal integer not null,
			role text not null,
			kind text not null,
			text text not null
		)`,
		`create index if not exists chunks_session_idx on chunks(session_id, ordinal)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`create virtual table if not exists chunks_fts using fts5(text, role unindexed, kind unindexed, session_id unindexed, ordinal unindexed)`); err == nil {
		_, _ = db.Exec(`insert into schema_meta (key, value) values ('fts_available', '1') on conflict(key) do update set value = excluded.value`)
	} else {
		_, _ = db.Exec(`insert into schema_meta (key, value) values ('fts_available', '0') on conflict(key) do update set value = excluded.value`)
	}
	return nil
}

func rebuildSessions(db *sql.DB, items []sessions.Session, force bool) (int, int, error) {
	if force {
		if _, err := db.Exec(`delete from chunks`); err != nil {
			return 0, 0, err
		}
		_, _ = db.Exec(`delete from chunks_fts`)
	}
	indexed := 0
	skipped := 0
	for _, item := range items {
		if !force {
			fresh, err := cacheFresh(db, item)
			if err != nil {
				return indexed, skipped, err
			}
			if fresh {
				skipped++
				continue
			}
		}
		if err := rebuildSession(db, item); err != nil {
			return indexed, skipped, err
		}
		indexed++
	}
	return indexed, skipped, nil
}

func cacheFresh(db *sql.DB, item sessions.Session) (bool, error) {
	size, mtimeNS, err := fileFingerprint(item.Path)
	if err != nil {
		size = 0
		mtimeNS = 0
	}
	var cachedSize, cachedMtime, cachedUpdated int64
	var status string
	err = db.QueryRow(`select indexed_size, indexed_mtime_ns, indexed_updated_at_ms, index_status from sessions where session_id = ?`, item.ID).Scan(&cachedSize, &cachedMtime, &cachedUpdated, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != "indexed" && status != "truncated" && status != "missing" {
		return false, nil
	}
	return cachedSize == size && cachedMtime == mtimeNS && cachedUpdated == item.UpdatedAt.UnixMilli(), nil
}

func rebuildSession(db *sql.DB, item sessions.Session) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	size, mtimeNS, statErr := fileFingerprint(item.Path)
	status := "indexed"
	indexErr := ""
	truncated := false
	var chunks []transcriptChunk
	if item.Path == "" {
		status = "missing"
	} else if statErr != nil {
		status = "missing"
		indexErr = statErr.Error()
	} else {
		chunks, truncated, err = extractTranscript(item.Path)
		if err != nil {
			status = "failed"
			indexErr = err.Error()
		} else if truncated {
			status = "truncated"
		}
	}

	_, err = tx.Exec(`
insert into sessions (
	session_id, title, preview, cwd, project, source, path, parent_id,
	created_at_ms, updated_at_ms, turns, tokens_used,
	indexed_size, indexed_mtime_ns, indexed_updated_at_ms, indexed_at_ms,
	index_status, index_error
) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(session_id) do update set
	title = excluded.title,
	preview = excluded.preview,
	cwd = excluded.cwd,
	project = excluded.project,
	source = excluded.source,
	path = excluded.path,
	parent_id = excluded.parent_id,
	created_at_ms = excluded.created_at_ms,
	updated_at_ms = excluded.updated_at_ms,
	turns = excluded.turns,
	tokens_used = excluded.tokens_used,
	indexed_size = excluded.indexed_size,
	indexed_mtime_ns = excluded.indexed_mtime_ns,
	indexed_updated_at_ms = excluded.indexed_updated_at_ms,
	indexed_at_ms = excluded.indexed_at_ms,
	index_status = excluded.index_status,
	index_error = excluded.index_error`,
		item.ID, item.Title, item.Preview, item.CWD, item.Project, item.Source, item.Path, item.ParentID,
		item.CreatedAt.UnixMilli(), item.UpdatedAt.UnixMilli(), item.Turns, item.TokensUsed,
		size, mtimeNS, item.UpdatedAt.UnixMilli(), time.Now().UnixMilli(), status, indexErr)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from chunks where session_id = ?`, item.ID); err != nil {
		return err
	}
	_, _ = tx.Exec(`delete from chunks_fts where session_id = ?`, item.ID)
	for ordinal, chunk := range chunks {
		if _, err := tx.Exec(`insert into chunks (session_id, ordinal, role, kind, text) values (?, ?, ?, ?, ?)`, item.ID, ordinal, chunk.Role, chunk.Kind, chunk.Text); err != nil {
			return err
		}
		_, _ = tx.Exec(`insert into chunks_fts (text, role, kind, session_id, ordinal) values (?, ?, ?, ?, ?)`, chunk.Text, chunk.Role, chunk.Kind, item.ID, ordinal)
	}
	return tx.Commit()
}

func fileFingerprint(path string) (int64, int64, error) {
	if path == "" {
		return 0, 0, os.ErrNotExist
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	return info.Size(), info.ModTime().UnixNano(), nil
}

func extractTranscript(path string) ([]transcriptChunk, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = file.Close()
	}()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	var chunks []transcriptChunk
	var total int
	truncated := false
	for scanner.Scan() {
		chunk, ok := extractLine(scanner.Bytes())
		if !ok {
			continue
		}
		for _, piece := range splitChunk(chunk.Text) {
			total += len([]rune(piece))
			if total > maxIndexedRunes {
				truncated = true
				return chunks, truncated, nil
			}
			chunks = append(chunks, transcriptChunk{Role: chunk.Role, Kind: chunk.Kind, Text: piece})
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) || errors.Is(err, io.ErrShortBuffer) {
			return chunks, true, nil
		}
		return chunks, truncated, err
	}
	return chunks, truncated, nil
}

func extractLine(raw []byte) (transcriptChunk, bool) {
	var item rolloutItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return transcriptChunk{}, false
	}
	switch item.Type {
	case "response_item":
		var response responseItem
		if err := json.Unmarshal(item.Payload, &response); err != nil {
			return transcriptChunk{}, false
		}
		if response.Type != "message" || !humanRole(response.Role) {
			return transcriptChunk{}, false
		}
		text := cleanText(contentText(response.Content))
		if text == "" || infrastructureText(text) {
			return transcriptChunk{}, false
		}
		return transcriptChunk{Role: response.Role, Kind: "message", Text: text}, true
	case "event_msg":
		var event eventMsg
		if err := json.Unmarshal(item.Payload, &event); err != nil {
			return transcriptChunk{}, false
		}
		text := cleanText(event.Msg)
		if text == "" || infrastructureText(text) {
			return transcriptChunk{}, false
		}
		return transcriptChunk{Role: "event", Kind: "event_msg", Text: text}, true
	default:
		return transcriptChunk{}, false
	}
}

func contentText(chunks []responseChunk) string {
	var parts []string
	for _, chunk := range chunks {
		if chunk.Text == "" {
			continue
		}
		switch chunk.Type {
		case "input_text", "output_text", "text":
			parts = append(parts, chunk.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func splitChunk(text string) []string {
	runes := []rune(text)
	if len(runes) <= maxChunkRunes {
		return []string{text}
	}
	var out []string
	for len(runes) > 0 {
		n := min(maxChunkRunes, len(runes))
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func humanRole(role string) bool {
	return role == "user" || role == "assistant"
}

func infrastructureText(text string) bool {
	prefixes := []string{
		"# AGENTS.md instructions",
		"<environment_context>",
		"<goal_context>",
		"<permissions instructions>",
		"<collaboration_mode>",
		"<skills_instructions>",
		"<plugins_instructions>",
		"## Memory",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func readStatus(db *sql.DB, paths paths) (Status, error) {
	status := Status{
		CachePath:     paths.cachePath,
		CodexHome:     paths.codexHome,
		StateDBPath:   filepath.Join(paths.codexHome, "state_5.sqlite"),
		SchemaVersion: schemaVersion,
	}
	status.FTSAvailable, _ = ftsAvailable(db)
	_ = db.QueryRow(`select count(*) from sessions`).Scan(&status.TotalSessions)
	_ = db.QueryRow(`select count(*) from sessions where index_status = 'indexed'`).Scan(&status.IndexedSessions)
	_ = db.QueryRow(`select count(*) from sessions where index_status = 'pending'`).Scan(&status.PendingSessions)
	_ = db.QueryRow(`select count(*) from sessions where index_status = 'failed'`).Scan(&status.FailedSessions)
	_ = db.QueryRow(`select count(*) from sessions where index_status = 'missing'`).Scan(&status.MissingSessions)
	_ = db.QueryRow(`select count(*) from sessions where index_status = 'truncated'`).Scan(&status.TruncatedSessions)
	_ = db.QueryRow(`select count(*) from chunks`).Scan(&status.ChunkCount)
	var latest int64
	_ = db.QueryRow(`select coalesce(max(indexed_at_ms), 0) from sessions`).Scan(&latest)
	if latest > 0 {
		status.LatestIndexedAt = time.UnixMilli(latest).Local().Format(time.RFC3339)
	}
	if info, err := os.Stat(paths.cachePath); err == nil {
		status.CacheBytes = info.Size()
	}
	return status, nil
}

func ftsAvailable(db *sql.DB) (bool, error) {
	var name string
	err := db.QueryRow(`select name from sqlite_master where type = 'table' and name = 'chunks_fts'`).Scan(&name)
	if err != nil {
		return false, err
	}
	return name == "chunks_fts", nil
}

func searchFTS(db *sql.DB, query string, limit int) ([]SearchResult, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	rows, err := db.Query(`
select session_id, role, kind, snippet(chunks_fts, 0, '[', ']', ' ... ', 12), bm25(chunks_fts) as rank
from chunks_fts
where chunks_fts match ?
order by rank
limit ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var rank float64
		if err := rows.Scan(&result.SessionID, &result.Role, &result.Kind, &result.Snippet, &rank); err != nil {
			return nil, err
		}
		result.Score = 1000 - int(rank*100)
		if result.Role == "user" {
			result.Score += 50
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results, nil
}

func searchLike(db *sql.DB, query string, limit int) ([]SearchResult, error) {
	rows, err := db.Query(`
select session_id, role, kind, text
from chunks
where lower(text) like ?
limit ?`, "%"+strings.ToLower(query)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var text string
		if err := rows.Scan(&result.SessionID, &result.Role, &result.Kind, &text); err != nil {
			return nil, err
		}
		result.Snippet = snippet(text, query)
		result.Score = 100
		if result.Role == "user" {
			result.Score += 50
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func searchLikeSession(db *sql.DB, sessionID, query string, limit int) ([]sessions.Line, error) {
	rows, err := db.Query(`
select role, text
from chunks
where session_id = ? and lower(text) like ?
order by ordinal
limit ?`, sessionID, "%"+strings.ToLower(query)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var lines []sessions.Line
	for rows.Next() {
		var line sessions.Line
		if err := rows.Scan(&line.Role, &line.Text); err != nil {
			return nil, err
		}
		line.Text = snippet(line.Text, query)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func ftsQuery(query string) string {
	terms := strings.Fields(query)
	kept := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.Trim(term, `"*'():`)
		if len([]rune(term)) < 2 {
			continue
		}
		kept = append(kept, `"`+strings.ReplaceAll(term, `"`, `""`)+`"*`)
	}
	return strings.Join(kept, " ")
}

func snippet(text, query string) string {
	lower := strings.ToLower(text)
	needle := strings.ToLower(strings.TrimSpace(query))
	index := strings.Index(lower, needle)
	if index < 0 {
		return oneLine(text, 240)
	}
	start := max(0, index-80)
	end := min(len(text), index+len(needle)+140)
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(text) {
		suffix = "..."
	}
	return prefix + oneLine(text[start:end], 240) + suffix
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
