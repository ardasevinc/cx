package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/picker"
	"github.com/ardasevinc/cx/internal/sessions"
)

const version = "0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	codexHome := flag.String("codex-home", "", "override Codex home directory")
	listOnly := flag.Bool("list", false, "print indexed sessions and exit")
	limit := flag.Int("limit", 20, "maximum sessions to print with --list")
	noAltScreen := flag.Bool("no-alt-screen", false, "run the picker without alternate screen mode")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
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
		printList(items, *limit)
		return
	}

	model := picker.New(items)
	options := []tea.ProgramOption{tea.WithOutput(os.Stderr)}
	if !*noAltScreen {
		options = append(options, tea.WithAltScreen())
	}
	finalModel, err := tea.NewProgram(model, options...).Run()
	if err != nil {
		die(err)
	}

	result := finalModel.(picker.Model).Result()
	if result.Action == picker.ActionNone {
		return
	}
	if err := runCodex(result); err != nil {
		die(err)
	}
}

func printList(items []sessions.Session, limit int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.Project, item.Title)
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
