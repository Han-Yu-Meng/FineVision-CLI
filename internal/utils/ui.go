package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
)

func init() {
	color.NoColor = false

	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}
}

func makeColorFunc(attrs ...color.Attribute) func(format string, a ...interface{}) string {
	c := color.New(attrs...)
	c.EnableColor()
	return c.SprintfFunc()
}

var (
	HeaderColor  = makeColorFunc(color.FgHiCyan, color.Bold)
	SuccessColor = makeColorFunc(color.FgHiGreen, color.Bold)
	ErrorColor   = makeColorFunc(color.FgHiRed, color.Bold)
	WarningColor = makeColorFunc(color.FgHiYellow, color.Bold)
	InfoColor    = makeColorFunc(color.FgHiBlue, color.Bold)
	DimColor     = makeColorFunc(color.FgHiBlack)
)

func LogSection(w io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(w, "%s %s\n", InfoColor("> "), InfoColor(msg))
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

func LogInfo(w io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(w, "%s %s\n", InfoColor("- "), InfoColor(msg))
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

func LogSuccess(w io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(w, "%s %s\n", SuccessColor("✔ "), SuccessColor(msg))
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

func LogError(w io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(w, "%s %s\n", ErrorColor("✘ "), ErrorColor(msg))
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

func LogWarning(w io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	yellow := makeColorFunc(color.FgHiYellow, color.Bold)
	fmt.Fprintf(w, "%s %s\n", WarningColor("! "), yellow(msg))
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

const (
	stateNormal = iota
	stateWarning
	stateError
)

var (
	reProgress      = regexp.MustCompile(`^\[\d+/\d+\]`)
	reHeaderError   = regexp.MustCompile(`(?i): (error|fatal error):`)
	reHeaderWarning = regexp.MustCompile(`(?i): warning:`)
)

type IndentedWriter struct {
	Target io.Writer
	Prefix string
	state  int
}

func NewBuildWriter(w io.Writer) *IndentedWriter {
	return &IndentedWriter{
		Target: w,
		Prefix: DimColor("   | "),
		state:  stateNormal,
	}
}

func (iw *IndentedWriter) Write(p []byte) (n int, err error) {
	lines := bytes.Split(p, []byte{'\n'})

	for i, line := range lines {
		if i == len(lines)-1 && len(line) == 0 {
			continue
		}

		lineStr := string(line)
		trimmedLine := strings.TrimSpace(lineStr)

		if reProgress.MatchString(trimmedLine) {
			iw.state = stateNormal
		} else if reHeaderError.MatchString(lineStr) {
			iw.state = stateError
		} else if reHeaderWarning.MatchString(lineStr) {
			iw.state = stateWarning
		}

		var finalOutput string
		switch iw.state {
		case stateError:
			finalOutput = ErrorColor(lineStr)
		case stateWarning:
			finalOutput = WarningColor(lineStr)
		default:
			finalOutput = lineStr
		}

		fmt.Fprintf(iw.Target, "%s%s\n", iw.Prefix, finalOutput)

		if f, ok := iw.Target.(interface{ Flush() }); ok {
			f.Flush()
		}
	}

	return len(p), nil
}

func (iw *IndentedWriter) Flush() {
	if f, ok := iw.Target.(interface{ Flush() }); ok {
		f.Flush()
	}
}
