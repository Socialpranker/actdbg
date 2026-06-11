// Package enginerun drives act's runner: plan → execute → stop at failure →
// hand off to the shell.
package enginerun

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nektos/act/pkg/model"
	"github.com/nektos/act/pkg/runner"
	"gopkg.in/yaml.v3"

	"github.com/Socialpranker/actdbg/internal/shellenv"
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

type stepInfo struct {
	ID     string
	Name   string
	Number int // 1-based
	Env    map[string]string
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
				id := s.ID
				if id == "" {
					id = strconv.Itoa(i)
				}
				name := s.Name
				if name == "" {
					if s.Uses != "" {
						name = s.Uses
					} else {
						name = firstLine(s.Run)
					}
				}
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

// Run executes the workflow and stops at the first failure.
func Run(o Options) error {
	wfPath := o.WorkflowPath
	planner, err := model.NewWorkflowPlanner(wfPath, false, false)
	if err != nil {
		return fmt.Errorf("reading workflows at %s: %w", wfPath, err)
	}
	var plan *model.Plan
	if o.Job != "" {
		plan, err = planner.PlanJob(o.Job)
	} else {
		plan, err = planner.PlanEvent(o.Event)
	}
	if err != nil {
		return err
	}
	if len(plan.Stages) == 0 {
		return fmt.Errorf("nothing to run for event %q (try -W <file> or -e <event>)", o.Event)
	}

	stepsByJob, totals, jobEnv := stepIndex(plan)

	tr := timeline.New(os.Stdout, o.Verbose)
	tr.NameFor = func(jobID, _, stepID string) string {
		if si, ok := stepsByJob[jobID][stepID]; ok {
			return si.Name
		}
		return ""
	}

	workdir, _ := filepath.Abs(".")
	cfg := &runner.Config{
		Workdir:         workdir,
		BindWorkdir:     o.Bind,
		EventName:       o.Event,
		EventPath:       o.EventFile,
		DefaultBranch:   "main",
		ReuseContainers: true, // keep the corpse: that's the whole point
		LogOutput:       true,
		Env:             map[string]string{},
		Secrets:         o.Secrets,
		Platforms:       o.Platforms,
		ContainerArchitecture: o.Arch,
		AutoRemove:            false,
		Token:                 o.Secrets["GITHUB_TOKEN"],
	}
	r, err := runner.New(cfg)
	if err != nil {
		return err
	}

	before := listActContainers()
	ctx := runner.WithJobLoggerFactory(context.Background(), tr)
	execErr := r.NewPlanExecutor(plan)(ctx)

	fail := tr.FirstFailure()
	if fail == nil {
		if execErr != nil {
			return fmt.Errorf("run error (no failed step recorded): %w", execErr)
		}
		fmt.Println("\n✔ all steps passed locally.")
		fmt.Println("  reality check: green here ≠ green on GitHub — `actdbg check` shows what differs.")
		return nil
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

	workdirIn := containerWorkdir(container)
	st := &state.State{
		Container:  container,
		JobID:      fail.JobID,
		StepName:   si.Name,
		StepNumber: si.Number,
		TotalSteps: totals[fail.JobID],
		Workdir:    workdirIn,
		Env:        env,
		Workflow:   wfPath,
	}
	if err := state.Save(st); err != nil {
		fmt.Fprintln(os.Stderr, "actdbg: could not save state:", err)
	}

	fmt.Printf("\n⏸  stopped: step %d/%d %q failed (job %s)\n",
		si.Number, totals[fail.JobID], si.Name, fail.JobID)
	for _, l := range lastN(tr.Tail(fail), 12) {
		fmt.Println("   │", l)
	}
	if container == "" {
		fmt.Println("   (could not locate the job container — it may have exited; `docker ps -a` to inspect)")
		return nil
	}
	fmt.Printf("   container kept alive: %s\n", container)

	if o.NoShell || !isTerminal() {
		fmt.Println("\n→ enter it later with: actdbg shell")
		return nil
	}
	fmt.Print("\ndrop into a shell at the failed step? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	ans, _ := reader.ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans == "" || ans == "y" || ans == "yes" {
		return shellenv.Enter(st)
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
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name=act-", "--format", "{{.Names}}").Output()
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
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name=act-",
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
	out, err := exec.Command("docker", "inspect", "-f", "{{.Config.WorkingDir}}", container).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Clean removes act-* containers and networks.
func Clean(w *os.File) error {
	out, _ := exec.Command("docker", "ps", "-a", "--filter", "name=act-", "--format", "{{.Names}}").Output()
	names := strings.Fields(string(out))
	sort.Strings(names)
	for _, n := range names {
		if err := exec.Command("docker", "rm", "-f", n).Run(); err == nil {
			fmt.Fprintln(w, "removed container", n)
		}
	}
	nets, _ := exec.Command("docker", "network", "ls", "--filter", "name=act-", "--format", "{{.Name}}").Output()
	for _, n := range strings.Fields(string(nets)) {
		if err := exec.Command("docker", "network", "rm", n).Run(); err == nil {
			fmt.Fprintln(w, "removed network", n)
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(w, "nothing to clean")
	}
	return nil
}
