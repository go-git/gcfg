package gcfg

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// gitValue returns what git config --get reports for a.k in text. If git
// rejects the text as malformed (exit 128 and a "bad config" message, which
// the rest of the message varies by release), it returns rejected true.
// Any other failure, such as a missing key or git not running, fails the test.
func gitValue(t *testing.T, text string) (val string, rejected bool) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "config", "-f", file, "--get", "a.k")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 128 && bytes.Contains(ee.Stderr, []byte("bad config")) {
			return "", true
		}
		t.Fatalf("git config: %v", err)
	}
	return strings.TrimSuffix(string(out), "\n"), false
}

// versionAtLeast reports whether the output of git --version names a
// release at or above major.minor. Output it cannot parse counts as
// older.
func versionAtLeast(out string, major, minor int) bool {
	f := strings.Fields(out)
	if len(f) < 3 || f[0] != "git" || f[1] != "version" {
		return false
	}
	parts := strings.SplitN(f[2], ".", 3)
	if len(parts) < 2 {
		return false
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return ma > major || ma == major && mi >= minor
}

func runValueCases(t *testing.T, cases []valueCase) {
	t.Helper()
	runValueCasesSince(t, 0, 0, cases)
}

// runValueCasesSince is runValueCases for rows that only git major.minor
// and newer reads this way. The gcfg side always runs; the comparison with
// git is skipped on older releases.
func runValueCasesSince(t *testing.T, major, minor int, cases []valueCase) {
	t.Helper()
	_, gitErr := exec.LookPath("git")
	var gitVer string
	if gitErr == nil {
		out, err := exec.Command("git", "--version").Output()
		if err != nil {
			gitErr = err
		}
		gitVer = string(out)
	}
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
					t.Skipf("git unusable: %v", gitErr)
				}
				if !versionAtLeast(gitVer, major, minor) {
					t.Skipf("needs git %d.%d or newer, have %s", major, minor, strings.TrimSpace(gitVer))
				}
				gv, rejected := gitValue(t, tc.text)
				if rejected != tc.fail || (!rejected && gv != tc.want) {
					t.Errorf("git disagrees with the table: value %q rejected=%v; fix the test", gv, rejected)
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

func TestValueBareCRMatchesGit(t *testing.T) {
	runValueCases(t, []valueCase{
		{"bare CR at end of quotes", "[a]\nk=\"a\r\"\n", "a\r", false},
		{"backslash CR without LF", "[a]\nk=a\\\rb\n", "", true},
		{"backslash CR without LF in quotes", "[a]\nk=\"a\\\rb\"\n", "", true},
		{"trailing CR trimmed", "[a]\nk=a\r", "a", false},
		{"trailing CR before comment", "[a]\nk=a\r;c\n", "a", false},
		{"leading bare CR dropped", "[a]\nk=\rv\n", "v", false},
		{"CRLF line end", "[a]\r\nk=a\r\n", "a", false},
		{"CRLF continuation", "[a]\r\nk=a\\\r\nb\r\n", "ab", false},
		{"CRLF continuation in quotes", "[a]\r\nk=\"a\\\r\nb\"\r\n", "ab", false},
	})
}

// Git 2.45 changed parse_value to keep whitespace inside an unquoted value
// as written. Earlier releases turn each such byte into a space, so a bare
// CR there reads as " ".
func TestValueBareCRInsideMatchesGit(t *testing.T) {
	runValueCasesSince(t, 2, 45, []valueCase{
		{"bare CR inside value", "[a]\nk=a\rb\n", "a\rb", false},
		{"bare CR before quote", "[a]\nk=a\r\"b\"\n", "a\rb", false},
		{"bare CR before CRLF continuation", "[a]\nk=a\rb\\\r\nc\n", "a\rbc", false},
		{"bare CR after CRLF continuation", "[a]\nk=a\\\r\nb\rc\n", "ab\rc", false},
	})
}

func TestGitVersionAtLeast(t *testing.T) {
	for _, tc := range []struct {
		out        string
		major, min int
		want       bool
	}{
		{"git version 2.43.0\n", 2, 45, false},
		{"git version 2.44.1\n", 2, 45, false},
		{"git version 2.45.0\n", 2, 45, true},
		{"git version 2.54.0\n", 2, 45, true},
		{"git version 3.0.0\n", 2, 45, true},
		{"git version 1.99.0\n", 2, 45, false},
		{"git version 2.45.0.rc1\n", 2, 45, true},
		{"git version 2.39.5 (Apple Git-154)\n", 2, 45, false},
		{"git version 2.47.1.windows.1\n", 2, 45, true},
		{"garbage\n", 2, 45, false},
	} {
		if got := versionAtLeast(tc.out, tc.major, tc.min); got != tc.want {
			t.Errorf("versionAtLeast(%q, %d, %d) = %v, want %v",
				tc.out, tc.major, tc.min, got, tc.want)
		}
	}
}
