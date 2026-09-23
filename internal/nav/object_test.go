package nav

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestPrefixesMatchTypeTable(t *testing.T) {
	if len(objectTypes) != len(prefixLetters) {
		t.Fatalf("types %d, letters %d", len(objectTypes), len(prefixLetters))
	}
	seenPrefix := map[string]bool{}
	seenName := map[string]string{}
	seenSymbol := map[string]string{}
	for _, typ := range objectTypes {
		if len(typ.Prefix) != 1 || !strings.Contains(prefixLetters, typ.Prefix) {
			t.Fatalf("prefix %q missing from %s", typ.Prefix, prefixLetters)
		}
		if seenPrefix[typ.Prefix] {
			t.Fatalf("duplicate prefix %s", typ.Prefix)
		}
		seenPrefix[typ.Prefix] = true
		name, ok := TypeName(typ.Prefix)
		if !ok || name != typ.Name {
			t.Fatalf("TypeName(%s) = %q, %v", typ.Prefix, name, ok)
		}
		if got, ok := PrefixForType(typ.Name); !ok || got != typ.Prefix {
			t.Fatalf("PrefixForType(%s) = %s, %v", typ.Name, got, ok)
		}
		if prev, ok := seenName[strings.ToLower(typ.Name)]; ok {
			t.Fatalf("type name %s used by %s and %s", typ.Name, prev, typ.Prefix)
		}
		seenName[strings.ToLower(typ.Name)] = typ.Prefix
		if typ.Symbol == "" {
			continue
		}
		if got, ok := PrefixForType(strings.ToLower(typ.Symbol)); !ok || got != typ.Prefix {
			t.Fatalf("PrefixForType(%s) = %s, %v", typ.Symbol, got, ok)
		}
		if prev, ok := seenSymbol[strings.ToLower(typ.Symbol)]; ok {
			t.Fatalf("symbol %s used by %s and %s", typ.Symbol, prev, typ.Prefix)
		}
		seenSymbol[strings.ToLower(typ.Symbol)] = typ.Prefix
		if _, ok := CanonicalKey(typ.Prefix + "1"); !ok {
			t.Fatalf("key %s1 rejected", typ.Prefix)
		}
	}
	if _, ok := PrefixForType("nope"); ok {
		t.Fatal("unknown type resolved")
	}
	if _, ok := PrefixForType(""); ok {
		t.Fatal("empty type resolved")
	}
}

func TestParseFileName(t *testing.T) {
	got, ok := ParseFileName("c50001 - Exchange Management.txt")
	if !ok {
		t.Fatal("expected a match")
	}
	want := FileName{Key: "c50001", Prefix: "c", ID: 50001, Name: "Exchange Management"}
	if got != want {
		t.Fatalf("got %+v", got)
	}

	got, ok = ParseFileName("C12 - Gen. Jnl.-Post Line.TXT")
	if !ok || got.Key != "c12" || got.Name != "Gen. Jnl.-Post Line" {
		t.Fatalf("got %+v, ok %v", got, ok)
	}

	got, ok = ParseFileName("t18 - Customer.txt")
	if !ok || got != (FileName{Key: "t18", Prefix: "t", ID: 18, Name: "Customer"}) {
		t.Fatalf("got %+v", got)
	}

	for _, name := range []string{
		"readme.txt",
		"c50001.txt",
		"c50001 Exchange Management.txt",
		"cod12 - Name.txt",
		"c - Name.txt",
		"12 - Name.txt",
		"c12 - .txt",
		"c12 - Name.txt.bak",
		"notes - something.txt",
	} {
		if _, ok := ParseFileName(name); ok {
			t.Fatalf("%s should be ignored", name)
		}
	}
}

func TestParseObjectHeader(t *testing.T) {
	h, ok := ParseObjectHeader("OBJECT Codeunit 50001 Exchange Management")
	if !ok || h != (ObjectHeader{TypeName: "Codeunit", ID: 50001, Name: "Exchange Management"}) {
		t.Fatalf("got %+v, ok %v", h, ok)
	}
	h, ok = ParseObjectHeader(`  OBJECT Codeunit 12 "Gen. Jnl.-Post Line"  `)
	if !ok || h.Name != "Gen. Jnl.-Post Line" || h.ID != 12 {
		t.Fatalf("got %+v, ok %v", h, ok)
	}
	h, ok = ParseObjectHeader(`OBJECT Codeunit 1 "Say ""Hi"""`)
	if !ok || h.Name != `Say "Hi"` {
		t.Fatalf("got %+v, ok %v", h, ok)
	}
	h, ok = ParseObjectHeader("OBJECT xmlport 50000 My Export")
	if !ok || h.TypeName != "xmlport" || h.ID != 50000 || h.Name != "My Export" {
		t.Fatalf("got %+v, ok %v", h, ok)
	}
	h, ok = ParseObjectHeader("OBJECT Table 00018 Customer")
	if !ok || h.ID != 18 || h.Name != "Customer" {
		t.Fatalf("got %+v, ok %v", h, ok)
	}
	for _, line := range []string{
		"OBJECT-PROPERTIES",
		"OBJECT Codeunit 12",
		`OBJECT Codeunit 12 ""`,
		`OBJECT Codeunit 12 "Name" extra`,
		"{",
	} {
		if _, ok := ParseObjectHeader(line); ok {
			t.Fatalf("%q should not parse", line)
		}
	}
}

func TestDecodeText(t *testing.T) {
	if got := DecodeText([]byte("OBJECT Table 18 Customer")); got != "OBJECT Table 18 Customer" {
		t.Fatalf("ascii %q", got)
	}
	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte("Café")...)
	if got := DecodeText(bom); got != "Café" {
		t.Fatalf("utf-8 bom %q", got)
	}
	if got := DecodeText([]byte("Café")); got != "Café" {
		t.Fatalf("utf-8 %q", got)
	}

	cp := []byte{'C', 'r', 0xE8, 'm', 'e'}
	if got := DecodeText(cp); got != "Crème" {
		t.Fatalf("cp1252 %q", got)
	}
	if got := DecodeText([]byte{0x80, 0x93, 0x9F, 0xE9}); got != "€“Ÿé" {
		t.Fatalf("cp1252 extras %q", got)
	}

	le := utf16Bytes("OBJECT Codeunit 11 Check", true)
	if got := DecodeText(le); got != "OBJECT Codeunit 11 Check" {
		t.Fatalf("utf-16 le %q", got)
	}
	be := utf16Bytes("OBJECT Table 18 Café", false)
	if got := DecodeText(be); got != "OBJECT Table 18 Café" {
		t.Fatalf("utf-16 be %q", got)
	}
}

func utf16Bytes(s string, little bool) []byte {
	units := utf16.Encode([]rune(s))
	out := []byte{0xFE, 0xFF}
	if little {
		out = []byte{0xFF, 0xFE}
	}
	for _, u := range units {
		hi, lo := byte(u>>8), byte(u)
		if little {
			out = append(out, lo, hi)
			continue
		}
		out = append(out, hi, lo)
	}
	return out
}
