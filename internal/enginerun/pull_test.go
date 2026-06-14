package enginerun

import (
	"reflect"
	"strings"
	"testing"
)

func TestMissingImages(t *testing.T) {
	present := map[string]bool{
		"node:16-buster-slim":           true,
		"catthehacker/ubuntu:act-22.04": true,
	}
	cases := []struct {
		name   string
		needed []string
		want   []string
	}{
		{
			name:   "all present",
			needed: []string{"node:16-buster-slim", "catthehacker/ubuntu:act-22.04"},
			want:   nil,
		},
		{
			name:   "one missing",
			needed: []string{"catthehacker/ubuntu:act-latest"},
			want:   []string{"catthehacker/ubuntu:act-latest"},
		},
		{
			name:   "dedup and sort",
			needed: []string{"z:img", "a:img", "z:img"},
			want:   []string{"a:img", "z:img"},
		},
		{
			name:   "empty needed",
			needed: nil,
			want:   nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := missingImages(c.needed, present)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("missingImages(%v) = %v, want %v", c.needed, got, c.want)
			}
		})
	}
}

// A run with no recorded runs-on still falls back to the default medium image,
// so the warning fires on a fresh machine rather than silently pulling.
func TestImagesForRunFallsBackToDefault(t *testing.T) {
	platforms := DefaultPlatforms()
	imgs := imagesForRun(platforms, nil)
	if len(imgs) != 1 || imgs[0] != platforms["ubuntu-latest"] {
		t.Fatalf("empty runs-on should default to the medium ubuntu image, got %v", imgs)
	}
}

func TestImagesForRunResolvesRunsOn(t *testing.T) {
	platforms := DefaultPlatforms()
	imgs := imagesForRun(platforms, []string{"ubuntu-22.04", "ubuntu-latest", "ubuntu-22.04"})
	// deduped + sorted, unknown labels dropped (act would map them itself).
	want := []string{platforms["ubuntu-22.04"], platforms["ubuntu-latest"]}
	if !reflect.DeepEqual(imgs, want) {
		t.Fatalf("imagesForRun = %v, want %v", imgs, want)
	}
}

func TestPullNoticeMentionsSizeAndImages(t *testing.T) {
	n := pullNotice([]string{"catthehacker/ubuntu:act-latest"})
	for _, want := range []string{"catthehacker/ubuntu:act-latest", "GB", "one-time"} {
		if !strings.Contains(n, want) {
			t.Errorf("notice missing %q:\n%s", want, n)
		}
	}
	if pullNotice(nil) != "" {
		t.Error("no missing images should yield no notice")
	}
}
