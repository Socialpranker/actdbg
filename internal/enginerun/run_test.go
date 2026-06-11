package enginerun

import "testing"

func TestParseMatrix(t *testing.T) {
	if m, err := ParseMatrix(nil); err != nil || m != nil {
		t.Fatalf("empty input should yield nil, nil — got %v, %v", m, err)
	}
	m, err := ParseMatrix([]string{"os=ubuntu-latest", "go=1.24", "go=1.25"})
	if err != nil {
		t.Fatal(err)
	}
	if !m["os"]["ubuntu-latest"] || !m["go"]["1.24"] || !m["go"]["1.25"] {
		t.Errorf("matrix filter wrong: %v", m)
	}
	if len(m) != 2 || len(m["go"]) != 2 {
		t.Errorf("unexpected shape: %v", m)
	}
	for _, bad := range []string{"novalue", "=v", "k="} {
		if _, err := ParseMatrix([]string{bad}); err == nil {
			t.Errorf("ParseMatrix(%q) should fail", bad)
		}
	}
}

func TestParsePlatformsDefaultsPreserved(t *testing.T) {
	m := ParsePlatforms([]string{"Ubuntu-Latest=node:16", "self-hosted=img"})
	if m["ubuntu-latest"] != "node:16" {
		t.Errorf("override lost: %v", m)
	}
	if m["ubuntu-22.04"] == "" {
		t.Error("defaults should be preserved")
	}
}
