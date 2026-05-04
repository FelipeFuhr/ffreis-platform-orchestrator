package prompt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	platformui "github.com/ffreis/platform-orchestrator/internal/ui"
)

// Prompter interactively collects a single value from the operator.
type Prompter interface {
	// Ask presents the spec to the operator and returns the confirmed value.
	Ask(ctx context.Context, spec InputSpec) (string, error)

	// Confirm presents a yes/no question and returns true if the operator agrees.
	Confirm(ctx context.Context, message string) (bool, error)

	// Gate requires the operator to type a specific keyword to proceed.
	Gate(ctx context.Context, message, keyword string) error
}

// InteractivePrompter reads from the terminal using plain stdio.
// It does not require a TTY library, keeping dependencies minimal.
type InteractivePrompter struct {
	in        *bufio.Reader
	out       io.Writer
	presenter *platformui.Presenter
}

const (
	errReadInput    = "read input: %w"
	errWritePrompt  = "write prompt: %w"
	requiredRetry   = "  value is required, please try again\n"
	invalidRetryFmt = "  %v, please try again\n"
)

// NewInteractivePrompter constructs a prompter that reads from os.Stdin.
func NewInteractivePrompter(presenter *platformui.Presenter) *InteractivePrompter {
	return &InteractivePrompter{
		in:        bufio.NewReader(os.Stdin),
		out:       os.Stderr, // prompts to stderr so stdout stays clean for machine output
		presenter: presenter,
	}
}

func (p *InteractivePrompter) Ask(_ context.Context, spec InputSpec) (string, error) {
	for {
		input, err := p.promptAndRead(spec)
		if err != nil {
			return "", err
		}
		input = defaultIfEmpty(input, spec.Default)

		if retry, err := p.validateInput(spec, input); err != nil {
			return "", err
		} else if retry {
			continue
		}

		return input, nil
	}
}

func (p *InteractivePrompter) promptAndRead(spec InputSpec) (string, error) {
	label := promptLabel(spec)
	// For sensitive inputs, readInput prints the label itself alongside the
	// "input will not be echoed" notice, so we skip printPrompt to avoid
	// printing the label twice.
	if !spec.Sensitive {
		if err := p.printPrompt(label, spec.Default); err != nil {
			return "", err
		}
	}
	return p.readInput(label, spec.Sensitive)
}

func (p *InteractivePrompter) validateInput(spec InputSpec, input string) (bool, error) {
	if input == "" && !spec.Optional {
		if err := p.writePrompt(requiredRetry); err != nil {
			return false, err
		}
		return true, nil
	}

	if err := spec.Verify(input); err != nil {
		if err := p.writePrompt(invalidRetryFmt, err); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (p *InteractivePrompter) writePrompt(format string, args ...any) error {
	if _, err := fmt.Fprintf(p.out, format, args...); err != nil {
		return fmt.Errorf(errWritePrompt, err)
	}
	return nil
}

func (p *InteractivePrompter) writelnPrompt() error {
	if _, err := fmt.Fprintln(p.out); err != nil {
		return fmt.Errorf(errWritePrompt, err)
	}
	return nil
}

func promptLabel(spec InputSpec) string {
	if spec.Label != "" {
		return spec.Label
	}
	return spec.Key
}

func (p *InteractivePrompter) printPrompt(label, def string) error {
	promptLabel := label
	if def != "" {
		promptLabel = fmt.Sprintf("%s [%s]", label, def)
		if p.presenter != nil {
			promptLabel = p.presenter.Status("info", ">", promptLabel)
		}
		if err := p.writePrompt("%s: ", promptLabel); err != nil {
			return err
		}
		return nil
	}
	if p.presenter != nil {
		promptLabel = p.presenter.Status("info", ">", label)
	}
	return p.writePrompt("%s: ", promptLabel)
}

func (p *InteractivePrompter) readInput(label string, sensitive bool) (string, error) {
	if sensitive {
		sensitivePrompt := "(input will not be echoed)"
		if p.presenter != nil {
			sensitivePrompt = p.presenter.Status("warn", "secret", label)
		}
		if err := p.writePrompt("%s\n%s: ", sensitivePrompt, label); err != nil {
			return "", err
		}
		raw, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if err := p.writelnPrompt(); err != nil {
			return "", err
		}
		return string(raw), nil
	}

	raw, err := p.in.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf(errReadInput, err)
	}
	return strings.TrimRight(raw, "\r\n"), nil
}

func defaultIfEmpty(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func (p *InteractivePrompter) Confirm(_ context.Context, message string) (bool, error) {
	if p.presenter != nil {
		message = p.presenter.Status("warn", "confirm", message)
	}
	if err := p.writePrompt("%s [y/N]: ", message); err != nil {
		return false, err
	}
	raw, err := p.in.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf(errReadInput, err)
	}
	answer := strings.ToLower(strings.TrimSpace(raw))
	return answer == "y" || answer == "yes", nil
}

func (p *InteractivePrompter) Gate(_ context.Context, message, keyword string) error {
	if p.presenter != nil {
		message = p.presenter.Status("warn", "confirm", message)
	}
	if err := p.writePrompt("\n%s\nType %q to confirm: ", message, keyword); err != nil {
		return err
	}
	raw, err := p.in.ReadString('\n')
	if err != nil {
		return fmt.Errorf(errReadInput, err)
	}
	input := strings.TrimSpace(raw)
	if input != keyword {
		return fmt.Errorf("gate not confirmed: expected %q, got %q", keyword, input)
	}
	return nil
}
