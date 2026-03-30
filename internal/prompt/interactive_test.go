package prompt

import (
	"bufio"
	"bytes"
	"testing"
)

func TestInteractivePrompterAsk_UsesDefaultOnEmpty(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("\n"))
	var out bytes.Buffer

	p := &InteractivePrompter{in: in, out: &out}
	spec := InputSpec{Key: "k", Default: "d"}

	got, err := p.Ask(spec)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "d" {
		t.Fatalf("expected %q, got %q", "d", got)
	}
}

func TestInteractivePrompterConfirm_Yes(t *testing.T) {
	in := bufio.NewReader(bytes.NewBufferString("yes\n"))
	var out bytes.Buffer

	p := &InteractivePrompter{in: in, out: &out}
	ok, err := p.Confirm("confirm?")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
}
