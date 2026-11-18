package gcfg

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type valueCase struct {
	name string
	text string
	want string // expected value of a.k
	fail bool   // git rejects the file
}

// readValue returns the value gcfg reads for a.k from text.
func readValue(text string) (string, error) {
	var got string
	err := ReadWithCallback(bytes.NewReader([]byte(text)),
		func(s, ss, k, v string, bv bool) error {
			if s == "a" && k == "k" {
				got = v
			}
			return nil
		})
	return got, err
}

// gitValue returns what git config --get reports for a.k in text.
func gitValue(t *testing.T, text string) (string, bool) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "config", "-f", file, "--get", "a.k")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSuffix(string(out), "\n"), true
}

func runValueCases(t *testing.T, cases []valueCase) {
	t.Helper()
	_, gitErr := exec.LookPath("git")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readValue(tc.text)
			switch {
			case tc.fail && err == nil:
				t.Errorf("gcfg: expected an error, got %q", got)
			case !tc.fail && err != nil:
				t.Errorf("gcfg: unexpected error: %v", err)
			case !tc.fail && got != tc.want:
				t.Errorf("gcfg: got %q, want %q", got, tc.want)
			}
			t.Run("git", func(t *testing.T) {
				if gitErr != nil {
					t.Skip("git not found")
				}
				gv, ok := gitValue(t, tc.text)
				if ok == tc.fail || (ok && gv != tc.want) {
					t.Errorf("git disagrees with the table: value %q ok=%v; fix the test", gv, ok)
				}
			})
		})
	}
}

func TestValueLeadingWhitespaceMatchesGit(t *testing.T) {
	runValueCases(t, []valueCase{
		{"space after continuation", "[a]\nk=\\\n  v\n", "v", false},
		{"space after empty quotes", "[a]\nk=\"\"  v\n", "v", false},
		{"tab after continuation", "[a]\nk=\\\n\tv\n", "v", false},
		{"space then continuation", "[a]\nk= \\\n a\n", "a", false},
		{"continuations keep inner space", "[a]\nk=\\\n\t\\\n v w\n", "v w", false},
		{"quoted leading space kept", "[a]\nk=\"  v\"\n", "  v", false},
		{"escaped tab kept", "[a]\nk=\\t v\n", "\t v", false},
		{"space after quote content kept", "[a]\nk=\"x\" v\n", "x v", false},
		{"CR after continuation", "[a]\nk=\\\n\rv\n", "v", false},
		{"CR after empty quotes", "[a]\nk=\"\"\rv\n", "v", false},
	})
}
