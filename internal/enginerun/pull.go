package enginerun

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/nektos/act/pkg/model"
)

// imagesForRun resolves the container images a run will use from its jobs'
// runs-on labels via the platform map. Labels absent from the map are dropped
// (act resolves those itself); an empty result falls back to the default
// medium image, so a fresh machine is still warned rather than silently
// pulling. Returns a deduplicated, sorted slice.
func imagesForRun(platforms map[string]string, runsOn []string) []string {
	set := map[string]bool{}
	for _, label := range runsOn {
		if img := platforms[strings.ToLower(strings.TrimSpace(label))]; img != "" {
			set[img] = true
		}
	}
	if len(set) == 0 {
		if def := platforms["ubuntu-latest"]; def != "" {
			set[def] = true
		}
	}
	return sortedKeys(set)
}

// missingImages returns the needed image refs not present locally, deduplicated
// and sorted. present is the set of local "repo:tag" refs (docker images).
func missingImages(needed []string, present map[string]bool) []string {
	set := map[string]bool{}
	for _, img := range needed {
		if !present[img] {
			set[img] = true
		}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pullNotice formats the one-time heads-up shown before a run pulls images it
// doesn't have yet. Empty when nothing is missing.
func pullNotice(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"⬇ first run on this machine: pulling %s (~1–2 GB, one-time — cached after).\n"+
			"  this is the medium image actdbg defaults to so your CI tools (pnpm, etc.) actually exist.\n",
		strings.Join(missing, ", "))
}

// localImages returns the set of "repo:tag" refs Docker has locally. A nil map
// (docker unavailable) makes warnIfPullNeeded skip silently — the run itself
// will surface a clearer Docker error.
func localImages() map[string]bool {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return nil
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set
}

// warnIfPullNeeded prints a one-time notice to stderr if the run's images are
// not present locally, so a multi-gigabyte pull doesn't look like a hang. Best
// effort: any failure to introspect is silent.
func warnIfPullNeeded(plan *model.Plan, platforms map[string]string) {
	present := localImages()
	if present == nil {
		return
	}
	labels := runsOnLabels(plan)
	missing := missingImages(imagesForRun(platforms, labels), present)
	if n := pullNotice(missing); n != "" {
		fmt.Fprint(os.Stderr, n)
	}
}

// runsOnLabels collects every job's runs-on labels from the plan.
func runsOnLabels(plan *model.Plan) []string {
	var labels []string
	for _, stage := range plan.Stages {
		for _, run := range stage.Runs {
			job := run.Workflow.GetJob(run.JobID)
			if job == nil {
				continue
			}
			labels = append(labels, job.RunsOn()...)
		}
	}
	return labels
}
