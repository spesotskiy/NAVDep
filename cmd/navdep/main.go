package main

import (
	"fmt"
	"os"

	"navdep/internal/repl"
)

func main() {
	if err := repl.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
