package repl

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"navdep/internal/nav"
)

const prompt = "navdep> "

// Run reads commands from in until exit, quit, or EOF.
// Command errors are written to out and do not end the session.
func Run(in io.Reader, out io.Writer) error {
	s := &session{out: out}
	sc := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, prompt)
		if !sc.Scan() {
			break
		}
		if s.execute(sc.Text()) {
			return nil
		}
	}
	return sc.Err()
}

type session struct {
	out   io.Writer
	graph *nav.Graph
}

func (s *session) execute(line string) (exit bool) {
	tokens, err := tokenize(line)
	if err != nil {
		fmt.Fprintf(s.out, "error: %s\n", err)
		return false
	}
	if len(tokens) == 0 {
		return false
	}
	switch strings.ToLower(tokens[0]) {
	case "build":
		s.cmdBuild(tokens[1:])
	case "dependents":
		s.cmdDependents(tokens[1:])
	case "help":
		s.cmdHelp(tokens[1:])
	case "exit", "quit":
		if len(tokens) != 1 {
			fmt.Fprintln(s.out, "error: exit takes no arguments")
			return false
		}
		return true
	default:
		fmt.Fprintf(s.out, "error: unknown command %q\n", tokens[0])
	}
	return false
}

func (s *session) cmdBuild(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(s.out, "error: build requires a folder")
		return
	}
	g, sum, err := nav.Build(args[0])
	if err != nil {
		fmt.Fprintf(s.out, "error: %s\n", err)
		return
	}
	// A successful build replaces the previous map. A failed build leaves it.
	s.graph = g
	for _, w := range sum.Warnings {
		fmt.Fprintf(s.out, "warning: %s\n", w)
	}
	fmt.Fprintf(s.out, "objects: %d, links: %d, unresolved: %d\n", sum.Objects, sum.Links, sum.Unresolved)
}

func (s *session) cmdDependents(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(s.out, "error: dependents requires an object key")
		return
	}
	key, ok := nav.CanonicalKey(args[0])
	if !ok {
		fmt.Fprintf(s.out, "error: invalid object key %q\n", args[0])
		return
	}
	if s.graph == nil {
		fmt.Fprintln(s.out, "error: run build before dependents")
		return
	}
	for _, d := range s.graph.Dependents(key) {
		if d.Name == "" {
			fmt.Fprintln(s.out, d.Key)
			continue
		}
		fmt.Fprintf(s.out, "%s - %s\n", d.Key, d.Name)
	}
}

func (s *session) cmdHelp(args []string) {
	if len(args) != 0 {
		fmt.Fprintln(s.out, "error: help takes no arguments")
		return
	}
	fmt.Fprint(s.out, `commands:
  build <folder>    scan a folder and replace the dependency map
  dependents <key>  list objects that reference the key (for example c12)
  help              show this help
  exit, quit        end the session
`)
}

// tokenize splits a command line on whitespace, keeping double-quoted sections intact.
func tokenize(line string) ([]string, error) {
	var tokens []string
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuote {
			if c == '"' {
				inQuote = false
				continue
			}
			b.WriteByte(c)
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case ' ', '\t', '\r':
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quote")
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens, nil
}
