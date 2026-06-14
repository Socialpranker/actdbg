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

func TestMergeDynamicEnv(t *testing.T) {
	// The bug this fixes: act zeroes /var/run/act/workflow/envs.txt at the start
	// of every step, so if the step that *failed* wrote nothing to $GITHUB_ENV,
	// the live file is empty and a var an earlier step exported is lost in the
	// shell. The accumulated snapshot deltas must fill that gap.
	accumulated := map[string]string{
		"FROM_STEP1": "one",
		"FROM_STEP2": "two",
		"OVERLAP":    "accumulated-stale",
	}

	t.Run("empty live file falls back to accumulated", func(t *testing.T) {
		got := MergeDynamicEnv("", accumulated)
		if got["FROM_STEP1"] != "one" || got["FROM_STEP2"] != "two" {
			t.Errorf("earlier-step vars lost: %+v", got)
		}
	})

	t.Run("live file wins where present (fresher)", func(t *testing.T) {
		live := "OVERLAP=live-fresh\nNEW_THIS_STEP=here"
		got := MergeDynamicEnv(live, accumulated)
		if got["OVERLAP"] != "live-fresh" {
			t.Errorf("live should win for OVERLAP, got %q", got["OVERLAP"])
		}
		if got["NEW_THIS_STEP"] != "here" {
			t.Errorf("live-only var missing: %+v", got)
		}
		if got["FROM_STEP1"] != "one" {
			t.Errorf("accumulated var dropped when live non-empty: %+v", got)
		}
	})

	t.Run("nil accumulated is safe", func(t *testing.T) {
		got := MergeDynamicEnv("X=1", nil)
		if got["X"] != "1" {
			t.Errorf("got %+v", got)
		}
	})
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
