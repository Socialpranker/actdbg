// Package fidelity statically inspects a workflow and reports where a local
// act-based run is known to differ from the real GitHub runner.
//
// Every rule is sourced from act's own documentation (not_supported) or from
// high-traffic act issues; this is the "honest debugger" half of actdbg.
package fidelity

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Level string

const (
	ERR  Level = "ERR"  // will not work locally
	WARN Level = "WARN" // works, but behaves differently
	INFO Level = "INFO" // worth knowing
)

type Finding struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Level  Level  `json:"level"`
	What   string `json:"what"`
	Advice string `json:"advice"`
}

// CheckPath checks a file or every workflow in a directory.
func CheckPath(path, job string) ([]Finding, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if st.IsDir() {
		for _, ext := range []string{"*.yml", "*.yaml"} {
			m, _ := filepath.Glob(filepath.Join(path, ext))
			files = append(files, m...)
		}
	} else {
		files = []string{path}
	}
	var all []Finding
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fs, err := Check(filepath.Base(f), b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, fs...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	_ = job // job-scoping reserved for later
	return all, nil
}

var (
	reStepSummary = regexp.MustCompile(`GITHUB_STEP_SUMMARY`)
	reGithubToken = regexp.MustCompile(`(?:secrets\.GITHUB_TOKEN|github\.token)`)
	reOIDCAction  = regexp.MustCompile(`(?i)uses:\s*"?(aws-actions/configure-aws-credentials|google-github-actions/auth|azure/login)`)
	reCacheAction = regexp.MustCompile(`(?i)uses:\s*"?actions/cache`)
	reArtifact    = regexp.MustCompile(`(?i)uses:\s*"?actions/(upload|download)-artifact`)
)

// Check inspects one workflow document.
func Check(file string, src []byte) ([]Finding, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return nil, nil
	}
	doc := root.Content[0]
	var out []Finding
	add := func(line int, lvl Level, what, advice string) {
		out = append(out, Finding{File: file, Line: line, Level: lvl, What: what, Advice: advice})
	}

	// --- top-level keys ---
	for k, v := range mapPairs(doc) {
		switch k.Value {
		case "concurrency":
			add(k.Line, WARN, "`concurrency` is ignored locally",
				"act runs everything; dedup/cancel-in-progress only exists on GitHub")
		case "run-name":
			add(k.Line, INFO, "`run-name` is ignored locally", "cosmetic only")
		case "permissions":
			add(k.Line, WARN, "`permissions` is ignored locally",
				"no token scoping happens in act; on GitHub this can *reduce* what a step may do")
		case "env":
			_ = v
		}
	}

	jobs := mapValue(doc, "jobs")
	if jobs == nil {
		return out, nil
	}
	for jobKey, job := range mapPairs(jobs) {
		jobID := jobKey.Value
		// runs-on
		if ro := mapValue(job, "runs-on"); ro != nil {
			roStr := flatten(ro)
			low := strings.ToLower(roStr)
			if strings.Contains(low, "windows") || strings.Contains(low, "macos") {
				add(ro.Line, ERR, fmt.Sprintf("job %q runs on %s — Docker on your machine cannot emulate it", jobID, roStr),
					"act maps it to a Linux image or self-hosted; behavior will differ fundamentally")
			}
		}
		for k := range mapPairs(job) {
			switch k.Value {
			case "environment":
				add(k.Line, WARN, fmt.Sprintf("job %q uses `environment` — environment secrets/protection rules don't exist locally", jobID),
					"pass needed secrets via --secrets-file")
			case "timeout-minutes":
				add(k.Line, INFO, fmt.Sprintf("job %q: `timeout-minutes` is not enforced locally", jobID), "")
			case "continue-on-error":
				add(k.Line, WARN, fmt.Sprintf("job %q: job-level `continue-on-error` is not respected by act", jobID),
					"a red step stops the local run even if GitHub would continue")
			case "services":
				add(k.Line, WARN, fmt.Sprintf("job %q uses `services:` — supported, but networking/health-check timing differs from GitHub-hosted", jobID),
					"if a service is flaky only locally, suspect startup timing")
			case "uses":
				add(k.Line, WARN, fmt.Sprintf("job %q calls a reusable workflow — act's support is partial", jobID),
					"remote reusable workflows + secrets inheritance are common failure points")
			}
		}
	}

	// --- text-level scans (line numbers via scan) ---
	lines := strings.Split(string(src), "\n")
	seen := map[string]bool{}
	for i, l := range lines {
		n := i + 1
		if reStepSummary.MatchString(l) && !seen["sum"] {
			seen["sum"] = true
			add(n, WARN, "$GITHUB_STEP_SUMMARY is discarded locally", "act drops summaries; don't debug them here")
		}
		if reGithubToken.MatchString(l) && !seen["tok"] {
			seen["tok"] = true
			add(n, WARN, "uses GITHUB_TOKEN — act does NOT issue one automatically",
				"pass a PAT: actdbg run -s GITHUB_TOKEN=$(gh auth token)")
		}
		if m := reOIDCAction.FindStringSubmatch(l); m != nil && !seen["oidc"] {
			seen["oidc"] = true
			add(n, ERR, fmt.Sprintf("%s needs OIDC (ACTIONS_ID_TOKEN_REQUEST_URL) — not available locally", m[1]),
				"authenticate with local credentials instead (env vars / mounted config)")
		}
		if reCacheAction.MatchString(l) && !seen["cache"] {
			seen["cache"] = true
			add(n, WARN, "actions/cache hits a local cache server, not GitHub's",
				"first run is always a miss; size/eviction semantics differ")
		}
		if reArtifact.MatchString(l) && !seen["art"] {
			seen["art"] = true
			add(n, WARN, "artifact actions need act's local artifact server",
				"runs only if --artifact-server-path is configured; cross-job artifact flow differs")
		}
		if strings.Contains(l, "id-token:") && !seen["oidc2"] {
			seen["oidc2"] = true
			add(n, ERR, "`id-token: write` (OIDC) has no local equivalent",
				"any step exchanging OIDC tokens will fail locally — fake it with local creds")
		}
	}
	return out, nil
}

// --- yaml helpers ---

func mapPairs(n *yaml.Node) map[*yaml.Node]*yaml.Node {
	out := map[*yaml.Node]*yaml.Node{}
	if n == nil || n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		out[n.Content[i]] = n.Content[i+1]
	}
	return out
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	for k, v := range mapPairs(n) {
		if k.Value == key {
			return v
		}
	}
	return nil
}

func flatten(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value
	case yaml.SequenceNode:
		var parts []string
		for _, c := range n.Content {
			parts = append(parts, flatten(c))
		}
		return strings.Join(parts, ",")
	}
	return ""
}

// --- output ---

func Render(fs []Finding) string {
	if len(fs) == 0 {
		return "✔ no known local-vs-GitHub divergences detected in this workflow.\n" +
			"  (that's about KNOWN ones — a green local run still isn't a guarantee)\n"
	}
	var b strings.Builder
	b.WriteString("fidelity report — where this workflow behaves differently under act:\n\n")
	for _, f := range fs {
		icon := map[Level]string{ERR: "✖", WARN: "⚠", INFO: "ⓘ"}[f.Level]
		fmt.Fprintf(&b, "%s %-4s %s:%d  %s\n", icon, f.Level, f.File, f.Line, f.What)
		if f.Advice != "" {
			fmt.Fprintf(&b, "         ↳ %s\n", f.Advice)
		}
	}
	b.WriteString("\nrules sourced from act's not-supported list and high-traffic act issues.\n")
	return b.String()
}

// Summary is the one-paragraph pre-run version.
func Summary(fs []Finding) string {
	var e, w int
	for _, f := range fs {
		switch f.Level {
		case ERR:
			e++
		case WARN:
			w++
		}
	}
	if e+w == 0 {
		return ""
	}
	return fmt.Sprintf("ⓘ fidelity: %d thing(s) here behave differently than on GitHub (%d won't work locally) — details: actdbg check", e+w, e)
}

func WriteJSON(w io.Writer, fs []Finding) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(fs)
}
