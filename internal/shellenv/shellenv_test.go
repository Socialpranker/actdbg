package shellenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Socialpranker/actdbg/internal/state"
)

func TestParseGithubEnvFile(t *testing.T) {
	in := "FOO=bar\nMULTI<<EOF\nline1\nline2\nEOF\nBAZ=with=equals\n\n"
	got := ParseGithubEnvFile(in)
	if got["FOO"] != "bar" {
		t.Errorf("FOO=%q", got["FOO"])
	}
	if got["MULTI"] != "line1\nline2" {
		t.Errorf("MULTI=%q", got["MULTI"])
	}
	if got["BAZ"] != "with=equals" {
		t.Errorf("BAZ=%q", got["BAZ"])
	}
}

func TestBuildScriptQuotingAndPrecedence(t *testing.T) {
	st := &state.State{
		StepName: "Build", StepNumber: 2, TotalSteps: 5, JobID: "test",
		Env: map[string]string{
			"STATIC":  "from-yaml",
			"TRICKY":  "it's a 'quote'",
			"EXPR":    "${{ secrets.X }}",
			"DYN_WIN": "static-version",
		},
	}
	dyn := map[string]string{"DYN_WIN": "dynamic-version", "ADDED": "later"}
	s := BuildScript(st, dyn, []string{"/opt/tool/bin"})

	for _, want := range []string{
		`export STATIC='from-yaml'`,
		`export TRICKY='it'\''s a '\''quote'\'''`,
		`export DYN_WIN='dynamic-version'`, // dynamic wins
		`export ADDED='later'`,
		`export PATH='/opt/tool/bin':"$PATH"`,
		"export GITHUB_ENV=" + EnvsFilePath,
		"1 value(s) contain unevaluated",
		`step 2/5 "Build"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q\n---\n%s", want, s)
		}
	}
}

func TestCollectSecrets(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sec")
	os.WriteFile(f, []byte("# comment\nA=1\nB=\"two\"\n"), 0o600)
	got, err := CollectSecrets(f, []string{"B=override", "C=3"})
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "1" || got["B"] != "override" || got["C"] != "3" {
		t.Errorf("got %+v", got)
	}
	if _, err := CollectSecrets("", []string{"broken"}); err == nil {
		t.Error("want error for KEY without =")
	}
}
