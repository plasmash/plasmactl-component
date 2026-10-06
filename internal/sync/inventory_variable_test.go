package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// Variables used only in Jinja statements ({% for %}, {% if %}) count as used,
// otherwise their changes never propagate to the component (e.g. a CSV template
// looping over a list variable).
func TestExtractLinesWithVariables(t *testing.T) {
	file := filepath.Join(t.TempDir(), "labels.csv.j2")
	content := "header;line\n" +
		"{% for label in account_person_labels -%}\n" +
		"{{ label.name }};1\n" +
		"{% if enable_extra %}extra{% endif %}\n" +
		"# comment with }}\n" +
		"plain line\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	lines, err := extractLinesWithVariables(file)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"{% for label in account_person_labels -%}",
		"{{ label.name }};1",
		"{% if enable_extra %}extra{% endif %}",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines %q, want %q", len(lines), lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, lines[i], want[i])
		}
	}
}
