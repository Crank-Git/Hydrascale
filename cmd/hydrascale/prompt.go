package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// prompter reads interactive answers from an input stream and writes questions
// to an output stream. secretFn reads a value without echoing it (masked); it is
// overridable in tests.
type prompter struct {
	in       *bufio.Reader
	out      io.Writer
	secretFn func() (string, error)
}

// newPrompter returns a prompter reading from in and writing prompts to out.
// By default secret input is read from the terminal without echo.
func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{
		in:  bufio.NewReader(in),
		out: out,
		secretFn: func() (string, error) {
			b, err := term.ReadPassword(os.Stdin.Fd())
			return string(b), err
		},
	}
}

// line asks a question and returns the trimmed response, or def when the
// response is empty.
func (p *prompter) line(question, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", question)
	}
	text, _ := p.in.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return def
	}
	return text
}

// yesNo asks a yes/no question and returns the parsed answer, or def when the
// response is empty or unrecognised.
func (p *prompter) yesNo(question string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Fprintf(p.out, "%s [%s]: ", question, hint)
	text, _ := p.in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// secret asks for a value and returns it trimmed. On a real terminal the input
// is read without echo (masked); otherwise (piped input) it is read as a normal
// line from the same buffered reader, so it stays consistent with line/yesNo.
func (p *prompter) secret(question string) (string, error) {
	fmt.Fprintf(p.out, "%s: ", question)
	if term.IsTerminal(os.Stdin.Fd()) {
		v, err := p.secretFn()
		fmt.Fprintln(p.out)
		return strings.TrimSpace(v), err
	}
	line, err := p.in.ReadString('\n')
	return strings.TrimSpace(line), err
}
