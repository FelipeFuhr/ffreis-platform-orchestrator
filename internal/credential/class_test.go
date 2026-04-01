package credential

import "testing"

func TestClassString(t *testing.T) {
	tests := []struct {
		name string
		in   Class
		want string
	}{
		{"root", ClassRoot, "root"},
		{"admin", ClassAdmin, "admin"},
		{"operator", ClassOperator, "operator"},
		{"unknown", Class(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
