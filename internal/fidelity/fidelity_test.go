package fidelity

import (
	"strings"
	"testing"
)

const wfFull = `
name: ci
run-name: deploy by @${{ github.actor }}
concurrency: prod
permissions:
  id-token: write
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    environment: production
    timeout-minutes: 30
    continue-on-error: true
    services:
      db:
        image: postgres
    steps:
      - uses: actions/checkout@v4
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - uses: actions/cache@v4
      - uses: actions/upload-artifact@v4
      - uses: aws-actions/configure-aws-credentials@v4
      - run: echo done >> $GITHUB_STEP_SUMMARY
  win:
    runs-on: windows-latest
    steps:
      - run: echo hi
`

func levels(fs []Finding) map[Level]int {
	m := map[Level]int{}
	for _, f := range fs {
		m[f.Level]++
	}
	return m
}

func mustCheck(t *testing.T, src string) []Finding {
	t.Helper()
	fs, err := Check("wf.yml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestCheckFindsKnownDivergences(t *testing.T) {
	fs := mustCheck(t, wfFull)
	wantSubstr := []string{
		"concurrency", "permissions", "environment", "timeout-minutes",
		"continue-on-error", "services", "windows", "GITHUB_TOKEN",
		"OIDC", "actions/cache", "artifact", "GITHUB_STEP_SUMMARY", "id-token",
	}
	joined := ""
	for _, f := range fs {
		joined += f.What + "\n"
	}
	for _, w := range wantSubstr {
		if !strings.Contains(strings.ToLower(joined), strings.ToLower(w)) {
			t.Errorf("expected a finding mentioning %q; got:\n%s", w, joined)
		}
	}
	lv := levels(fs)
	if lv[ERR] < 2 { // windows runner + OIDC (id-token и aws-actions могут схлопнуться в один — минимум 2)
		t.Errorf("want >=2 ERR findings, got %d", lv[ERR])
	}
}

func TestCheckLineNumbersPresent(t *testing.T) {
	for _, f := range mustCheck(t, wfFull) {
		if f.Line <= 0 {
			t.Errorf("finding %q has no line number", f.What)
		}
	}
}

func TestCleanWorkflowIsQuiet(t *testing.T) {
	clean := `
name: ok
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	fs := mustCheck(t, clean)
	if len(fs) != 0 {
		t.Errorf("clean workflow should produce no findings, got %+v", fs)
	}
	if !strings.Contains(Render(fs), "no known") {
		t.Error("Render(empty) should say no known divergences")
	}
}

func TestSummaryCounts(t *testing.T) {
	fs := mustCheck(t, wfFull)
	s := Summary(fs)
	if !strings.Contains(s, "differently") || !strings.Contains(s, "actdbg check") {
		t.Errorf("summary unexpected: %q", s)
	}
	if Summary(nil) != "" {
		t.Error("empty summary expected for no findings")
	}
}
