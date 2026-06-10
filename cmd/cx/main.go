package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/indexer"
	"github.com/ardasevinc/cx/internal/launcher"
	"github.com/ardasevinc/cx/internal/picker"
	"github.com/ardasevinc/cx/internal/sessions"
	"github.com/ardasevinc/cx/internal/updater"
)

const version = "v0.1.2"

func main() {
	run(os.Args[1:], os.Stdout, os.Stderr)
}

func run(args []string, stdout io.Writer, stderr io.Writer) {
	if len(args) > 0 && args[0] == "new" {
		if err := runNew(args[1:], stdout, stderr); err != nil {
			die(err)
		}
		return
	}
	if len(args) > 0 && args[0] == "update" {
		if err := runUpdate(args[1:], stdout, stderr); err != nil {
			die(err)
		}
		return
	}
	if len(args) > 0 && args[0] == "index" {
		if err := runIndex(args[1:], stdout, stderr); err != nil {
			die(err)
		}
		return
	}
	if len(args) > 0 && args[0] == "doctor" {
		if err := runDoctor(args[1:], stdout, stderr); err != nil {
			die(err)
		}
		return
	}

	flags := flag.NewFlagSet("cx", flag.ExitOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print version and exit")
	showVersionShort := flags.Bool("V", false, "print version and exit")
	codexHome := flags.String("codex-home", "", "override Codex home directory")
	listOnly := flags.Bool("list", false, "print indexed sessions and exit")
	limit := flags.Int("limit", 20, "maximum sessions to print with --list")
	noAltScreen := flags.Bool("no-alt-screen", false, "run the picker without alternate screen mode")
	flags.Usage = func() {
		printUsage(stderr)
	}
	args = normalizeCommandArgs(args)
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}

	if *showVersion || *showVersionShort {
		_, _ = fmt.Fprintln(stdout, version)
		return
	}

	items, err := sessions.Load(sessions.Options{CodexHome: *codexHome})
	if err != nil {
		die(err)
	}
	if len(items) == 0 {
		die(fmt.Errorf("no Codex sessions found"))
	}

	if *listOnly {
		printList(stdout, items, *limit)
		return
	}

	model := picker.NewWithIndex(items, indexer.Options{CodexHome: *codexHome})
	options := []tea.ProgramOption{tea.WithOutput(os.Stderr), tea.WithMouseCellMotion()}
	if !*noAltScreen {
		options = append(options, tea.WithAltScreen())
	}
	finalModel, err := tea.NewProgram(model, options...).Run()
	if err != nil {
		die(err)
	}

	result := finalModel.(picker.Model).Result()
	if result.Action == picker.ActionNone || result.Action == picker.ActionQuit {
		return
	}
	if err := runCodex(result); err != nil {
		die(err)
	}
}

func normalizeCommandArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	switch args[0] {
	case "ls", "list":
		return append([]string{"--list"}, args[1:]...)
	case "v", "version":
		return append([]string{"--version"}, args[1:]...)
	case "h", "help":
		return append([]string{"--help"}, args[1:]...)
	default:
		return args
	}
}

func printUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, `cx - Codex session picker

Usage:
  cx [flags]
  cx list [--limit N]
  cx new [name]
  cx new --cwd DIR
  cx index status|refresh|rebuild|vacuum
  cx doctor
  cx update [--check]
  cx version | cx --version | cx -V

Flags:
  --codex-home DIR    Read Codex state from DIR instead of ~/.codex.
  --list              Print sessions and exit.
  --limit N           Maximum sessions for --list. Default: 20.
  --no-alt-screen     Render in the current terminal buffer.
  --version, -V       Print version and exit.
  --help              Show this help.

TUI keys:
  type                Search immediately. Printable j/k search too.
  arrows, ^j/^k       Move selection.
  left/right          Close/open selected group in grouped views.
  mouse wheel         Move selection.
  pgup/pgdn home/end  Jump around the list.
  enter               Resume session, start selected new chat/project, or toggle group.
  ^n                  Start fresh Codex in selected session/project context.
  ^p                  Show project launcher.
  ^g                  Show grouped projects.
  ^f                  Run codex --yolo -C <cwd> fork <session-id>.
  y                   Copy selected session id.
  :                   Command mode.
  ?                   Help overlay.
  tab                 Toggle preview panel.
  ^e                  Toggle detail view.
  ^v                  Toggle compact/comfy rows.
  esc, ^c             Exit.

Commands:
  :new [name]
  :resume
  :fork
  :copy id|path|cwd|title|resume|fork
  :view all|chats|projects|grouped|compact|comfy
  :group projects|chats
  :open | :close | :toggle | :open-all | :close-all
  :preview
  :detail
  :clear
  :quit`)
}

func runNew(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("cx new", flag.ExitOnError)
	flags.SetOutput(stderr)
	cwd := flags.String("cwd", "", "start a fresh Codex thread in an existing project directory")
	printOnly := flags.Bool("print-dir", false, "print the launch directory without starting Codex")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, `cx new - start a fresh Codex thread

Usage:
  cx new [name]
  cx new --cwd DIR

Behavior:
  cx new [name] creates a fresh dated chat directory under
  ~/Documents/Codex/YYYY-MM-DD/ and runs:

    codex --yolo -C <created-dir>

  cx new --cwd DIR starts Codex in an existing project directory with:

    codex --yolo -C <dir>

Flags:
  --cwd DIR      Existing project directory to start from.
  --print-dir   Print the resolved directory and do not launch Codex.
  --help        Show this help.`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}

	var dir string
	if *cwd != "" {
		if flags.NArg() > 0 {
			return fmt.Errorf("cx new --cwd does not accept a chat name")
		}
		resolved, err := launcher.ProjectDir(*cwd)
		if err != nil {
			return err
		}
		dir = resolved
	} else {
		name := strings.Join(flags.Args(), " ")
		plan, err := launcher.PlanChat(name, launcher.Options{})
		if err != nil {
			return err
		}
		dir = plan.Dir
	}

	if *printOnly {
		_, _ = fmt.Fprintln(stdout, dir)
		return nil
	}
	return runCodexFresh(dir)
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("cx update", flag.ExitOnError)
	flags.SetOutput(stderr)
	checkOnly := flags.Bool("check", false, "check the latest tagged cx version without installing")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, `cx update - check or install the latest tagged cx release

Usage:
  cx update
  cx update --check

Behavior:
  cx update --check compares this binary against the latest GitHub tag.
  cx update installs the latest GitHub tag with:

    go install github.com/ardasevinc/cx/cmd/cx@<latest-tag>

Flags:
  --check      Check for updates without installing.
  --help       Show this help.`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("cx update does not accept positional arguments")
	}

	ctx := context.Background()
	check, err := updater.CheckLatest(ctx, version)
	if err != nil {
		return err
	}
	printUpdateCheck(stdout, check)
	if *checkOnly || check.Status == updater.Current || check.Status == updater.Ahead {
		return nil
	}

	_, _ = fmt.Fprintf(stdout, "installing %s...\n", check.Latest)
	if err := updater.Install(ctx, check.Latest, os.Stdout, os.Stderr); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "installed cx %s\n", check.Latest)
	return nil
}

func runIndex(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	command := args[0]
	flags := flag.NewFlagSet("cx index "+command, flag.ExitOnError)
	flags.SetOutput(stderr)
	codexHome := flags.String("codex-home", "", "override Codex home directory")
	cachePath := flags.String("cache", "", "override cx index cache path")
	jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, `cx index - inspect and maintain the local transcript cache

Usage:
  cx index status [--json]
  cx index refresh [--json]
  cx index rebuild [--json]
  cx index vacuum

Flags:
  --codex-home DIR    Read Codex state from DIR instead of ~/.codex.
  --cache PATH        Use a custom cx index cache path.
  --json              Print machine-readable JSON.
  --help              Show this help.`)
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	opts := indexer.Options{CodexHome: *codexHome, CachePath: *cachePath}
	switch command {
	case "status":
		status, err := indexer.CurrentStatus(opts)
		if err != nil {
			return err
		}
		return printIndexStatus(stdout, status, *jsonOutput)
	case "rebuild":
		result, err := indexer.Rebuild(opts)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeJSON(stdout, result)
		}
		printStatusText(stdout, result.Status)
		_, _ = fmt.Fprintf(stdout, "elapsed: %s\n", result.Elapsed.Round(10_000_000))
		_, _ = fmt.Fprintf(stdout, "indexed: %d skipped=%d\n", result.IndexedCount, result.SkippedCount)
		return nil
	case "refresh":
		result, err := indexer.Refresh(opts)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeJSON(stdout, result)
		}
		printStatusText(stdout, result.Status)
		_, _ = fmt.Fprintf(stdout, "elapsed: %s\n", result.Elapsed.Round(10_000_000))
		_, _ = fmt.Fprintf(stdout, "indexed: %d skipped=%d\n", result.IndexedCount, result.SkippedCount)
		return nil
	case "vacuum":
		if err := indexer.Vacuum(opts); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, "vacuumed cx index cache")
		return nil
	default:
		return fmt.Errorf("unknown index command: %s", command)
	}
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("cx doctor", flag.ExitOnError)
	flags.SetOutput(stderr)
	codexHome := flags.String("codex-home", "", "override Codex home directory")
	cachePath := flags.String("cache", "", "override cx index cache path")
	jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, `cx doctor - check local cx/Codex storage compatibility

Usage:
  cx doctor [--json]

Flags:
  --codex-home DIR    Read Codex state from DIR instead of ~/.codex.
  --cache PATH        Use a custom cx index cache path.
  --json              Print machine-readable JSON.
  --help              Show this help.`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	status, err := indexer.Doctor(indexer.Options{CodexHome: *codexHome, CachePath: *cachePath})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, status)
	}
	printStatusText(stdout, status)
	if len(status.Problems) == 0 {
		_, _ = fmt.Fprintln(stdout, "doctor: ok")
		return nil
	}
	_, _ = fmt.Fprintln(stdout, "doctor: problems")
	for _, problem := range status.Problems {
		_, _ = fmt.Fprintf(stdout, "- %s\n", problem)
	}
	return nil
}

func printIndexStatus(out io.Writer, status indexer.Status, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, status)
	}
	printStatusText(out, status)
	return nil
}

func printStatusText(out io.Writer, status indexer.Status) {
	_, _ = fmt.Fprintf(out, "cache:       %s\n", status.CachePath)
	_, _ = fmt.Fprintf(out, "codex home:  %s\n", status.CodexHome)
	_, _ = fmt.Fprintf(out, "state db:    %s\n", status.StateDBPath)
	_, _ = fmt.Fprintf(out, "schema:      %d\n", status.SchemaVersion)
	_, _ = fmt.Fprintf(out, "fts:         %t\n", status.FTSAvailable)
	_, _ = fmt.Fprintf(out, "sessions:    %d indexed=%d pending=%d failed=%d missing=%d truncated=%d\n",
		status.TotalSessions, status.IndexedSessions, status.PendingSessions, status.FailedSessions, status.MissingSessions, status.TruncatedSessions)
	_, _ = fmt.Fprintf(out, "chunks:      %d\n", status.ChunkCount)
	_, _ = fmt.Fprintf(out, "cache bytes: %d\n", status.CacheBytes)
	if status.LatestIndexedAt != "" {
		_, _ = fmt.Fprintf(out, "latest:      %s\n", status.LatestIndexedAt)
	}
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUpdateCheck(out io.Writer, check updater.Check) {
	_, _ = fmt.Fprintf(out, "current: %s\n", check.Current)
	_, _ = fmt.Fprintf(out, "latest:  %s\n", check.Latest)
	_, _ = fmt.Fprintf(out, "status:  %s\n", updater.StatusText(check.Status))
}

func printList(out io.Writer, items []sessions.Session, limit int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", item.ID, item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.Project, item.Title)
	}
}

func runCodex(result picker.Result) error {
	if result.Action == picker.ActionNew {
		if result.Chat {
			plan, err := launcher.PlanChat(result.Name, launcher.Options{})
			if err != nil {
				return err
			}
			return runCodexFresh(plan.Dir)
		}
		if result.Dir != "" {
			return runCodexFresh(result.Dir)
		}
		plan, err := launcher.PlanChat("", launcher.Options{})
		if err != nil {
			return err
		}
		return runCodexFresh(plan.Dir)
	}
	args := []string{"--yolo"}
	if result.Session.CWD != "" {
		args = append(args, "-C", result.Session.CWD)
	}
	args = append(args, string(result.Action), result.Session.ID)
	return runCodexArgs(args)
}

func runCodexFresh(dir string) error {
	return runCodexArgs([]string{"--yolo", "-C", dir})
}

func runCodexArgs(args []string) error {
	cmd := exec.Command("codex", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func die(err error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "unknown error"
	}
	fmt.Fprintln(os.Stderr, "cx:", message)
	os.Exit(1)
}
