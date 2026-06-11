// Package cmdlog keeps a transparent record of every external command actdbg
// executes — the lazygit principle: a tool that touches your machine should
// never hide what it did. Commands land in an in-memory ring (for the TUI
// panel and `run --show-commands`) and are appended, timestamped, to
// ~/.actdbg/commands.log.
package cmdlog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ringCap = 200

var (
	mu     sync.Mutex
	recent []string
)

// Record notes one executed command in argv form. Safe for concurrent use;
// file write is best-effort and never fails the caller.
func Record(argv []string) {
	if len(argv) == 0 {
		return
	}
	line := strings.Join(argv, " ")
	mu.Lock()
	recent = append(recent, line)
	if len(recent) > ringCap {
		recent = recent[len(recent)-ringCap:]
	}
	mu.Unlock()
	appendToFile(line)
}

// Tail returns up to the last n recorded commands, oldest first.
func Tail(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	if n > len(recent) {
		n = len(recent)
	}
	if n <= 0 {
		return nil
	}
	out := make([]string, n)
	copy(out, recent[len(recent)-n:])
	return out
}

// Path is where the persistent command log lives ("" if no home dir).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".actdbg", "commands.log")
}

func appendToFile(line string) {
	p := Path()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
	_ = f.Close()
}

// Docker records and builds a docker command — the single chokepoint all of
// actdbg's docker calls go through.
func Docker(args ...string) *exec.Cmd {
	Record(append([]string{"docker"}, args...))
	return exec.Command("docker", args...)
}
