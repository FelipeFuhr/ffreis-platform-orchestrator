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
	in  *bufio.Reader
	out io.Writer
}

const errReadInput = "read input: %w"

// NewInteractivePrompter constructs a prompter that reads from os.Stdin.
func NewInteractivePrompter() *InteractivePrompter {
	return &InteractivePrompter{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stderr, // prompts to stderr so stdout stays clean for machine output
	}
}

func (p *InteractivePrompter) Ask(_ context.Context, spec InputSpec) (string, error) {
	for {
		label := promptLabel(spec)
		if err := p.printPrompt(label, spec.Default); err != nil {
			return "", err
		}

		input, err := p.readInput(label, spec.Sensitive)
		if err != nil {
			return "", err
		}
		input = defaultIfEmpty(input, spec.Default)

		if input == "" && !spec.Optional {
			if _, err := fmt.Fprintf(p.out, "  value is required, please try again\n"); err != nil {
				return "", fmt.Errorf("write prompt: %w", err)
			}
			continue
		}

		if err := spec.Verify(input); err != nil {
			if _, writeErr := fmt.Fprintf(p.out, "  %v, please try again\n", err); writeErr != nil {
				return "", fmt.Errorf("write prompt: %w", writeErr)
			}
			continue
		}

		return input, nil
	}
}

func promptLabel(spec InputSpec) string {
	if spec.Label != "" {
		return spec.Label
	}
	return spec.Key
}

func (p *InteractivePrompter) printPrompt(label, def string) error {
	if def != "" {
		_, err := fmt.Fprintf(p.out, "%s [%s]: ", label, def)
		return err
	}
	_, err := fmt.Fprintf(p.out, "%s: ", label)
	return err
}

func (p *InteractivePrompter) readInput(label string, sensitive bool) (string, error) {
	if sensitive {
		if _, err := fmt.Fprintf(p.out, "(input will not be echoed)\n%s: ", label); err != nil {
			return "", fmt.Errorf("write prompt: %w", err)
		}
		raw, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if _, err := fmt.Fprintln(p.out); err != nil { // newline after hidden input
			return "", fmt.Errorf("write prompt: %w", err)
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
	if _, err := fmt.Fprintf(p.out, "%s [y/N]: ", message); err != nil {
		return false, fmt.Errorf("write prompt: %w", err)
	}
	raw, err := p.in.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf(errReadInput, err)
	}
	answer := strings.ToLower(strings.TrimSpace(raw))
	return answer == "y" || answer == "yes", nil
}

func (p *InteractivePrompter) Gate(_ context.Context, message, keyword string) error {
	if _, err := fmt.Fprintf(p.out, "\n%s\nType %q to confirm: ", message, keyword); err != nil {
		return fmt.Errorf("write prompt: %w", err)
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
