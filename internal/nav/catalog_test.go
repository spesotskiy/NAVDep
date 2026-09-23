package nav

import (
	"os"
	"path/filepath"
	"testing"
)

func writeObject(t *testing.T, dir, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadCatalogIndexesNames(t *testing.T) {
	dir := t.TempDir()
	writeObject(t, dir, "c50001 - Exchange Management.txt", []byte("OBJECT Codeunit 50001 \"Exchange Management\"\r\n"))
	writeObject(t, dir, "c12 - Something Else.txt", []byte("OBJECT Codeunit 12 \"Gen. Jnl.-Post Line\"\r\n"))
	writeObject(t, dir, "t18 - Customer.txt", []byte("OBJECT Table 18 Customer\r\n"))
	writeObject(t, dir, "t19 - Other Customer.txt", []byte("OBJECT Table 19 Customer\r\n"))
	writeObject(t, dir, "p21 - Customer Card.txt", []byte("OBJECT Page 21 Customer\r\n"))
	writeObject(t, dir, "readme.txt", []byte("OBJECT Codeunit 1 Should Ignore\r\n"))
	writeObject(t, dir, "c99.txt", []byte("OBJECT Codeunit 99 Ignored\r\n"))
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeObject(t, filepath.Join(dir, "nested"), "c3 - Hidden.txt", []byte("OBJECT Codeunit 3 Hidden\r\n"))

	cp := append([]byte("OBJECT Table 20 Cr"), 0xE8, 'm', 'e', '\r', '\n')
	writeObject(t, dir, "t20 - Creme.txt", cp)
	writeObject(t, dir, "t22 - Cafe.txt", append([]byte{0xEF, 0xBB, 0xBF}, []byte("OBJECT Table 22 Café\n")...))
	writeObject(t, dir, "c11 - Check.txt", utf16Bytes("OBJECT Codeunit 11 Gen. Jnl.-Check Line\r\n", true))

	cat, warnings, err := ReadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings %v", warnings)
	}
	if len(cat.Objects) != 8 {
		t.Fatalf("objects %d", len(cat.Objects))
	}

	obj, ok := cat.Object("C50001")
	if !ok || obj.Name != "Exchange Management" || obj.TypeName != "Codeunit" || obj.ID != 50001 {
		t.Fatalf("c50001 %+v %v", obj, ok)
	}
	if key, ok := cat.ResolveName("CODEUNIT", "gen. jnl.-post line"); !ok || key != "c12" {
		t.Fatalf("codeunit name -> %s %v", key, ok)
	}
	if _, ok := cat.ResolveName("c", "Something Else"); ok {
		t.Fatal("filename name should not resolve")
	}
	if _, ok := cat.ResolveName("Table", "Customer"); ok {
		t.Fatal("shared table name should not resolve")
	}
	if key, ok := cat.ResolveName("PAGE", "customer"); !ok || key != "p21" {
		t.Fatalf("page Customer -> %s %v", key, ok)
	}
	if _, ok := cat.Object("c3"); ok {
		t.Fatal("subdirectory object was indexed")
	}
	if _, ok := cat.Object("c1"); ok {
		t.Fatal("non-matching file was indexed")
	}
	if _, ok := cat.Object("c99"); ok {
		t.Fatal("file without the name separator was indexed")
	}
	if key, ok := cat.ResolveName("DATABASE", "Crème"); !ok || key != "t20" {
		t.Fatalf("cp1252 name -> %s %v", key, ok)
	}
	if key, ok := cat.ResolveName("t", "Café"); !ok || key != "t22" {
		t.Fatalf("utf-8 name -> %s %v", key, ok)
	}
	if key, ok := cat.ResolveName("Codeunit", "Gen. Jnl.-Check Line"); !ok || key != "c11" {
		t.Fatalf("utf-16 name -> %s %v", key, ok)
	}
	if _, ok := cat.ResolveName("CODEUNIT", "Missing"); ok {
		t.Fatal("unknown name resolved")
	}
}

func TestReadCatalogSkipsAndReports(t *testing.T) {
	dir := t.TempDir()
	writeObject(t, dir, "c1 - Empty.txt", []byte("\r\nOBJECT-PROPERTIES\r\n"))
	writeObject(t, dir, "c2 - Bad.txt", []byte("OBJECT Codeunit\r\n"))
	writeObject(t, dir, "c11 - Mismatch.txt", []byte("OBJECT Codeunit 99 Mismatch\r\n"))
	writeObject(t, dir, "c12 - A.txt", []byte("OBJECT Codeunit 12 First\r\n"))
	writeObject(t, dir, "c12 - B.txt", []byte("OBJECT Codeunit 12 Second\r\n"))
	writeObject(t, dir, "p21 - Wrong Type.txt", []byte("OBJECT Codeunit 21 Wrong Type\r\n"))

	cat, warnings, err := ReadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"c1 - Empty.txt: missing OBJECT header",
		"c11 - Mismatch.txt: OBJECT id 99 does not match file id 11",
		"c12 - B.txt: duplicate object c12",
		"c2 - Bad.txt: invalid OBJECT header",
		"p21 - Wrong Type.txt: OBJECT type Codeunit does not match file type Page",
	}
	if len(warnings) != len(want) {
		t.Fatalf("warnings = %v", warnings)
	}
	for i := range want {
		if warnings[i] != want[i] {
			t.Fatalf("warnings = %v", warnings)
		}
	}
	obj, ok := cat.Object("c12")
	if !ok || obj.Name != "First" {
		t.Fatalf("kept %+v %v", obj, ok)
	}
	if len(cat.Objects) != 1 {
		t.Fatalf("objects %+v", cat.Objects)
	}
	if _, ok := cat.ResolveName("c", "Second"); ok {
		t.Fatal("skipped duplicate name resolved")
	}
}
