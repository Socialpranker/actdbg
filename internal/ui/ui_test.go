package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Socialpranker/actdbg/internal/enginerun"
)

func TestStatusIcon(t *testing.T) {
	cases := map[string]string{
		"":        "·",
		"running": "▶",
		"success": "✅",
		"failure": "❌",
		"skipped": "⏭",
		"weird":   "·",
	}
	for in, want := range cases {
		if got := statusIcon(in); got != want {
			t.Errorf("statusIcon(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string should pass through, got %q", got)
	}
	got := truncate("hello world", 7)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > 7 {
		t.Errorf("truncate misbehaved: %q", got)
	}
	if got := truncate("anything", 0); got != "" {
		t.Errorf("width 0 should yield empty, got %q", got)
	}
}

func TestLastLinesAndWindow(t *testing.T) {
	in := []string{"a", "b", "c", "d", "e"}
	if got := lastLines(in, 2); !reflect.DeepEqual(got, []string{"d", "e"}) {
		t.Errorf("lastLines wrong: %v", got)
	}
	if got := lastLines(in, 10); !reflect.DeepEqual(got, in) {
		t.Errorf("lastLines should return all when n > len: %v", got)
	}
	if lastLines(in, 0) != nil {
		t.Error("lastLines(_,0) should be nil")
	}

	if got := window(in, 0, 10); !reflect.DeepEqual(got, in) {
		t.Errorf("window should return all when it fits: %v", got)
	}
	got := window(in, 4, 2) // focus on last line
	if len(got) != 2 || got[1] != "e" {
		t.Errorf("window should keep focus visible: %v", got)
	}
	got = window(in, 0, 2)
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("window at start wrong: %v", got)
	}
}

func TestComboLabelSorted(t *testing.T) {
	c := map[string]interface{}{"os": "ubuntu-latest", "go": 1.25}
	if got := comboLabel(c); got != "go:1.25 os:ubuntu-latest" {
		t.Errorf("comboLabel = %q", got)
	}
}

func TestMatrixFilter(t *testing.T) {
	if matrixFilter(nil) != nil {
		t.Error("empty combo should yield nil filter")
	}
	f := matrixFilter(map[string]interface{}{"os": "macos", "v": 2})
	if !f["os"]["macos"] || !f["v"]["2"] {
		t.Errorf("filter wrong: %v", f)
	}
}

func TestBuildRowsAndCursor(t *testing.T) {
	jobs := []enginerun.JobMeta{
		{ID: "build", Steps: []enginerun.StepMeta{{ID: "0", Name: "checkout", Number: 1}, {ID: "1", Name: "make", Number: 2}}},
		{ID: "lint", Steps: []enginerun.StepMeta{{ID: "0", Name: "vet", Number: 1}}},
	}
	rows := buildRows(jobs)
	if len(rows) != 5 { // 2 headers + 3 steps
		t.Fatalf("want 5 rows, got %d: %v", len(rows), rows)
	}
	if rows[0].step != "" || rows[0].job != "build" {
		t.Errorf("row 0 should be the build header: %+v", rows[0])
	}
	if first := firstStepRow(rows); first != 1 {
		t.Errorf("firstStepRow = %d, want 1", first)
	}
	// cursor skips the lint header between rows 2 and 4
	if got := moveCursor(rows, 2, +1); got != 4 {
		t.Errorf("moveCursor down over header: got %d, want 4", got)
	}
	if got := moveCursor(rows, 4, -1); got != 2 {
		t.Errorf("moveCursor up over header: got %d, want 2", got)
	}
	if got := moveCursor(rows, 1, -1); got != 1 {
		t.Errorf("moveCursor at top should stay: got %d", got)
	}
	if got := moveCursor(rows, 4, +1); got != 4 {
		t.Errorf("moveCursor at bottom should stay: got %d", got)
	}
}

func TestPickWorkflow(t *testing.T) {
	dir := t.TempDir()
	if _, err := PickWorkflow(dir); err == nil {
		t.Error("empty dir should be an error")
	}
	for _, f := range []string{"b.yml", "a.yml", "z.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("on: push\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := PickWorkflow(dir)
	if err != nil || filepath.Base(got) != "a.yml" {
		t.Errorf("PickWorkflow(dir) = %q, %v — want a.yml", got, err)
	}
	one := filepath.Join(dir, "b.yml")
	if got, err := PickWorkflow(one); err != nil || got != one {
		t.Errorf("file should pass through: %q, %v", got, err)
	}
}
