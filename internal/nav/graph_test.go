package nav

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRejectsMissingAndNonDirectory(t *testing.T) {
	if _, _, err := Build(""); err == nil {
		t.Fatal("empty folder should fail")
	}
	if _, _, err := Build(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing folder should fail")
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Build(file); err == nil {
		t.Fatal("file should fail")
	}
}

func TestBuildEmptyGraph(t *testing.T) {
	g, sum, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{}) {
		t.Fatalf("summary = %+v", sum)
	}
	if got := g.Dependents("c12"); len(got) != 0 {
		t.Fatalf("dependents = %+v", got)
	}
	if _, ok := CanonicalKey("C12"); !ok {
		t.Fatal("C12 should be a key")
	}
	if _, ok := CanonicalKey("12"); ok {
		t.Fatal("bare id should be rejected")
	}
}

func TestDependentsSortedByTypeThenID(t *testing.T) {
	g := &Graph{
		callers: map[string]map[string]struct{}{
			"c12": {
				"t81": {},
				"c11": {},
				"c2":  {},
			},
		},
		names: map[string]string{
			"c11": "Gen. Jnl.-Check Line",
			"t81": "Gen. Journal Line",
			"c2":  "Other",
		},
	}
	got := g.Dependents("C12")
	want := []Dependent{
		{Key: "c2", Name: "Other"},
		{Key: "c11", Name: "Gen. Jnl.-Check Line"},
		{Key: "t81", Name: "Gen. Journal Line"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
}
