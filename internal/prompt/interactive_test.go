package prompt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestNewInteractivePrompter(t *testing.T) {
	p := NewInteractivePrompter()
	if p == nil || p.in == nil || p.out == nil {
		t.Fatal("expected initialized prompter")
	}
}

func TestInteractivePrompterAsk_UsesDefaultOnEmpty(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("\n"))
	var out bytes.Buffer

	p := &InteractivePrompter{in: in, out: &out}
	spec := InputSpec{Key: "k", Default: "d"}

	got, err := p.Ask(context.Background(), spec)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "d" {
		t.Fatalf("expected %q, got %q", "d", got)
	}
}

func TestInteractivePrompterAsk_RetriesUntilValid(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("\ninvalid\nvalid\n"))
	var out bytes.Buffer
	p := &InteractivePrompter{in: in, out: &out}

	got, err := p.Ask(context.Background(), InputSpec{
		Key: "k",
		Validate: func(v string) error {
			if v != "valid" {
				return errors.New("bad")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if got != "valid" {
		t.Fatalf("Ask() = %q", got)
	}
}

func TestInteractivePrompterAsk_RequiredRetryAndConfirmError(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("\nvalue\n"))
	var out bytes.Buffer
	p := &InteractivePrompter{in: in, out: &out}

	got, err := p.Ask(context.Background(), InputSpec{Key: "k"})
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if got != "value" {
		t.Fatalf("Ask() = %q", got)
	}

	p = &InteractivePrompter{in: bufio.NewReader(errReader{}), out: io.Discard}
	if _, err := p.Confirm(context.Background(), "confirm?"); err == nil {
		t.Fatal("expected confirm read error")
	}
}

func TestInteractivePrompterConfirm_Yes(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("yes\n"))
	var out bytes.Buffer

	p := &InteractivePrompter{in: in, out: &out}
	ok, err := p.Confirm(context.Background(), "confirm?")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
}

func TestInteractiveHelpers(t *testing.T) {
	if got := promptLabel(InputSpec{Label: "Label", Key: "key"}); got != "Label" {
		t.Fatalf("promptLabel() = %q", got)
	}
	if got := promptLabel(InputSpec{Key: "key"}); got != "key" {
		t.Fatalf("promptLabel() fallback = %q", got)
	}
	if got := defaultIfEmpty("", "fallback"); got != "fallback" {
		t.Fatalf("defaultIfEmpty() = %q", got)
	}
	if got := defaultIfEmpty("value", "fallback"); got != "value" {
		t.Fatalf("defaultIfEmpty() non-empty = %q", got)
	}
}

func TestInteractivePrompterReadInputAndPrompt(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("value\n"))
	var out bytes.Buffer
	p := &InteractivePrompter{in: in, out: &out}

	p.printPrompt("Name", "default")
	if !bytes.Contains(out.Bytes(), []byte("Name [default]: ")) {
		t.Fatalf("unexpected prompt output: %q", out.String())
	}
	p.printPrompt("Name", "")
	if !bytes.Contains(out.Bytes(), []byte("Name: ")) {
		t.Fatalf("expected plain prompt output, got %q", out.String())
	}

	got, err := p.readInput("Name", false)
	if err != nil {
		t.Fatalf("readInput() error: %v", err)
	}
	if got != "value" {
		t.Fatalf("readInput() = %q", got)
	}
}

func TestInteractivePrompterReadInputError(t *testing.T) {
	p := &InteractivePrompter{in: bufio.NewReader(errReader{}), out: io.Discard}
	_, err := p.readInput("Name", false)
	if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("readInput() error = %v", err)
	}
}

func TestInteractivePrompterGate(t *testing.T) {
	var out bytes.Buffer
	p := &InteractivePrompter{in: bufio.NewReader(bytes.NewBufferString("deploy\n")), out: &out}
	if err := p.Gate(context.Background(), "Proceed", "deploy"); err != nil {
		t.Fatalf("Gate() error: %v", err)
	}

	p = &InteractivePrompter{in: bufio.NewReader(bytes.NewBufferString("nope\n")), out: io.Discard}
	if err := p.Gate(context.Background(), "Proceed", "deploy"); err == nil {
		t.Fatal("expected gate confirmation error")
	}
}

func TestInteractivePrompterConfirmNo(t *testing.T) {
	p := &InteractivePrompter{in: bufio.NewReader(bytes.NewBufferString("no\n")), out: io.Discard}
	ok, err := p.Confirm(context.Background(), "confirm?")
	if err != nil {
		t.Fatalf("Confirm() error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}
