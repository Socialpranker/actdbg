package replay

import "testing"

func TestParseURL(t *testing.T) {
	o, r, id, err := ParseURL("https://github.com/Socialpranker/actdbg/actions/runs/123456/job/789")
	if err != nil || o != "Socialpranker" || r != "actdbg" || id != "123456" {
		t.Fatalf("%s %s %s %v", o, r, id, err)
	}
	if _, _, _, err := ParseURL("https://github.com/foo/bar"); err == nil {
		t.Fatal("want error for non-run URL")
	}
}

func TestJobIDByName(t *testing.T) {
	id, err := JobIDByName("../../.github/workflows/ci.yml", "e2e")
	if err != nil || id != "e2e" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if id, err = JobIDByName("../../.github/workflows/ci.yml", "test (1.25)"); err != nil || id != "test" {
		t.Fatalf("matrix prefix: id=%q err=%v", id, err)
	}
	if _, err = JobIDByName("../../.github/workflows/ci.yml", "ghost"); err == nil {
		t.Fatal("want not-found error")
	}
}
