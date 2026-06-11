package cmdlog

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// setHome redirects the persistent log into a temp dir.
func setHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)        // unix
	t.Setenv("USERPROFILE", dir) // windows
}

func reset() {
	mu.Lock()
	recent = nil
	mu.Unlock()
}

func TestRecordAndTail(t *testing.T) {
	setHome(t)
	reset()
	Record(nil) // no-op, must not panic
	Record([]string{"docker", "ps", "-a"})
	Record([]string{"docker", "inspect", "c1"})

	got := Tail(10)
	if len(got) != 2 || got[0] != "docker ps -a" || got[1] != "docker inspect c1" {
		t.Fatalf("Tail(10) wrong: %v", got)
	}
	if got := Tail(1); len(got) != 1 || got[0] != "docker inspect c1" {
		t.Fatalf("Tail(1) wrong: %v", got)
	}
	if Tail(0) != nil {
		t.Error("Tail(0) should be nil")
	}

	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("persistent log not written: %v", err)
	}
	if !strings.Contains(string(b), "docker ps -a") {
		t.Errorf("file missing entry:\n%s", b)
	}
}

func TestRingCap(t *testing.T) {
	setHome(t)
	reset()
	for i := 0; i < ringCap+50; i++ {
		Record([]string{"docker", fmt.Sprint(i)})
	}
	got := Tail(ringCap + 100)
	if len(got) != ringCap {
		t.Fatalf("ring should cap at %d, got %d", ringCap, len(got))
	}
	if want := fmt.Sprintf("docker %d", ringCap+49); got[len(got)-1] != want {
		t.Errorf("newest entry = %q, want %q", got[len(got)-1], want)
	}
}

func TestConcurrent(t *testing.T) {
	setHome(t)
	reset()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				Record([]string{"docker", "exec", fmt.Sprintf("g%d-%d", g, i)})
				_ = Tail(3)
			}
		}(g)
	}
	wg.Wait()
	if got := len(Tail(ringCap)); got != ringCap { // 400 records > ringCap
		t.Errorf("expected a full ring after 400 records, got %d", got)
	}
}

func TestDockerHelper(t *testing.T) {
	setHome(t)
	reset()
	cmd := Docker("version", "--format", "x")
	if cmd == nil || len(cmd.Args) == 0 || cmd.Args[0] != "docker" {
		t.Fatalf("bad cmd: %+v", cmd)
	}
	if got := Tail(1); len(got) != 1 || got[0] != "docker version --format x" {
		t.Errorf("docker call not recorded: %v", got)
	}
}
