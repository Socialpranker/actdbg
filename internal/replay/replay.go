// Package replay reproduces a real failed GitHub Actions run locally:
// paste the run URL, actdbg fetches what failed and re-runs that job up to
// the failure — then the normal debugger flow (shell, back, rerun) applies.
package replay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nektos/act/pkg/model"
)

var reURL = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/actions/runs/(\d+)`)

// ParseURL extracts owner, repo and run id from a run URL (job suffix ok).
func ParseURL(u string) (owner, repo, id string, err error) {
	m := reURL.FindStringSubmatch(u)
	if m == nil {
		return "", "", "", fmt.Errorf("not a run URL (want …github.com/<o>/<r>/actions/runs/<id>)")
	}
	return m[1], m[2], m[3], nil
}

type RunInfo struct {
	HeadSHA      string `json:"head_sha"`
	Path         string `json:"path"` // .github/workflows/ci.yml
	Event        string `json:"event"`
	Conclusion   string `json:"conclusion"`
	FailedJob    string // name
	FailedStep   string
	FailedNumber int
}

func get(url string, v any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API %s → %d (private repo? set GITHUB_TOKEN)", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Fetch pulls run metadata and locates the first failed job/step.
func Fetch(owner, repo, id string) (*RunInfo, error) {
	var ri RunInfo
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%s", owner, repo, id)
	if err := get(api, &ri); err != nil {
		return nil, err
	}
	var jobs struct {
		Jobs []struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
			Steps      []struct {
				Name       string `json:"name"`
				Number     int    `json:"number"`
				Conclusion string `json:"conclusion"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err := get(api+"/jobs?per_page=50", &jobs); err != nil {
		return nil, err
	}
	for _, j := range jobs.Jobs {
		if j.Conclusion == "failure" {
			ri.FailedJob = j.Name
			for _, s := range j.Steps {
				if s.Conclusion == "failure" {
					ri.FailedStep, ri.FailedNumber = s.Name, s.Number
					break
				}
			}
			break
		}
	}
	if ri.FailedJob == "" {
		return nil, fmt.Errorf("run conclusion is %q — no failed job to replay", ri.Conclusion)
	}
	return &ri, nil
}

// JobIDByName maps the API's job *name* back to the workflow's job *id*.
func JobIDByName(wfPath, name string) (string, error) {
	planner, err := model.NewWorkflowPlanner(wfPath, false, false)
	if err != nil {
		return "", err
	}
	plan, err := planner.PlanAll()
	if err != nil {
		return "", err
	}
	base := name
	if i := strings.Index(name, " ("); i > 0 { // matrix suffix "test (1.25)"
		base = name[:i]
	}
	for _, st := range plan.Stages {
		for _, run := range st.Runs {
			job := run.Workflow.GetJob(run.JobID)
			if job == nil {
				continue
			}
			if run.JobID == base || job.Name == base || job.Name == name {
				return run.JobID, nil
			}
		}
	}
	return "", fmt.Errorf("job %q not found in %s (matrix names are matched by prefix)", name, wfPath)
}

// CheckHead warns when the local checkout differs from the run's commit.
func CheckHead(sha string) (ok bool, local string) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return false, ""
	}
	local = strings.TrimSpace(string(out))
	return strings.HasPrefix(local, sha) || strings.HasPrefix(sha, local) || local == sha, local
}
