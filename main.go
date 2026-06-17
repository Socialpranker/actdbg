// actdbg — a debugger for GitHub Actions, locally.
//
// Not an emulator that pretends to be GitHub: a debugger that stops at the
// failed step, drops you into a shell with the step's environment, and tells
// you honestly where the local run differs from the real runner.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Socialpranker/actdbg/internal/cmdlog"
	"github.com/Socialpranker/actdbg/internal/doctor"
	"github.com/Socialpranker/actdbg/internal/enginerun"
	"github.com/Socialpranker/actdbg/internal/fidelity"
	"github.com/Socialpranker/actdbg/internal/replay"
	"github.com/Socialpranker/actdbg/internal/shellenv"
	"github.com/Socialpranker/actdbg/internal/snapshot"
	"github.com/Socialpranker/actdbg/internal/state"
	"github.com/Socialpranker/actdbg/internal/ui"
)

// version is overridden at release time via -ldflags "-X main.version=…"
// (GoReleaser). The default is the in-development version.
var version = "0.4.0"

const usage = `actdbg %s — a debugger for GitHub Actions, locally

USAGE
  actdbg                   in a terminal: open the TUI (same as actdbg ui)
  actdbg ui     [flags]    lazygit-style TUI: steps left, live log right, command log below
  actdbg run    [flags]    run a workflow; stop at the failed step; offer a shell
  actdbg shell             re-enter the last failed step's container
  actdbg replay <run-url>  reproduce a real failed GitHub run locally, then debug it
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
  --matrix key=val   run only matching strategy.matrix combination(s) (repeatable)
  --arch a           container architecture, e.g. linux/amd64
  --bind             bind the working directory instead of copying it
  --no-shell         do not offer a shell on failure (CI/scripted use)
  --show-commands    afterwards, print the docker commands actdbg executed
  --verbose          stream full act logs instead of the condensed timeline

A failed step leaves its container alive. 'actdbg shell' re-enters it with the
step's environment reconstructed (workflow env chain + $GITHUB_ENV deltas).
Every docker command actdbg runs is logged to ~/.actdbg/commands.log.
Reality check: a green local run does not guarantee green on GitHub — run
'actdbg check' to see what differs for YOUR workflow.
`

func main() {
	// One Ctrl+C cancels the run gracefully (act unwinds, containers kept for
	// inspection); a second Ctrl+C exits hard.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		if isTTY(os.Stdin) && isTTY(os.Stdout) {
			if err := cmdUI(nil); err != nil {
				fmt.Fprintln(os.Stderr, "actdbg:", err)
				os.Exit(1)
			}
			return
		}
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun(ctx, os.Args[2:])
	case "ui":
		err = cmdUI(os.Args[2:])
	case "shell":
		err = cmdShell()
	case "check":
		err = cmdCheck(os.Args[2:])
	case "replay":
		err = cmdReplay(ctx, os.Args[2:])
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

func cmdRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	opts := enginerun.Options{}
	var secretKVs, platforms, matrixKVs repeated
	var showCmds bool
	fs.StringVar(&opts.WorkflowPath, "W", ".github/workflows", "workflow file or dir")
	fs.StringVar(&opts.Job, "j", "", "job id")
	fs.StringVar(&opts.Event, "e", "push", "event name")
	fs.StringVar(&opts.EventFile, "event-file", "", "event payload JSON")
	fs.StringVar(&opts.SecretsFile, "secrets-file", "", "secrets file (KEY=VALUE lines)")
	fs.Var(&secretKVs, "s", "secret KEY=VALUE")
	fs.Var(&platforms, "P", "platform=image")
	fs.Var(&matrixKVs, "matrix", "matrix key=value filter (repeatable)")
	fs.StringVar(&opts.Arch, "arch", "", "container architecture (e.g. linux/amd64)")
	fs.BoolVar(&opts.Bind, "bind", false, "bind workdir instead of copy")
	fs.BoolVar(&opts.NoShell, "no-shell", false, "do not offer a shell on failure")
	fs.BoolVar(&showCmds, "show-commands", false, "afterwards, print the docker commands actdbg executed")
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
	opts.Matrix, err = enginerun.ParseMatrix(matrixKVs)
	if err != nil {
		return err
	}

	// Honest pre-flight: a short fidelity summary before running.
	if findings, ferr := fidelity.CheckPath(opts.WorkflowPath, opts.Job); ferr == nil && len(findings) > 0 {
		fmt.Println(fidelity.Summary(findings))
	}
	runErr := enginerun.Run(ctx, opts)
	if showCmds {
		printCommandLog()
	}
	return runErr
}

func printCommandLog() {
	cmds := cmdlog.Tail(20)
	if len(cmds) == 0 {
		fmt.Println("\ncommands actdbg ran: none recorded")
		return
	}
	fmt.Println("\ncommands actdbg ran (newest last):")
	for _, c := range cmds {
		fmt.Println("  $", c)
	}
	fmt.Println("  full log:", cmdlog.Path())
}

func cmdUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	wf := fs.String("W", ".github/workflows", "workflow file or dir (dir → its first workflow)")
	job := fs.String("j", "", "job id")
	event := fs.String("e", "push", "event name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := ui.PickWorkflow(*wf)
	if err != nil {
		return err
	}
	st, err := ui.Start(enginerun.Options{
		WorkflowPath: path,
		Job:          *job,
		Event:        *event,
		Platforms:    enginerun.ParsePlatforms(nil),
		Secrets:      map[string]string{},
	})
	if err != nil {
		return err
	}
	if st != nil {
		return shellenv.Enter(st)
	}
	return nil
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func cmdShell() error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("no stopped step found (%v) — run 'actdbg run' first", err)
	}
	return shellenv.Enter(st)
}

func cmdReplay(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	here := fs.Bool("here", false, "run on the current checkout even if it differs from the run's commit")
	noShell := fs.Bool("no-shell", false, "do not offer a shell on failure")
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: actdbg replay <github-run-url> — run it inside a clone of that repo")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	owner, repo, id, err := replay.ParseURL(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("⤓ fetching run %s (%s/%s)…\n", id, owner, repo)
	ri, err := replay.Fetch(owner, repo, id)
	if err != nil {
		return err
	}
	fmt.Printf("  failed: job %q, step %d %q · event %s · commit %.10s\n",
		ri.FailedJob, ri.FailedNumber, ri.FailedStep, ri.Event, ri.HeadSHA)
	if ok, local := replay.CheckHead(ri.HeadSHA); !ok {
		if local == "" {
			return fmt.Errorf("not inside a git repo — clone %s/%s first", owner, repo)
		}
		if !*here {
			return fmt.Errorf("your checkout (%.10s) differs from the run's commit %.10s.\nEither:  git fetch && git checkout %.10s   (exact replay)\nOr:      actdbg replay %s --here              (replay on current tree)",
				local, ri.HeadSHA, ri.HeadSHA, args[0])
		}
		fmt.Printf("  ⚠ replaying on current tree (%.10s), not the run's commit\n", local)
	}
	jobID, err := replay.JobIDByName(ri.Path, ri.FailedJob)
	if err != nil {
		return err
	}
	fmt.Printf("  replaying job %q locally — the debugger takes over at the failure.\n  honesty: local ≠ GitHub (images, secrets, OIDC) — `actdbg check` explains.\n\n", jobID)
	return enginerun.Run(ctx, enginerun.Options{
		WorkflowPath: ri.Path, Job: jobID, Event: ri.Event,
		Platforms: enginerun.ParsePlatforms(nil), NoShell: *noShell,
		Secrets: map[string]string{},
	})
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
