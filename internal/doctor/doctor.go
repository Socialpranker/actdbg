// Package doctor diagnoses the local environment against the most common
// act pitfalls (sourced from act's top-reacted issues).
package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type check struct {
	name string
	run  func() (ok bool, detail string)
}

// Run executes all checks and prints a report. Always returns nil: doctor
// diagnoses, it doesn't fail.
func Run(w io.Writer) error {
	checks := []check{
		{"docker CLI", func() (bool, string) {
			p, err := exec.LookPath("docker")
			if err != nil {
				return false, "docker not found in PATH — install Docker or Podman with docker socket"
			}
			return true, p
		}},
		{"docker daemon", func() (bool, string) {
			if err := exec.Command("docker", "info", "--format", "ok").Run(); err != nil {
				return false, "daemon unreachable — is Docker running? (act issue #1798: on Linux also check docker group membership)"
			}
			return true, "reachable"
		}},
		{"host architecture", func() (bool, string) {
			if runtime.GOARCH == "arm64" {
				return true, "arm64 — if an action ships amd64-only binaries, run with --arch linux/amd64 (slower, emulated)"
			}
			return true, runtime.GOARCH
		}},
		{"runner image", func() (bool, string) {
			out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
			if err != nil {
				return false, "cannot list images"
			}
			imgs := string(out)
			if strings.Contains(imgs, "catthehacker/ubuntu") {
				return true, "catthehacker/ubuntu present"
			}
			return false, "no catthehacker/ubuntu image yet — first run will pull ~1–2 GB.\n" +
				"      act's tiny default image causes the classic 'command not found' (act issues #107, #973);\n" +
				"      actdbg defaults to the medium image to avoid that."
		}},
		{"docker.sock permissions", func() (bool, string) {
			if runtime.GOOS != "linux" {
				return true, "n/a on " + runtime.GOOS
			}
			fi, err := os.Stat("/var/run/docker.sock")
			if err != nil {
				return false, "no /var/run/docker.sock"
			}
			return true, fmt.Sprintf("present (%s)", fi.Mode())
		}},
		{"state dir", func() (bool, string) {
			home, err := os.UserHomeDir()
			if err != nil {
				return false, err.Error()
			}
			d := home + "/.actdbg"
			if err := os.MkdirAll(d, 0o700); err != nil {
				return false, err.Error()
			}
			return true, d
		}},
	}

	fmt.Fprintln(w, "actdbg doctor — environment vs the usual act pitfalls")
	fmt.Fprintln(w)
	bad := 0
	for _, c := range checks {
		ok, detail := c.run()
		mark := "✔"
		if !ok {
			mark = "✖"
			bad++
		}
		fmt.Fprintf(w, " %s %-24s %s\n", mark, c.name, detail)
	}
	fmt.Fprintln(w)
	if bad == 0 {
		fmt.Fprintln(w, "all good. next: actdbg run")
	} else {
		fmt.Fprintf(w, "%d issue(s) above will bite before any workflow does.\n", bad)
	}
	return nil
}
