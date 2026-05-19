package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/launcher"
	"github.com/ardasevinc/cx/internal/picker"
	"github.com/ardasevinc/cx/internal/sessions"
)

const version = "0.0.0-dev"

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

	model := picker.New(items)
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
  mouse wheel         Move selection.
  pgup/pgdn home/end  Jump around the list.
  enter               Run codex --yolo -C <cwd> resume <session-id>.
  ^f                  Run codex --yolo -C <cwd> fork <session-id>.
  y                   Copy selected session id.
  :                   Command mode.
  ?                   Help overlay.
  tab                 Toggle preview panel.
  ^e                  Toggle detail view.
  ^v                  Toggle compact/comfy rows.
  esc, ^c             Exit.

Commands:
  :resume
  :fork
  :copy id|path|cwd|title|resume|fork
  :view compact|comfy
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

func printList(out io.Writer, items []sessions.Session, limit int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", item.ID, item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.Project, item.Title)
	}
}

func runCodex(result picker.Result) error {
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
