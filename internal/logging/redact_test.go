package logging

import "testing"

func TestRedact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"one char", "X", "*"},
		{"two chars", "12", "**"},
		{"three chars", "123", "***"},
		{"four chars", "1234", "****"},
		{"five chars", "12345", "*2345"},
		{"routing number", "021000021", "*****0021"},
		{"account number", "1234567890", "******7890"},
		{"short account", "98765", "*8765"},
		{"check number", "1001", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedact_PreservesLastFour(t *testing.T) {
	input := "9876543210"
	got := Redact(input)
	if got[len(got)-4:] != "3210" {
		t.Errorf("last 4 digits not preserved: got %q", got)
	}
	// Verify masked portion is all asterisks
	for i := 0; i < len(got)-4; i++ {
		if got[i] != '*' {
			t.Errorf("position %d should be '*', got '%c'", i, got[i])
		}
	}
}
