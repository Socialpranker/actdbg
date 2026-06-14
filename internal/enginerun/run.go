// Package enginerun drives act's runner: plan → execute → stop at failure →
// hand off to the shell.
package enginerun

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nektos/act/pkg/model"
	"github.com/nektos/act/pkg/runner"
	"gopkg.in/yaml.v3"

	"github.com/Socialpranker/actdbg/internal/cmdlog"
	"github.com/Socialpranker/actdbg/internal/shellenv"
	"github.com/Socialpranker/actdbg/internal/snapshot"
	"github.com/Socialpranker/actdbg/internal/state"
	"github.com/Socialpranker/actdbg/internal/timeline"
)

// Options are the run parameters collected by the CLI.
type Options struct {
	WorkflowPath string
	Job          string
	Event        string
	EventFile    string
	SecretsFile  string
	Secrets      map[string]string
	Platforms    map[string]string
	Arch         string
	Bind         bool
	NoShell      bool
	Verbose      bool
	NoSnapshot   bool
	// Matrix filters which strategy.matrix combinations run
	// (key → allowed values), act's own --matrix shape.
	Matrix map[string]map[string]bool
}

// DefaultPlatforms mirrors act's commonly used medium images.
func DefaultPlatforms() map[string]string {
	return map[string]string{
		"ubuntu-latest": "catthehacker/ubuntu:act-latest",
		"ubuntu-24.04":  "catthehacker/ubuntu:act-24.04",
		"ubuntu-22.04":  "catthehacker/ubuntu:act-22.04",
		"ubuntu-20.04":  "catthehacker/ubuntu:act-20.04",
	}
}

// ParsePlatforms turns repeated "platform=image" flags into a map.
func ParsePlatforms(kvs []string) map[string]string {
	m := DefaultPlatforms()
	for _, kv := range kvs {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return m
}

// ParseMatrix turns repeated "key=value" flags into act's matrix filter
// shape: only combinations whose key has one of the allowed values run.
func ParseMatrix(kvs []string) (map[string]map[string]bool, error) {
	if len(kvs) == 0 {
		return nil, nil
	}
	m := map[string]map[string]bool{}
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("--matrix %q: want key=value", kv)
		}
		if m[k] == nil {
			m[k] = map[string]bool{}
		}
		m[k][v] = true
	}
	return m, nil
}

type stepInfo struct {
	ID     string
	Name   string
	Number int // 1-based
	Env    map[string]string
}

// stepIdentity resolves the ID and display name of a planned step.
func stepIdentity(s *model.Step, i int) (id, name string) {
	id = s.ID
	if id == "" {
		id = strconv.Itoa(i)
	}
	name = s.Name
	if name == "" {
		if s.Uses != "" {
			name = s.Uses
		} else {
			name = firstLine(s.Run)
		}
	}
	return id, name
}

// stepIndex builds jobID -> stepID -> info from the planned workflows.
func stepIndex(plan *model.Plan) (map[string]map[string]stepInfo, map[string]int, map[string]map[string]string) {
	steps := map[string]map[string]stepInfo{}
	totals := map[string]int{}
	jobEnv := map[string]map[string]string{}
	for _, stage := range plan.Stages {
		for _, run := range stage.Runs {
			job := run.Workflow.GetJob(run.JobID)
			if job == nil {
				continue
			}
			env := map[string]string{}
			for k, v := range run.Workflow.Env {
				env[k] = v
			}
			mergeYamlEnv(env, job.Env)
			jobEnv[run.JobID] = env
			m := map[string]stepInfo{}
			for i, s := range job.Steps {
				id, name := stepIdentity(s, i)
				se := map[string]string{}
				mergeYamlEnv(se, s.Env)
				m[id] = stepInfo{ID: id, Name: name, Number: i + 1, Env: se}
			}
			steps[run.JobID] = m
			totals[run.JobID] = len(job.Steps)
		}
	}
	return steps, totals, jobEnv
}

func mergeYamlEnv(dst map[string]string, n yaml.Node) {
	var m map[string]string
	if err := n.Decode(&m); err == nil {
		for k, v := range m {
			dst[k] = v
		}
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i] + " …"
	}
	if len(s) > 60 {
		s = s[:57] + "…"
	}
	return s
}

// StepMeta describes one planned step for display purposes (TUI).
type StepMeta struct {
	ID     string
	Name   string
	Number int // 1-based
}

// JobMeta describes one planned job for display purposes (TUI).
type JobMeta struct {
	ID       string
	Steps    []StepMeta
	Matrixes []map[string]interface{} // nil when the job has no strategy.matrix
}

// Inspect plans a workflow and returns its jobs and steps in plan order,
// plus each job's matrix combinations. No containers are touched.
func Inspect(wfPath, job, event string) ([]JobMeta, error) {
	plan, err := buildPlan(wfPath, job, event)
	if err != nil {
		return nil, err
	}
	var out []JobMeta
	for _, stage := range plan.Stages {
		for _, run := range stage.Runs {
			j := run.Workflow.GetJob(run.JobID)
			if j == nil {
				continue
			}
			jm := JobMeta{ID: run.JobID}
			for i, s := range j.Steps {
				id, name := stepIdentity(s, i)
				jm.Steps = append(jm.Steps, StepMeta{ID: id, Name: name, Number: i + 1})
			}
			if ms, err := j.GetMatrixes(); err == nil &&
				(len(ms) > 1 || (len(ms) == 1 && len(ms[0]) > 0)) {
				jm.Matrixes = ms
			}
			out = append(out, jm)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nothing planned for event %q (try -W <file> or -e <event>)", event)
	}
	return out, nil
}

func buildPlan(wfPath, job, event string) (*model.Plan, error) {
	planner, err := model.NewWorkflowPlanner(wfPath, false, false)
	if err != nil {
		return nil, fmt.Errorf("reading workflows at %s: %w", wfPath, err)
	}
	var plan *model.Plan
	if job != "" {
		plan, err = planner.PlanJob(job)
	} else {
		plan, err = planner.PlanEvent(event)
	}
	if err != nil {
		return nil, err
	}
	if len(plan.Stages) == 0 {
		return nil, fmt.Errorf("nothing to run for event %q (try -W <file> or -e <event>)", event)
	}
	return plan, nil
}

// RunResult is what a finished run produced, for the caller to render.
type RunResult struct {
	Failure    *timeline.StepEvent // nil when every step passed
	State      *state.State        // saved stop state (nil when nothing failed)
	Container  string              // "" when the job container could not be located
	StepName   string
	StepNumber int
	TotalSteps int
	SaveErr    error // non-nil when persisting the stop state failed
}

// RunWithTracker plans and executes the workflow, reporting progress only
// through tr (its Out writer and callbacks) — it never prints on its own.
// The TUI uses it with tr.Out = io.Discard; Run wraps it for the CLI.
func RunWithTracker(o Options, tr *timeline.Tracker) (*RunResult, error) {
	wfPath := o.WorkflowPath
	plan, err := buildPlan(wfPath, o.Job, o.Event)
	if err != nil {
		return nil, err
	}

	// Heads-up before a multi-gigabyte image pull so the first run doesn't
	// look like a hang.
	warnIfPullNeeded(plan, o.Platforms)

	stepsByJob, totals, jobEnv := stepIndex(plan)

	tr.NameFor = func(jobID, _, stepID string) string {
		if si, ok := stepsByJob[jobID][stepID]; ok {
			return si.Name
		}
		return ""
	}
	mgr := snapshot.NewManager(wfPath, o.Event, !o.NoSnapshot)
	mgr.NameFor = func(jobID, stepID string) (string, int) {
		si := stepsByJob[jobID][stepID]
		return si.Name, si.Number
	}
	// Chain snapshot bookkeeping with any caller-installed callbacks (TUI).
	prevStart, prevResult := tr.OnStepStart, tr.OnStepResult
	tr.OnStepStart = func(jobID, stepID string) {
		mgr.StepStarted(jobID, stepID)
		if prevStart != nil {
			prevStart(jobID, stepID)
		}
	}
	tr.OnStepResult = func(jobID, stepID, result string) string {
		extra := mgr.OnStepResult(jobID, stepID, result)
		if prevResult != nil {
			if e := prevResult(jobID, stepID, result); extra == "" {
				extra = e
			}
		}
		return extra
	}

	workdir, _ := filepath.Abs(".")
	cfg := &runner.Config{
		Workdir:               workdir,
		BindWorkdir:           o.Bind,
		EventName:             o.Event,
		EventPath:             o.EventFile,
		DefaultBranch:         "main",
		ReuseContainers:       true, // keep the corpse: that's the whole point
		LogOutput:             true,
		Env:                   map[string]string{},
		Secrets:               o.Secrets,
		Platforms:             o.Platforms,
		ContainerArchitecture: o.Arch,
		AutoRemove:            false,
		Matrix:                o.Matrix,
		Token:                 o.Secrets["GITHUB_TOKEN"],
	}
	r, err := runner.New(cfg)
	if err != nil {
		return nil, err
	}

	before := listActContainers()
	ctx := runner.WithJobLoggerFactory(context.Background(), tr)
	execErr := r.NewPlanExecutor(plan)(ctx)

	fail := tr.FirstFailure()
	if fail == nil {
		if execErr != nil {
			return nil, fmt.Errorf("run error (no failed step recorded): %w", execErr)
		}
		return &RunResult{}, nil
	}

	si := stepsByJob[fail.JobID][fail.StepID]
	container := newestActContainer(before)
	env := map[string]string{}
	for k, v := range jobEnv[fail.JobID] {
		env[k] = v
	}
	for k, v := range si.Env {
		env[k] = v
	}
	env["CI"] = "true"
	env["GITHUB_ACTIONS"] = "true"
	env["GITHUB_JOB"] = fail.JobID
	env["GITHUB_EVENT_NAME"] = o.Event

	st := &state.State{
		Container:  container,
		JobID:      fail.JobID,
		StepName:   si.Name,
		StepNumber: si.Number,
		TotalSteps: totals[fail.JobID],
		Workdir:    containerWorkdir(container),
		Env:        env,
		Workflow:   wfPath,
	}
	return &RunResult{
		Failure:    fail,
		State:      st,
		Container:  container,
		StepName:   si.Name,
		StepNumber: si.Number,
		TotalSteps: totals[fail.JobID],
		SaveErr:    state.Save(st),
	}, nil
}

// Run executes the workflow and stops at the first failure (CLI mode:
// prints the timeline to stdout and offers a shell).
func Run(o Options) error {
	tr := timeline.New(os.Stdout, o.Verbose)
	res, err := RunWithTracker(o, tr)
	if err != nil {
		return err
	}
	if res.Failure == nil {
		fmt.Println("\n✔ all steps passed locally.")
		fmt.Println("  reality check: green here ≠ green on GitHub — `actdbg check` shows what differs.")
		return nil
	}
	if res.SaveErr != nil {
		fmt.Fprintln(os.Stderr, "actdbg: could not save state:", res.SaveErr)
	}

	fmt.Printf("\n⏸  stopped: step %d/%d %q failed (job %s)\n",
		res.StepNumber, res.TotalSteps, res.StepName, res.Failure.JobID)
	for _, l := range lastN(tr.Tail(res.Failure), 12) {
		fmt.Println("   │", l)
	}
	if res.Container == "" {
		fmt.Println("   (could not locate the job container — it may have exited; `docker ps -a` to inspect)")
		return nil
	}
	fmt.Printf("   container kept alive: %s\n", res.Container)

	if !o.NoSnapshot {
		fmt.Printf("   time-travel ready: actdbg back N · actdbg rerun --from %d · actdbg diff\n", res.StepNumber)
	}
	if o.NoShell || !isTerminal() {
		fmt.Println("\n→ enter it later with: actdbg shell")
		return nil
	}
	fmt.Print("\ndrop into a shell at the failed step? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	ans, _ := reader.ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans == "" || ans == "y" || ans == "yes" {
		return shellenv.Enter(res.State)
	}
	fmt.Println("→ later: actdbg shell")
	return nil
}

func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func listActContainers() map[string]bool {
	out, err := cmdlog.Docker("ps", "-a", "--filter", "name=act-", "--format", "{{.Names}}").Output()
	if err != nil {
		return map[string]bool{}
	}
	m := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		m[n] = true
	}
	return m
}

// newestActContainer prefers a running act-* container that did not exist
// before this run; falls back to the newest running, then newest any.
func newestActContainer(before map[string]bool) string {
	type row struct{ name, status string }
	out, err := cmdlog.Docker("ps", "-a", "--filter", "name=act-",
		"--format", "{{.Names}}\t{{.Status}}").Output()
	if err != nil {
		return ""
	}
	var fresh, running, all []row
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, status, _ := strings.Cut(line, "\t")
		if name == "" {
			continue
		}
		r := row{name, status}
		all = append(all, r) // docker ps lists newest first
		if strings.HasPrefix(status, "Up") {
			running = append(running, r)
			if !before[name] {
				fresh = append(fresh, r)
			}
		}
	}
	for _, set := range [][]row{fresh, running, all} {
		if len(set) > 0 {
			return set[0].name
		}
	}
	return ""
}

func containerWorkdir(container string) string {
	if container == "" {
		return ""
	}
	out, err := cmdlog.Docker("inspect", "-f", "{{.Config.WorkingDir}}", container).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Clean removes act-* containers and networks.
func Clean(w *os.File) error {
	out, _ := cmdlog.Docker("ps", "-a", "--filter", "name=act-", "--format", "{{.Names}}").Output()
	names := strings.Fields(string(out))
	sort.Strings(names)
	for _, n := range names {
		if err := cmdlog.Docker("rm", "-f", n).Run(); err == nil {
			fmt.Fprintln(w, "removed container", n)
		}
	}
	nets, _ := cmdlog.Docker("network", "ls", "--filter", "name=act-", "--format", "{{.Name}}").Output()
	for _, n := range strings.Fields(string(nets)) {
		if err := cmdlog.Docker("network", "rm", n).Run(); err == nil {
			fmt.Fprintln(w, "removed network", n)
		}
	}
	snapshot.CleanArtifacts(w)
	if len(names) == 0 {
		fmt.Fprintln(w, "nothing to clean")
	}
	return nil
}
