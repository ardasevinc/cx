package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultPreviewLimit = 24
	defaultIndexBytes   = 256 * 1024
	maxIndexLineBytes   = 256 * 1024
)

type Session struct {
	ID         string
	Title      string
	Preview    string
	CWD        string
	Project    string
	Source     string
	Path       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Turns      int
	TokensUsed int64
	Transcript []Line
	SearchText string
}

type Line struct {
	Role string
	Text string
}

type Options struct {
	Home         string
	CodexHome    string
	PreviewLimit int
}

func Load(opts Options) ([]Session, error) {
	codexHome := opts.CodexHome
	if codexHome == "" {
		home := opts.Home
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return nil, err
			}
		}
		codexHome = filepath.Join(home, ".codex")
	}

	if sessions, err := loadStateDB(codexHome); err == nil && len(sessions) > 0 {
		return sessions, nil
	}
	return loadJSONL(codexHome, opts)
}

func loadJSONL(codexHome string, opts Options) ([]Session, error) {
	sessionsRoot := filepath.Join(codexHome, "sessions")
	limit := opts.PreviewLimit
	if limit <= 0 {
		limit = defaultPreviewLimit
	}

	var out []Session
	err := filepath.WalkDir(sessionsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}

		session, err := parseFile(path, limit)
		if err != nil || session.ID == "" {
			return nil
		}
		out = append(out, session)
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("codex sessions directory not found: %s", sessionsRoot)
		}
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

type dbThread struct {
	ID               string `json:"id"`
	RolloutPath      string `json:"rollout_path"`
	CreatedAtMS      int64  `json:"created_at_ms"`
	UpdatedAtMS      int64  `json:"updated_at_ms"`
	Source           string `json:"source"`
	CWD              string `json:"cwd"`
	Title            string `json:"title"`
	FirstUserMessage string `json:"first_user_message"`
	ThreadSource     string `json:"thread_source"`
	Preview          string `json:"preview"`
	TokensUsed       int64  `json:"tokens_used"`
	AgentNickname    string `json:"agent_nickname"`
	AgentRole        string `json:"agent_role"`
}

func loadStateDB(codexHome string) ([]Session, error) {
	dbPath := filepath.Join(codexHome, "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, err
	}

	query := `
select
  id,
  rollout_path,
  coalesce(created_at_ms, created_at * 1000) as created_at_ms,
  coalesce(updated_at_ms, updated_at * 1000) as updated_at_ms,
  source,
  cwd,
  title,
  first_user_message,
  coalesce(thread_source, '') as thread_source,
  preview,
  tokens_used,
  coalesce(agent_nickname, '') as agent_nickname,
  coalesce(agent_role, '') as agent_role
from threads
where archived = 0
order by updated_at_ms desc, id desc;
`
	cmd := exec.Command("sqlite3", "-json", dbPath, query)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var rows []dbThread
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		session := sessionFromDB(row)
		if session.ID == "" {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func sessionFromDB(row dbThread) Session {
	title := firstNonEmpty(row.Title, row.FirstUserMessage, row.Preview)
	session := Session{
		ID:         row.ID,
		Title:      oneLine(title, 96),
		Preview:    oneLine(firstNonEmpty(row.Preview, row.FirstUserMessage, row.Title), 160),
		CWD:        row.CWD,
		Project:    projectName(row.CWD),
		Source:     firstNonEmpty(row.ThreadSource, sourceLabelFromRaw(row.Source)),
		Path:       row.RolloutPath,
		CreatedAt:  unixMilli(row.CreatedAtMS),
		UpdatedAt:  unixMilli(row.UpdatedAtMS),
		TokensUsed: row.TokensUsed,
	}
	if session.Title == "" {
		session.Title = fallbackTitle(session)
	}
	if session.Preview == "" {
		session.Preview = session.Title
	}
	if row.AgentNickname != "" || row.AgentRole != "" {
		session.Preview = strings.TrimSpace(session.Preview + " " + row.AgentNickname + " " + row.AgentRole)
	}
	if row.FirstUserMessage != "" {
		session.Transcript = append(session.Transcript, Line{Role: "user", Text: oneLine(row.FirstUserMessage, 600)})
	}
	if row.Preview != "" && row.Preview != row.FirstUserMessage {
		session.Transcript = append(session.Transcript, Line{Role: "preview", Text: oneLine(row.Preview, 600)})
	}
	session.SearchText = buildSearchText(session)
	return session
}

func unixMilli(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func Filter(all []Session, query string) []Session {
	query = normalizeQuery(query)
	if query == "" {
		return all
	}

	type scored struct {
		session Session
		score   int
	}
	matches := make([]scored, 0, len(all))
	for _, session := range all {
		score := matchScore(session.SearchText, query)
		if score > 0 {
			matches = append(matches, scored{session: session, score: score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	filtered := make([]Session, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, match.session)
	}
	return filtered
}

type rolloutItem struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID           string          `json:"id"`
	Timestamp    string          `json:"timestamp"`
	CWD          string          `json:"cwd"`
	Source       json.RawMessage `json:"source"`
	ThreadSource string          `json:"thread_source"`
}

type turnContext struct {
	CWD string `json:"cwd"`
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

func parseFile(path string, previewLimit int) (Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return Session{}, err
	}

	session := Session{Path: path, UpdatedAt: info.ModTime()}
	if err := scanPrefix(file, defaultIndexBytes, func(line []byte) {
		applyRolloutLine(&session, line, previewLimit)
	}); err != nil {
		return Session{}, err
	}

	if session.ID == "" {
		return Session{}, nil
	}
	if session.Title == "" {
		session.Title = fallbackTitle(session)
	}
	if session.Preview == "" {
		session.Preview = session.Title
	}
	session.Project = projectName(session.CWD)
	session.SearchText = buildSearchText(session)
	return session, nil
}

func scanPrefix(file *os.File, maxBytes int, fn func([]byte)) error {
	reader := bufio.NewReaderSize(file, 64*1024)
	var line []byte
	oversized := false
	read := 0

	for read < maxBytes {
		chunk, isPrefix, err := reader.ReadLine()
		read += len(chunk)
		if len(line)+len(chunk) > maxIndexLineBytes {
			oversized = true
			line = nil
		}
		if !oversized {
			line = append(line, chunk...)
		}
		if isPrefix {
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			continue
		}

		if len(line) > 0 && !oversized {
			fn(line)
		}
		line = nil
		oversized = false

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func applyRolloutLine(session *Session, raw []byte, previewLimit int) {
	line := bytes.TrimSpace(raw)
	if len(line) == 0 {
		return
	}

	var item rolloutItem
	if err := json.Unmarshal(line, &item); err != nil {
		return
	}

	switch item.Type {
	case "session_meta":
		applySessionMeta(session, item.Payload)
	case "turn_context":
		applyTurnContext(session, item.Payload)
	case "response_item":
		applyResponseItem(session, item.Payload, previewLimit)
	}
}

func applySessionMeta(session *Session, payload json.RawMessage) {
	var meta sessionMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return
	}
	session.ID = meta.ID
	session.CWD = meta.CWD
	session.Source = sourceLabel(meta)
	if created, ok := parseTime(meta.Timestamp); ok {
		session.CreatedAt = created
	}
}

func applyTurnContext(session *Session, payload json.RawMessage) {
	var turn turnContext
	if err := json.Unmarshal(payload, &turn); err != nil {
		return
	}
	if turn.CWD != "" {
		session.CWD = turn.CWD
	}
}

func applyResponseItem(session *Session, payload json.RawMessage, previewLimit int) {
	var item responseItem
	if err := json.Unmarshal(payload, &item); err != nil {
		return
	}
	if item.Type != "message" || !isHumanRole(item.Role) {
		return
	}

	text := cleanText(contentText(item.Content))
	if text == "" || isInfrastructureText(text) {
		return
	}

	session.Turns++
	if session.Title == "" && item.Role == "user" {
		session.Title = oneLine(text, 96)
	}
	if item.Role == "assistant" {
		session.Preview = oneLine(text, 160)
	}
	if len(session.Transcript) < previewLimit {
		session.Transcript = append(session.Transcript, Line{
			Role: item.Role,
			Text: oneLine(text, 600),
		})
	}
}

func sourceLabel(meta sessionMeta) string {
	if meta.ThreadSource != "" {
		return meta.ThreadSource
	}
	source := strings.TrimSpace(string(meta.Source))
	if source == "" || source == "null" {
		return "unknown"
	}
	if strings.HasPrefix(source, `"`) {
		var s string
		if err := json.Unmarshal(meta.Source, &s); err == nil {
			return s
		}
	}
	if strings.Contains(source, "subagent") {
		return "subagent"
	}
	return "unknown"
}

func sourceLabelFromRaw(source string) string {
	if source == "" {
		return "unknown"
	}
	if strings.Contains(source, "subagent") {
		return "subagent"
	}
	if source == "cli" {
		return "cli"
	}
	return "unknown"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	return t, err == nil
}

func isHumanRole(role string) bool {
	return role == "user" || role == "assistant"
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
	return strings.TrimSpace(strings.Join(kept, " "))
}

func isInfrastructureText(text string) bool {
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

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func fallbackTitle(session Session) string {
	if session.CWD != "" {
		return filepath.Base(session.CWD)
	}
	return session.ID
}

func projectName(cwd string) string {
	if cwd == "" {
		return "unknown"
	}

	home, err := os.UserHomeDir()
	if err == nil {
		codexDocs := filepath.Join(home, "Documents", "Codex")
		if rel, err := filepath.Rel(codexDocs, cwd); err == nil && !strings.HasPrefix(rel, "..") {
			return "chats"
		}
	}

	base := filepath.Base(cwd)
	if base == "." || base == string(filepath.Separator) {
		return cwd
	}
	return base
}

func buildSearchText(session Session) string {
	var b strings.Builder
	fields := []string{
		session.ID,
		session.Title,
		session.Preview,
		session.CWD,
		session.Project,
		session.Source,
		session.Path,
	}
	for _, field := range fields {
		b.WriteString(field)
		b.WriteByte('\n')
	}
	for _, line := range session.Transcript {
		b.WriteString(line.Role)
		b.WriteByte(' ')
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return strings.ToLower(b.String())
}

func normalizeQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func matchScore(haystack, query string) int {
	if query == "" {
		return 1
	}
	terms := strings.Fields(query)
	score := 0
	for _, term := range terms {
		switch {
		case strings.Contains(haystack, term):
			score += 100 + len(term)
		case len([]rune(term)) >= 3 && fuzzyContains(haystack, term):
			score += 10 + len(term)
		default:
			return 0
		}
	}
	return score
}

func fuzzyContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	needleRunes := []rune(needle)
	i := 0
	for _, r := range haystack {
		if unicode.ToLower(r) == needleRunes[i] {
			i++
			if i == len(needleRunes) {
				return true
			}
		}
	}
	return false
}
