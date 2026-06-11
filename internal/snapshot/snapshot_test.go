package snapshot

import (
	"strings"
	"testing"
)

func rf() *RunFile {
	return &RunFile{
		RunID: "t", Workflow: "wf.yml", Event: "push",
		Records: []Record{
			{Job: "test", Number: 1, Name: "Hello", Result: "success", Image: "i1",
				EnvDelta: map[string]string{"A": "1"}},
			{Job: "test", Number: 2, Name: "Build", Result: "failure", Image: "i2",
				Files: [3]int{3, 1, 0}, FileSample: []string{"A /ws/out.bin"},
				EnvDelta: map[string]string{"A": "2", "B": "x"}},
		},
	}
}

func TestEnvUpTo(t *testing.T) {
	f := rf()
	e1 := f.EnvUpTo("test", 1)
	if e1["A"] != "1" || len(e1) != 1 {
		t.Errorf("up to 1: %v", e1)
	}
	e2 := f.EnvUpTo("test", 2)
	if e2["A"] != "2" || e2["B"] != "x" {
		t.Errorf("up to 2: %v", e2)
	}
}

func TestRecordLookupAndHave(t *testing.T) {
	f := rf()
	if r := f.record("test", 2); r == nil || r.Name != "Build" {
		t.Fatalf("record 2: %+v", r)
	}
	if f.record("test", 9) != nil {
		t.Fatal("ghost record")
	}
	if !strings.Contains(f.have("test"), "2:Build(failure)") {
		t.Errorf("have: %s", f.have("test"))
	}
}

func TestRenderDiff(t *testing.T) {
	out := rf().RenderDiff(2)
	for _, w := range []string{`step 2 "Build"`, "+3 ~1 −0", "A /ws/out.bin", "env A=2", "env B=x"} {
		if !strings.Contains(out, w) {
			t.Errorf("diff missing %q:\n%s", w, out)
		}
	}
	if !strings.Contains(rf().RenderDiff(0), "step 1") {
		t.Error("diff 0 should render all")
	}
}

func TestParseEnvFileHeredoc(t *testing.T) {
	got := ParseEnvFile("X=1\nM<<EOF\na\nb\nEOF\n")
	if got["X"] != "1" || got["M"] != "a\nb" {
		t.Errorf("%v", got)
	}
}

func TestBackNumber(t *testing.T) {
	if n, err := BackNumber([]string{"3"}); err != nil || n != 3 {
		t.Errorf("n=%d err=%v", n, err)
	}
	if _, err := BackNumber(nil); err == nil {
		t.Error("want usage error")
	}
}

func TestQuote(t *testing.T) {
	if quote("it's") != `'it'\''s'` {
		t.Errorf("quote: %s", quote("it's"))
	}
}
