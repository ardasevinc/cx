package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	fmt.Fprintln(os.Stderr, "cx: session picker is not implemented yet")
	fmt.Fprintln(os.Stderr, "next: build the Codex session index over ~/.codex/sessions")
	os.Exit(2)
}
