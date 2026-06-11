// Package timeline turns act's logrus stream into a condensed step timeline
// and records which step failed (with its last output lines).
package timeline

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// StepEvent is one observed step with its outcome.
type StepEvent struct {
	JobID   string
	Stage   string // Pre / Main / Post
	StepID  string
	Result  string // "", "success", "failure", "skipped"
	Started bool
}

// Tracker is both a logrus.Hook and a JobLoggerFactory for act.
type Tracker struct {
	mu        sync.Mutex
	Out       io.Writer
	Verbose   bool
	order     []string // keys in first-seen order
	steps     map[string]*StepEvent
	tail      map[string][]string // last output lines per step key
	jobResult map[string]string
	curKey    string
	// NameFor resolves a display name for (jobID, stage, stepID); optional.
	NameFor func(jobID, stage, stepID string) string
	// OnStepStart/OnStepResult are optional callbacks (Main stage only).
	OnStepStart  func(jobID, stepID string)
	OnStepResult func(jobID, stepID, result string) string // returns extra line to print
}

func New(out io.Writer, verbose bool) *Tracker {
	return &Tracker{
		Out:       out,
		Verbose:   verbose,
		steps:     map[string]*StepEvent{},
		tail:      map[string][]string{},
		jobResult: map[string]string{},
	}
}

// WithJobLogger implements act's runner.JobLoggerFactory.
func (t *Tracker) WithJobLogger() *logrus.Logger {
	l := logrus.New()
	if t.Verbose {
		l.SetOutput(t.Out)
		l.SetLevel(logrus.DebugLevel)
	} else {
		l.SetOutput(io.Discard)
		l.SetLevel(logrus.InfoLevel)
	}
	l.AddHook(t)
	return l
}

func (t *Tracker) Levels() []logrus.Level { return logrus.AllLevels }

func key(jobID, stage, stepID string) string { return jobID + "\x00" + stage + "\x00" + stepID }

func stepIDOf(e *logrus.Entry) (string, bool) {
	v, ok := e.Data["stepID"]
	if !ok {
		return "", false
	}
	if ids, ok := v.([]string); ok && len(ids) > 0 {
		return ids[0], true
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

// Fire implements logrus.Hook.
func (t *Tracker) Fire(e *logrus.Entry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	jobID, _ := e.Data["jobID"].(string)
	if jr, ok := e.Data["jobResult"].(string); ok && jobID != "" {
		t.jobResult[jobID] = jr
		return nil
	}
	stepID, hasStep := stepIDOf(e)
	stage, _ := e.Data["stage"].(string)
	if !hasStep {
		return nil
	}
	k := key(jobID, stage, stepID)
	st, seen := t.steps[k]
	if !seen {
		st = &StepEvent{JobID: jobID, Stage: stage, StepID: stepID, Started: true}
		t.steps[k] = st
		t.order = append(t.order, k)
		t.curKey = k
		if !t.Verbose && stage == "Main" {
			fmt.Fprintf(t.Out, "▶ %s\n", t.displayName(st))
		}
		if t.OnStepStart != nil && stage == "Main" {
			t.OnStepStart(jobID, stepID)
		}
	}
	if raw, _ := e.Data["raw_output"].(bool); raw {
		lines := t.tail[k]
		lines = append(lines, strings.TrimRight(e.Message, "\n"))
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		t.tail[k] = lines
		return nil
	}
	if res, ok := e.Data["stepResult"]; ok {
		st.Result = fmt.Sprintf("%v", res)
		if !t.Verbose && stage == "Main" {
			mark := "✅"
			if st.Result == "failure" {
				mark = "❌"
			} else if st.Result == "skipped" {
				mark = "⏭"
			}
			fmt.Fprintf(t.Out, "%s %s\n", mark, t.displayName(st))
		}
		if t.OnStepResult != nil && stage == "Main" {
			if extra := t.OnStepResult(jobID, stepID, st.Result); extra != "" && !t.Verbose {
				fmt.Fprintln(t.Out, extra)
			}
		}
	}
	return nil
}

func (t *Tracker) displayName(st *StepEvent) string {
	if t.NameFor != nil {
		if n := t.NameFor(st.JobID, st.Stage, st.StepID); n != "" {
			return fmt.Sprintf("[%s] %s", st.JobID, n)
		}
	}
	return fmt.Sprintf("[%s] %s/%s", st.JobID, st.Stage, st.StepID)
}

// FirstFailure returns the first failed Main-stage step, or nil.
func (t *Tracker) FirstFailure() *StepEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range t.order {
		st := t.steps[k]
		if st.Result == "failure" && st.Stage == "Main" {
			return st
		}
	}
	// fall back to any failed step (Pre/Post)
	for _, k := range t.order {
		if st := t.steps[k]; st.Result == "failure" {
			return st
		}
	}
	return nil
}

// Tail returns the captured last output lines of a step.
func (t *Tracker) Tail(st *StepEvent) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tail[key(st.JobID, st.Stage, st.StepID)]
}

// JobResult returns the recorded result for a job ("" if none).
func (t *Tracker) JobResult(jobID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.jobResult[jobID]
}

// Steps returns observed steps in order (copy).
func (t *Tracker) Steps() []StepEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]StepEvent, 0, len(t.order))
	for _, k := range t.order {
		out = append(out, *t.steps[k])
	}
	return out
}
