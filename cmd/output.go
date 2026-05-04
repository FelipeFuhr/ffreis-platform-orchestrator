package cmd

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/ui"
)

type commandOutput struct {
	out       io.Writer
	err       io.Writer
	presenter *ui.Presenter
}

func newCommandOutput(cmd *cobra.Command, presenter *ui.Presenter) *commandOutput {
	return &commandOutput{
		out:       cmd.OutOrStdout(),
		err:       cmd.ErrOrStderr(),
		presenter: presenter,
	}
}

func (o *commandOutput) Line(text string) {
	if text == "" {
		_, _ = io.WriteString(o.out, "\n")
		return
	}
	_, _ = io.WriteString(o.out, text+"\n")
}

func (o *commandOutput) Header(title, subtitle string) {
	if o.presenter != nil {
		o.Line(o.presenter.Header(title, subtitle))
		return
	}
	o.Line(title)
	if subtitle != "" {
		o.Line(subtitle)
	}
}

func (o *commandOutput) Summary(title string, parts ...string) {
	if o.presenter != nil {
		o.Line(o.presenter.Summary(title, parts...))
		return
	}
	o.Line(title + ": " + strings.Join(parts, "  "))
}

func (o *commandOutput) StatusErr(kind, label, detail string) {
	message := detail
	if o.presenter != nil {
		message = o.presenter.Status(kind, label, detail)
	} else {
		message = "[" + label + "] " + detail
	}
	_, _ = io.WriteString(o.err, message+"\n")
}

func (o *commandOutput) Table(headers []string, rows [][]string) error {
	w := tabwriter.NewWriter(o.out, 0, 0, 2, ' ', tabwriter.StripEscape)
	_, _ = io.WriteString(w, strings.Join(headers, "\t")+"\n")
	for _, row := range rows {
		escaped := make([]string, len(row))
		for i, cell := range row {
			escaped[i] = escapeANSIForTabwriter(cell)
		}
		_, _ = io.WriteString(w, strings.Join(escaped, "\t")+"\n")
	}
	return w.Flush()
}

// ansiEscapeRe matches ANSI SGR escape sequences (e.g. \x1b[32;1m).
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// escapeANSIForTabwriter wraps ANSI escape sequences in tabwriter.Escape
// markers so that tabwriter excludes their bytes from column-width calculations
// while still passing the sequences through to the output unchanged.
func escapeANSIForTabwriter(s string) string {
	return ansiEscapeRe.ReplaceAllStringFunc(s, func(match string) string {
		return string(tabwriter.Escape) + match + string(tabwriter.Escape)
	})
}

func countPart(label string, value int) string {
	return label + "=" + strconv.Itoa(value)
}

func runSummary(runID, project, env string) string {
	return "run " + runID + " for " + project + "/" + env
}

func viaBinary(step, binary string) string {
	return step + " via " + binary
}

func failureDetail(step string, err error) string {
	return step + ": " + err.Error()
}

func stepStatusBadge(presenter *ui.Presenter, status pipeline.Status) string {
	if presenter == nil {
		return "[" + strings.ToLower(string(status)) + "]"
	}

	switch status {
	case pipeline.StatusSucceeded:
		return presenter.Badge("ok", "ok")
	case pipeline.StatusSkipped:
		return presenter.Badge("muted", "skip")
	case pipeline.StatusRunning:
		return presenter.Badge("running", "...")
	case pipeline.StatusFailed:
		return presenter.Badge("error", "fail")
	default:
		return presenter.Badge("info", strings.ToLower(string(status)))
	}
}
