// Package snapshot implements time-travel: a docker commit after every step,
// per-step file/env deltas (step x-ray), and restore/re-run from any step.
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nektos/act/pkg/model"
	"gopkg.in/yaml.v3"
)

type Record struct {
	Job        string            `json:"job"`
	StepID     string            `json:"step_id"`
	Name       string            `json:"name"`
	Number     int               `json:"number"`
	Result     string            `json:"result"`
	Image      string            `json:"image"` // snapshot tag
	Files      [3]int            `json:"files"` // added, changed, deleted (delta vs prev step)
	FileSample []string          `json:"file_sample"`
	EnvDelta   map[string]string `json:"env_delta"` // new/changed $GITHUB_ENV entries this step
	Took       string            `json:"took"`
}

type RunFile struct {
	RunID    string   `json:"run_id"`
	Workflow string   `json:"workflow"`
	Event    string   `json:"event"`
	Mounts   []string `json:"mounts"` // ready-made -v args
	Records  []Record `json:"records"`
}

type Manager struct {
	RunID     string
	Workflow  string
	Event     string
	Enabled   bool
	Container string
	NameFor   func(jobID, stepID string) (name string, number int)

	mounts   []string
	prevDiff map[string]string // path -> kind
	prevEnv  map[string]string
	file     RunFile
	started  map[string]time.Time
}

func NewManager(workflow, event string, enabled bool) *Manager {
	return &Manager{
		RunID:    time.Now().Format("20060102-150405"),
		Workflow: workflow, Event: event, Enabled: enabled,
		prevDiff: map[string]string{}, prevEnv: map[string]string{},
		started: map[string]time.Time{},
	}
}

func runsDir() string {
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, ".actdbg", "runs")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func (m *Manager) Path() string { return filepath.Join(runsDir(), m.RunID+".json") }

func dockerOut(args ...string) string {
	out, _ := exec.Command("docker", args...).Output()
	return string(out)
}

// FindContainer locates the act job container once and captures its volume
// mounts so restored containers can reattach them (toolcache etc.).
func (m *Manager) FindContainer() string {
	if m.Container != "" {
		return m.Container
	}
	out := dockerOut("ps", "--filter", "name=act-", "--format", "{{.Names}}")
	names := strings.Fields(out)
	if len(names) == 0 {
		return ""
	}
	m.Container = names[0] // newest first
	var mounts []struct {
		Type, Name, Destination string
		RW                      bool
	}
	raw := dockerOut("inspect", "-f", "{{json .Mounts}}", m.Container)
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &mounts)
	for _, mt := range mounts {
		if mt.Type == "volume" && mt.Name != "" {
			m.mounts = append(m.mounts, "-v", mt.Name+":"+mt.Destination)
		}
	}
	return m.Container
}

func (m *Manager) StepStarted(jobID, stepID string) {
	m.started[jobID+"/"+stepID] = time.Now()
}

// OnStepResult commits a snapshot and computes the per-step x-ray delta.
// Returns a one-line summary for the timeline ("" when disabled).
func (m *Manager) OnStepResult(jobID, stepID, result string) string {
	if !m.Enabled {
		return ""
	}
	c := m.FindContainer()
	if c == "" {
		return ""
	}
	name, num := jobID, 0
	if m.NameFor != nil {
		name, num = m.NameFor(jobID, stepID)
	}
	img := fmt.Sprintf("actdbg/snap:%s-%s-%d", m.RunID, jobID, num)
	_ = exec.Command("docker", "commit", "-p", c, img).Run()

	// cumulative docker diff -> delta vs previous step
	cur := map[string]string{}
	for _, line := range strings.Split(dockerOut("diff", c), "\n") {
		if len(line) > 2 {
			cur[line[2:]] = line[:1]
		}
	}
	var added, changed, deleted int
	var sample []string
	keys := make([]string, 0, len(cur))
	for p := range cur {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		k := cur[p]
		if pk, ok := m.prevDiff[p]; ok && pk == k {
			continue // unchanged since previous step
		}
		switch k {
		case "A":
			added++
		case "C":
			changed++
		case "D":
			deleted++
		}
		if len(sample) < 20 && !strings.HasPrefix(p, "/tmp") && !strings.HasPrefix(p, "/var/run/act") {
			sample = append(sample, k+" "+p)
		}
	}
	m.prevDiff = cur

	// $GITHUB_ENV delta
	envNow := ParseEnvFile(dockerOut("exec", c, "cat", "/var/run/act/workflow/envs.txt"))
	delta := map[string]string{}
	for k, v := range envNow {
		if m.prevEnv[k] != v {
			delta[k] = v
		}
	}
	m.prevEnv = envNow

	took := ""
	if t0, ok := m.started[jobID+"/"+stepID]; ok {
		took = time.Since(t0).Round(time.Millisecond * 100).String()
	}
	m.file = RunFile{RunID: m.RunID, Workflow: m.Workflow, Event: m.Event, Mounts: m.mounts,
		Records: append(m.file.Records, Record{
			Job: jobID, StepID: stepID, Name: name, Number: num, Result: result,
			Image: img, Files: [3]int{added, changed, deleted}, FileSample: sample,
			EnvDelta: delta, Took: took,
		})}
	b, _ := json.MarshalIndent(m.file, "", " ")
	_ = os.WriteFile(m.Path(), b, 0o600)

	parts := []string{fmt.Sprintf("Δ %d file(s)", added+changed+deleted)}
	if len(delta) > 0 {
		parts = append(parts, fmt.Sprintf("+%d env", len(delta)))
	}
	parts = append(parts, "snap ✓")
	return "   ⎘ " + strings.Join(parts, " · ")
}

// ParseEnvFile parses the $GITHUB_ENV format (KEY=V and KEY<<EOF blocks).
func ParseEnvFile(content string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if k, delim, ok := strings.Cut(line, "<<"); ok && !strings.Contains(k, "=") {
			var val []string
			for i++; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == strings.TrimSpace(delim) {
					break
				}
				val = append(val, lines[i])
			}
			out[strings.TrimSpace(k)] = strings.Join(val, "\n")
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = v
		}
	}
	return out
}

// ---------- restore / rerun / diff ----------

func LoadLatestRun() (*RunFile, error) {
	entries, err := filepath.Glob(filepath.Join(runsDir(), "*.json"))
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("no snapshot runs found — `actdbg run` first (snapshots are on by default)")
	}
	sort.Strings(entries)
	b, err := os.ReadFile(entries[len(entries)-1])
	if err != nil {
		return nil, err
	}
	var rf RunFile
	return &rf, json.Unmarshal(b, &rf)
}

func (rf *RunFile) record(job string, num int) *Record {
	for i := range rf.Records {
		r := &rf.Records[i]
		if (job == "" || r.Job == job) && r.Number == num {
			return r
		}
	}
	return nil
}

// EnvUpTo merges $GITHUB_ENV deltas of all steps up to and including num.
func (rf *RunFile) EnvUpTo(job string, num int) map[string]string {
	env := map[string]string{}
	for _, r := range rf.Records {
		if (job == "" || r.Job == job) && r.Number <= num {
			for k, v := range r.EnvDelta {
				env[k] = v
			}
		}
	}
	return env
}

// Restore starts a fresh container from the snapshot taken after step num.
func (rf *RunFile) Restore(job string, num int) (container string, err error) {
	r := rf.record(job, num)
	if r == nil {
		return "", fmt.Errorf("no snapshot for step %d (have: %s)", num, rf.have(job))
	}
	name := fmt.Sprintf("actdbg-tt-%d", time.Now().UnixNano()%1e9)
	args := append([]string{"run", "-d", "--name", name, "--entrypoint", ""}, rf.Mounts...)
	args = append(args, r.Image, "tail", "-f", "/dev/null")
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("restore: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return name, nil
}

func (rf *RunFile) have(job string) string {
	var s []string
	for _, r := range rf.Records {
		if job == "" || r.Job == job {
			s = append(s, fmt.Sprintf("%d:%s(%s)", r.Number, r.Name, r.Result))
		}
	}
	return strings.Join(s, ", ")
}

// RenderDiff prints the x-ray for one step or all.
func (rf *RunFile) RenderDiff(num int) string {
	var b strings.Builder
	for _, r := range rf.Records {
		if num != 0 && r.Number != num {
			continue
		}
		fmt.Fprintf(&b, "step %d %q [%s] — %s · +%d ~%d −%d files",
			r.Number, r.Name, r.Job, r.Result, r.Files[0], r.Files[1], r.Files[2])
		if r.Took != "" {
			fmt.Fprintf(&b, " · %s", r.Took)
		}
		b.WriteString("\n")
		for _, f := range r.FileSample {
			fmt.Fprintf(&b, "    %s\n", f)
		}
		ek := make([]string, 0, len(r.EnvDelta))
		for k := range r.EnvDelta {
			ek = append(ek, k)
		}
		sort.Strings(ek)
		for _, k := range ek {
			fmt.Fprintf(&b, "    env %s=%s\n", k, r.EnvDelta[k])
		}
	}
	if b.Len() == 0 {
		return "no records for that step\n"
	}
	return b.String()
}

// ---------- rerun ----------

type planStep struct {
	Number int
	Name   string
	Run    string
	Uses   string
	Env    map[string]string
	Wd     string
}

func planRunSteps(wfPath, event, jobID string) (string, []planStep, map[string]string, error) {
	planner, err := model.NewWorkflowPlanner(wfPath, false, false)
	if err != nil {
		return "", nil, nil, err
	}
	var plan *model.Plan
	if jobID != "" {
		plan, err = planner.PlanJob(jobID)
	} else {
		plan, err = planner.PlanEvent(event)
	}
	if err != nil {
		return "", nil, nil, err
	}
	for _, st := range plan.Stages {
		for _, run := range st.Runs {
			if jobID != "" && run.JobID != jobID {
				continue
			}
			job := run.Workflow.GetJob(run.JobID)
			if job == nil {
				continue
			}
			jobEnv := map[string]string{}
			for k, v := range run.Workflow.Env {
				jobEnv[k] = v
			}
			mergeEnv(jobEnv, job.Env)
			var steps []planStep
			for i, s := range job.Steps {
				se := map[string]string{}
				mergeEnv(se, s.Env)
				name := s.Name
				if name == "" {
					name = s.Uses
				}
				steps = append(steps, planStep{Number: i + 1, Name: name, Run: s.Run,
					Uses: s.Uses, Env: se, Wd: s.WorkingDirectory})
			}
			return run.JobID, steps, jobEnv, nil
		}
	}
	return "", nil, nil, fmt.Errorf("job not found in plan")
}

func mergeEnv(dst map[string]string, n yaml.Node) {
	var m map[string]string
	if err := n.Decode(&m); err == nil {
		for k, v := range m {
			dst[k] = v
		}
	}
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Rerun restores the snapshot before step `from` and re-executes run-steps
// from there using the CURRENT workflow file (that's the point: you fixed it).
func Rerun(from int, job string) error {
	rf, err := LoadLatestRun()
	if err != nil {
		return err
	}
	if from < 2 {
		return fmt.Errorf("rerun needs --from ≥ 2 (no pre-step-1 snapshot exists; a full `actdbg run` is just as fast there)")
	}
	jobID, steps, jobEnv, err := planRunSteps(rf.Workflow, rf.Event, job)
	if err != nil {
		return err
	}
	c, err := rf.Restore(jobID, from-1)
	if err != nil {
		return err
	}
	fmt.Printf("⏪ restored state after step %d into %s\n", from-1, c)
	wd := strings.TrimSpace(dockerOut("inspect", "-f", "{{.Config.WorkingDir}}", c))

	dyn := rf.EnvUpTo(jobID, from-1)
	for _, st := range steps {
		if st.Number < from {
			continue
		}
		if st.Uses != "" {
			fmt.Printf("⏸ step %d %q is a `uses:` action — actdbg re-runs only `run:` steps (v0.2 honesty; full re-runs in v0.3)\n", st.Number, st.Name)
			fmt.Printf("→ inspect the restored container instead: docker exec -it %s sh\n", c)
			return nil
		}
		if strings.Contains(st.Run, "${{") {
			fmt.Printf("⚠ step %d contains ${{ }} expressions — executed literally (unevaluated)\n", st.Number)
		}
		var sb strings.Builder
		sb.WriteString("set -e\n")
		merged := map[string]string{"CI": "true", "GITHUB_ACTIONS": "true", "GITHUB_ENV": "/tmp/actdbg-envs.txt", "GITHUB_PATH": "/tmp/actdbg-paths.txt"}
		for k, v := range jobEnv {
			merged[k] = v
		}
		for k, v := range dyn {
			merged[k] = v
		}
		for k, v := range st.Env {
			merged[k] = v
		}
		keys := make([]string, 0, len(merged))
		for k := range merged {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "export %s=%s\n", k, quote(merged[k]))
		}
		sb.WriteString("touch $GITHUB_ENV $GITHUB_PATH\n")
		if st.Wd != "" {
			fmt.Fprintf(&sb, "cd %s\n", quote(st.Wd))
		}
		sb.WriteString(st.Run)

		fmt.Printf("▶ step %d/%d %s\n", st.Number, len(steps), st.Name)
		put := exec.Command("docker", "exec", "-i", c, "sh", "-c", "cat > /tmp/actdbg-step.sh")
		put.Stdin = strings.NewReader(sb.String())
		if err := put.Run(); err != nil {
			return fmt.Errorf("inject step script: %w", err)
		}
		runArgs := []string{"exec"}
		if wd != "" {
			runArgs = append(runArgs, "-w", wd)
		}
		runArgs = append(runArgs, c, "sh", "-c", "bash /tmp/actdbg-step.sh 2>&1 || sh /tmp/actdbg-step.sh 2>&1")
		cmd := exec.Command("docker", runArgs...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ step %d failed again — container %s kept for inspection (docker exec -it %s sh)\n", st.Number, c, c)
			return fmt.Errorf("step %d failed", st.Number)
		}
		// absorb $GITHUB_ENV writes from this step for the next ones
		for k, v := range ParseEnvFile(dockerOut("exec", c, "cat", "/tmp/actdbg-envs.txt")) {
			dyn[k] = v
		}
		fmt.Printf("✅ step %d done\n", st.Number)
	}
	fmt.Printf("✔ re-run from step %d finished — without re-running steps 1–%d. cleanup: docker rm -f %s\n", from, from-1, c)
	return nil
}

// CleanArtifacts removes snapshot images and tt-containers.
func CleanArtifacts(w *os.File) {
	for _, c := range strings.Fields(dockerOut("ps", "-aq", "--filter", "name=actdbg-tt-")) {
		if exec.Command("docker", "rm", "-f", c).Run() == nil {
			fmt.Fprintln(w, "removed container", c)
		}
	}
	for _, i := range strings.Fields(dockerOut("images", "actdbg/snap", "-q")) {
		if exec.Command("docker", "rmi", "-f", i).Run() == nil {
			fmt.Fprintln(w, "removed snapshot image", i)
		}
	}
}

// EnterRestored opens a shell (or runs one command) in a restored container
// with the accumulated $GITHUB_ENV state exported.
func EnterRestored(container string, env map[string]string, banner, cmd string) error {
	var sb strings.Builder
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "export %s=%s\n", k, quote(env[k]))
	}
	sb.WriteString("export GITHUB_ENV=/tmp/actdbg-envs.txt GITHUB_PATH=/tmp/actdbg-paths.txt\ntouch $GITHUB_ENV $GITHUB_PATH\n")
	if banner != "" {
		fmt.Fprintf(&sb, "echo %s\n", quote(banner))
	}
	put := exec.Command("docker", "exec", "-i", container, "sh", "-c", "cat > /tmp/actdbg-env.sh")
	put.Stdin = strings.NewReader(sb.String())
	if err := put.Run(); err != nil {
		return fmt.Errorf("inject env: %w", err)
	}
	wd := strings.TrimSpace(dockerOut("inspect", "-f", "{{.Config.WorkingDir}}", container))
	args := []string{"exec"}
	if cmd == "" {
		args = append(args, "-it")
	}
	if wd != "" {
		args = append(args, "-w", wd)
	}
	sh := ". /tmp/actdbg-env.sh; exec bash 2>/dev/null || exec sh"
	if cmd != "" {
		sh = ". /tmp/actdbg-env.sh >/dev/null; " + cmd
	}
	args = append(args, container, "sh", "-c", sh)
	c := exec.Command("docker", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// BackNumber parses the `back N` argument.
func BackNumber(args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("usage: actdbg back <stepNumber> [--cmd '...']")
	}
	return strconv.Atoi(args[0])
}
