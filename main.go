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

	"github.com/Socialpranker/actdbg/internal/doctor"
	"github.com/Socialpranker/actdbg/internal/enginerun"
	"github.com/Socialpranker/actdbg/internal/fidelity"
	"github.com/Socialpranker/actdbg/internal/shellenv"
	"github.com/Socialpranker/actdbg/internal/state"
)

const version = "0.1.0"

const usage = `actdbg %s — a debugger for GitHub Actions, locally

USAGE
  actdbg run    [flags]    run a workflow; stop at the failed step; offer a shell
  actdbg shell             re-enter the last failed step's container
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
