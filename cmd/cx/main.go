package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/picker"
	"github.com/ardasevinc/cx/internal/sessions"
)

const version = "0.0.0-dev"

func main() {
	run(os.Args[1:], os.Stdout, os.Stderr)
}

func run(args []string, stdout io.Writer, stderr io.Writer) {
	flags := flag.NewFlagSet("cx", flag.ExitOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print version and exit")
	showVersionShort := flags.Bool("V", false, "print version and exit")
	codexHome := flags.String("codex-home", "", "override Codex home directory")
	listOnly := flags.Bool("list", false, "print indexed sessions and exit")
	limit := flags.Int("limit", 20, "maximum sessions to print with --list")
	noAltScreen := flags.Bool("no-alt-screen", false, "run the picker without alternate screen mode")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: cx [--list] [--limit N] [--codex-home DIR] [--no-alt-screen]")
		_, _ = fmt.Fprintln(stderr, "       cx list [--limit N]")
		_, _ = fmt.Fprintln(stderr, "       cx version | cx --version | cx -V")
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

func printList(out io.Writer, items []sessions.Session, limit int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", item.ID, item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.Project, item.Title)
	}
}

func runCodex(result picker.Result) error {
	args := []string{string(result.Action), result.Session.ID}
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
