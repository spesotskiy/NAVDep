package nav

import (
	"os"
	"path/filepath"
	"strings"
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
	if sum.Objects != 0 || sum.Links != 0 || sum.Unresolved != 0 || len(sum.Warnings) != 0 {
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

func TestBuildCatalogsObjects(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"c12 - Gen. Jnl.-Post Line.txt": "OBJECT Codeunit 12 \"Gen. Jnl.-Post Line\"\r\n",
		"t81 - Gen. Journal Line.txt":   "OBJECT Table 81 Gen. Journal Line\r\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g, sum, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Objects != 2 || sum.Links != 0 || sum.Unresolved != 0 || len(sum.Warnings) != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if g.names["c12"] != "Gen. Jnl.-Post Line" || g.names["t81"] != "Gen. Journal Line" {
		t.Fatalf("names = %+v", g.names)
	}
	if key, ok := g.catalog.ResolveName("Codeunit", "Gen. Jnl.-Post Line"); !ok || key != "c12" {
		t.Fatalf("resolve = %s %v", key, ok)
	}
	if got := g.Dependents("c12"); len(got) != 0 {
		t.Fatalf("dependents = %+v", got)
	}
}

func TestBuildFillsReverseMap(t *testing.T) {
	g, sum := buildTestdata(t)
	if sum.Objects != 7 || sum.Links != 4 || sum.Unresolved != 3 || len(sum.Warnings) != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	assertDependents(t, g, "c12",
		Dependent{Key: "c11", Name: "Gen. Jnl.-Check Line"},
		Dependent{Key: "t81", Name: "Gen. Journal Line"},
	)
	assertDependents(t, g, "c99")
	wantNames := []UnresolvedName{
		{Prefix: "c", Name: "Also Named", Count: 1, Reason: "ambiguous", Callers: []string{"c50002"}},
		{Prefix: "c", Name: "Missing", Count: 1, Reason: "missing", Callers: []string{"t81"}},
		{Prefix: "t", Name: "Customer", Count: 1, Reason: "missing", Callers: []string{"t81"}},
	}
	if len(sum.UnresolvedNames) != len(wantNames) {
		t.Fatalf("unresolved names = %+v", sum.UnresolvedNames)
	}
	for i := range wantNames {
		got := sum.UnresolvedNames[i]
		want := wantNames[i]
		if got.Prefix != want.Prefix || got.Name != want.Name || got.Count != want.Count || got.Reason != want.Reason || len(got.Callers) != 1 || got.Callers[0] != want.Callers[0] {
			t.Fatalf("unresolved names = %+v, want %+v", sum.UnresolvedNames, wantNames)
		}
	}
}

func TestPageSourceTableDependent(t *testing.T) {
	g, _ := buildTestdata(t)
	assertDependents(t, g, "t18", Dependent{Key: "p21", Name: "Customer Card"})
	assertDependents(t, g, "p21", Dependent{Key: "c80", Name: "Open Customer Card"})
}

func buildTestdata(t *testing.T) (*Graph, Summary) {
	t.Helper()
	g, sum, err := Build("testdata")
	if err != nil {
		t.Fatal(err)
	}
	return g, sum
}

func assertDependents(t *testing.T, g *Graph, key string, want ...Dependent) {
	t.Helper()
	got := g.Dependents(key)
	if len(got) != len(want) {
		t.Fatalf("Dependents(%s) = %+v, want %+v", key, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Dependents(%s) = %+v, want %+v", key, got, want)
		}
	}
}

func TestFormatDependentsGroupsIDsByType(t *testing.T) {
	deps := []Dependent{
		{Key: "c2"},
		{Key: "c11"},
		{Key: "c12"},
		{Key: "t17"},
		{Key: "t81"},
	}
	got := FormatDependents(deps)
	want := `{"c":"2|11|12","t":"17|81"}`
	if got != want {
		t.Fatalf("format = %s, want %s", got, want)
	}
	if FormatDependents(nil) != "{}" {
		t.Fatal("empty dependents should be {}")
	}
}

func TestUnusedSkipsReferencedObjects(t *testing.T) {
	g, _ := buildTestdata(t)
	got := g.Unused()
	want := []UnusedObject{
		{Key: "c11", Name: "Gen. Jnl.-Check Line"},
		{Key: "c80", Name: "Open Customer Card"},
		{Key: "c50001", Name: "Also Named"},
		{Key: "c50002", Name: "Also Named"},
		{Key: "t81", Name: "Gen. Journal Line"},
	}
	if len(got) != len(want) {
		t.Fatalf("unused = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unused = %+v, want %+v", got, want)
		}
	}
}

func TestWriteUnusedLog(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteUnusedLog(dir, `C:\NAV\Objects`, []UnusedObject{
		{Key: "t81", Name: "Gen. Journal Line"},
		{Key: "c80", Name: "Open Customer Card"},
		{Key: "c11", Name: "Gen. Jnl.-Check Line"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"Count": 2`) || !strings.Contains(text, `"11 - Gen. Jnl.-Check Line"`) || !strings.Contains(text, `"81 - Gen. Journal Line"`) {
		t.Fatalf("log = %s", text)
	}
	codeunits := strings.Index(text, `"codeunits"`)
	tables := strings.Index(text, `"tables"`)
	if codeunits < 0 || tables < 0 || codeunits > tables {
		t.Fatalf("groups out of order:\n%s", text)
	}
	empty, err := WriteUnusedLog(dir, `C:\NAV\Objects`, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"unused": 0`) || strings.Contains(string(data), `"codeunits"`) {
		t.Fatalf("empty log = %s", data)
	}
}

func TestWriteUnresolvedLog(t *testing.T) {
	dir := t.TempDir()
	sum := Summary{
		Unresolved: 1,
		UnresolvedNames: []UnresolvedName{
			{Prefix: "c", Name: "Missing", Count: 1, Reason: "missing", Callers: []string{"t81"}},
		},
	}
	path, err := WriteUnresolvedLog(dir, `C:\NAV\Objects`, sum)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"name": "Missing"`) || !strings.Contains(text, `"reason": "missing"`) {
		t.Fatalf("log = %s", text)
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
