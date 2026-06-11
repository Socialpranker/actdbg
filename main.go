// actdbg — a debugger for GitHub Actions, locally.
//
// Not an emulator that pretends to be GitHub: a debugger that stops at the
// failed step, drops you into a shell with the step's environment, and tells
// you honestly where the local run differs from the real runner.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Socialpranker/actdbg/internal/doctor"
	"github.com/Socialpranker/actdbg/internal/enginerun"
	"github.com/Socialpranker/actdbg/internal/fidelity"
	"github.com/Socialpranker/actdbg/internal/shellenv"
	"github.com/Socialpranker/actdbg/internal/snapshot"
	"github.com/Socialpranker/actdbg/internal/state"
)

const version = "0.2.0"

const usage = `actdbg %s — a debugger for GitHub Actions, locally

USAGE
  actdbg run    [flags]    run a workflow; stop at the failed step; offer a shell
  actdbg shell             re-enter the last failed step's container
  actdbg back N [--cmd c]  time-travel: container state as of right after step N
  actdbg rerun --from N    fix the workflow, then re-run from step N (run-steps)
  actdbg diff [N]          step x-ray: files touched + $GITHUB_ENV delta per step
  actdbg check  [flags]    fidelity report: where a local run differs from GitHub
  actdbg doctor            diagnose Docker / images / common act pitfalls
  actdbg clean             remove act-* containers and networks left behind
  actdbg version

RUN FLAGS
  -W path            workflow file or directory (default .github/workflows)
  -j job             run a single job
  -e event           event name (default push)
  --event-file f     JSON payload for the event
  --secrets-file f   KEY=VALUE lines (like act's .secrets); values never printed
  -s KEY=VALUE       set a secret (repeatable)
  -P plat=image      map runs-on platform to image (repeatable)
  --arch a           container architecture, e.g. linux/amd64
  --bind             bind the working directory instead of copying it
  --no-shell         do not offer a shell on failure (CI/scripted use)
  --verbose          stream full act logs instead of the condensed timeline

A failed step leaves its container alive. 'actdbg shell' re-enters it with the
step's environment reconstructed (workflow env chain + $GITHUB_ENV deltas).
Reality check: a green local run does not guarantee green on GitHub — run
'actdbg check' to see what differs for YOUR workflow.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun(os.Args[2:])
	case "shell":
		err = cmdShell()
	case "check":
		err = cmdCheck(os.Args[2:])
	case "back":
		err = cmdBack(os.Args[2:])
	case "rerun":
		err = cmdRerun(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "doctor":
		err = doctor.Run(os.Stdout)
	case "clean":
		err = enginerun.Clean(os.Stdout)
	case "version", "--version", "-v":
		fmt.Println("actdbg", version)
	case "help", "--help", "-h":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, "actdbg: unknown command %q\n\n", os.Args[1])
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "actdbg:", err)
		os.Exit(1)
	}
}

type repeated []string

func (r *repeated) String() string     { return fmt.Sprint(*r) }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	opts := enginerun.Options{}
	var secretKVs, platforms repeated
	fs.StringVar(&opts.WorkflowPath, "W", ".github/workflows", "workflow file or dir")
	fs.StringVar(&opts.Job, "j", "", "job id")
	fs.StringVar(&opts.Event, "e", "push", "event name")
	fs.StringVar(&opts.EventFile, "event-file", "", "event payload JSON")
	fs.StringVar(&opts.SecretsFile, "secrets-file", "", "secrets file (KEY=VALUE lines)")
	fs.Var(&secretKVs, "s", "secret KEY=VALUE")
	fs.Var(&platforms, "P", "platform=image")
	fs.StringVar(&opts.Arch, "arch", "", "container architecture (e.g. linux/amd64)")
	fs.BoolVar(&opts.Bind, "bind", false, "bind workdir instead of copy")
	fs.BoolVar(&opts.NoShell, "no-shell", false, "do not offer a shell on failure")
	fs.BoolVar(&opts.Verbose, "verbose", false, "full act logs")
	fs.BoolVar(&opts.NoSnapshot, "no-snapshot", false, "disable per-step snapshots (time-travel)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	opts.Secrets, err = shellenv.CollectSecrets(opts.SecretsFile, secretKVs)
	if err != nil {
		return err
	}
	opts.Platforms = enginerun.ParsePlatforms(platforms)

	// Honest pre-flight: a short fidelity summary before running.
	if findings, ferr := fidelity.CheckPath(opts.WorkflowPath, opts.Job); ferr == nil && len(findings) > 0 {
		fmt.Println(fidelity.Summary(findings))
	}
	return enginerun.Run(opts)
}

func cmdShell() error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("no stopped step found (%v) — run 'actdbg run' first", err)
	}
	return shellenv.Enter(st)
}

func cmdBack(args []string) error {
	fs := flag.NewFlagSet("back", flag.ExitOnError)
	cmd := fs.String("cmd", "", "run one command instead of an interactive shell")
	var rest []string
	for _, a := range args { // allow `back 2 --cmd ...` order
		rest = append(rest, a)
	}
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		if err := fs.Parse(rest[1:]); err != nil {
			return err
		}
	} else {
		if err := fs.Parse(rest); err != nil {
			return err
		}
	}
	n, err := snapshot.BackNumber(args)
	if err != nil {
		return err
	}
	rf, err := snapshot.LoadLatestRun()
	if err != nil {
		return err
	}
	c, err := rf.Restore("", n)
	if err != nil {
		return err
	}
	fmt.Printf("⏪ state after step %d restored into %s\n", n, c)
	banner := fmt.Sprintf("actdbg: time-travel — container state as it was right after step %d. exit to leave; cleanup: docker rm -f %s", n, c)
	return snapshot.EnterRestored(c, rf.EnvUpTo("", n), banner, *cmd)
}

func cmdRerun(args []string) error {
	fs := flag.NewFlagSet("rerun", flag.ExitOnError)
	from := fs.Int("from", 0, "step number to re-run from (snapshot of the previous step is restored)")
	job := fs.String("j", "", "job id (defaults to the only/last job)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == 0 {
		return fmt.Errorf("usage: actdbg rerun --from N (e.g. the step that failed — fix the workflow first)")
	}
	return snapshot.Rerun(*from, *job)
}

func cmdDiff(args []string) error {
	n := 0
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &n)
	}
	rf, err := snapshot.LoadLatestRun()
	if err != nil {
		return err
	}
	fmt.Print(rf.RenderDiff(n))
	return nil
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	wf := fs.String("W", ".github/workflows", "workflow file or dir")
	job := fs.String("j", "", "job id")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	findings, err := fidelity.CheckPath(*wf, *job)
	if err != nil {
		return err
	}
	if *asJSON {
		return fidelity.WriteJSON(os.Stdout, findings)
	}
	fmt.Print(fidelity.Render(findings))
	return nil
}
