package timeline

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func entry(data logrus.Fields, msg string) *logrus.Entry {
	e := logrus.NewEntry(logrus.New())
	e.Data = data
	e.Message = msg
	return e
}

func TestTrackerDetectsFailureAndTail(t *testing.T) {
	var buf bytes.Buffer
	tr := New(&buf, false)
	tr.NameFor = func(job, stage, id string) string {
		if id == "1" {
			return "Build"
		}
		return "Checkout"
	}

	fire := func(d logrus.Fields, m string) { _ = tr.Fire(entry(d, m)) }

	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"0"}}, "⭐ Run Main Checkout")
	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"0"}, "stepResult": "success"}, "ok")
	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"1"}}, "⭐ Run Main Build")
	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"1"}, "raw_output": true}, "compiling...")
	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"1"}, "raw_output": true}, "error: pnpm not found")
	fire(logrus.Fields{"jobID": "test", "stage": "Main", "stepID": []string{"1"}, "stepResult": "failure"}, "fail")
	fire(logrus.Fields{"jobID": "test", "jobResult": "failure"}, "job failed")

	f := tr.FirstFailure()
	if f == nil {
		t.Fatal("failure not detected")
	}
	if f.StepID != "1" || f.JobID != "test" || f.Stage != "Main" {
		t.Errorf("wrong failed step: %+v", f)
	}
	tail := tr.Tail(f)
	if len(tail) != 2 || !strings.Contains(tail[1], "pnpm not found") {
		t.Errorf("tail wrong: %v", tail)
	}
	if tr.JobResult("test") != "failure" {
		t.Error("job result not recorded")
	}
	out := buf.String()
	for _, want := range []string{"▶ [test] Checkout", "✅ [test] Checkout", "❌ [test] Build"} {
		if !strings.Contains(out, want) {
			t.Errorf("timeline output missing %q:\n%s", want, out)
		}
	}
}

func TestTrackerRingBuffer(t *testing.T) {
	tr := New(&bytes.Buffer{}, false)
	for i := 0; i < 100; i++ {
		_ = tr.Fire(entry(logrus.Fields{"jobID": "j", "stage": "Main", "stepID": []string{"0"}, "raw_output": true}, "line"))
	}
	_ = tr.Fire(entry(logrus.Fields{"jobID": "j", "stage": "Main", "stepID": []string{"0"}, "stepResult": "failure"}, ""))
	if got := len(tr.Tail(tr.FirstFailure())); got != 40 {
		t.Errorf("ring buffer should cap at 40, got %d", got)
	}
}

func TestOnLineCallback(t *testing.T) {
	tr := New(&bytes.Buffer{}, false)
	var got []string
	tr.OnLine = func(job, step, line string) { got = append(got, job+"/"+step+": "+line) }
	_ = tr.Fire(entry(logrus.Fields{"jobID": "j", "stage": "Main", "stepID": []string{"0"}, "raw_output": true}, "hello\n"))
	_ = tr.Fire(entry(logrus.Fields{"jobID": "j", "stage": "Main", "stepID": []string{"0"}, "raw_output": true}, "world"))
	if len(got) != 2 || got[0] != "j/0: hello" || got[1] != "j/0: world" {
		t.Errorf("OnLine wrong: %v", got)
	}
}

func TestNoFailure(t *testing.T) {
	tr := New(&bytes.Buffer{}, false)
	_ = tr.Fire(entry(logrus.Fields{"jobID": "j", "stage": "Main", "stepID": []string{"0"}, "stepResult": "success"}, ""))
	if tr.FirstFailure() != nil {
		t.Error("no failure expected")
	}
}
