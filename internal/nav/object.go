package nav

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// prefixLetters are the filename prefixes accepted as NAV object types.
const prefixLetters = "cdfmprqtx"

// objectType is one entry in the prefix table.
// Name is the OBJECT header word. Symbol is the C/AL :: type used when a
// reference names an object, such as DATABASE::"Customer" or CODEUNIT::"Post".
type objectType struct {
	Prefix string
	Name   string
	Symbol string
}

var objectTypes = []objectType{
	{Prefix: "c", Name: "Codeunit", Symbol: "CODEUNIT"},
	{Prefix: "d", Name: "Dataport", Symbol: "DATAPORT"},
	{Prefix: "f", Name: "Form", Symbol: "FORM"},
	{Prefix: "m", Name: "MenuSuite"},
	{Prefix: "p", Name: "Page", Symbol: "PAGE"},
	{Prefix: "q", Name: "Query", Symbol: "QUERY"},
	{Prefix: "r", Name: "Report", Symbol: "REPORT"},
	{Prefix: "t", Name: "Table", Symbol: "DATABASE"},
	{Prefix: "x", Name: "XMLport", Symbol: "XMLPORT"},
}

// FileName is an object text file named "{prefix}{id} - {name}.txt".
type FileName struct {
	Key    string
	Prefix string
	ID     int
	Name   string
}

// ObjectHeader is the OBJECT line at the top of an object text file.
type ObjectHeader struct {
	TypeName string
	ID       int
	Name     string
}

var fileNameRE = regexp.MustCompile(`(?i)^([` + prefixLetters + `])([0-9]+)\s*-\s*(.+)\.txt$`)
var objectHeaderRE = regexp.MustCompile(`(?i)^OBJECT\s+([A-Za-z]+)\s+([0-9]+)\s+(.+)$`)

// TypeName returns the OBJECT header word for a filename prefix.
func TypeName(prefix string) (string, bool) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	for _, typ := range objectTypes {
		if typ.Prefix == prefix {
			return typ.Name, true
		}
	}
	return "", false
}

// PrefixForType maps a filename prefix, an OBJECT type name, or a C/AL ::
// symbol onto the filename prefix. Matching is case-insensitive.
func PrefixForType(typeOrPrefix string) (string, bool) {
	s := strings.TrimSpace(typeOrPrefix)
	if s == "" {
		return "", false
	}
	if len(s) == 1 {
		p := strings.ToLower(s)
		if _, ok := TypeName(p); ok {
			return p, true
		}
		return "", false
	}
	for _, typ := range objectTypes {
		if strings.EqualFold(typ.Name, s) || (typ.Symbol != "" && strings.EqualFold(typ.Symbol, s)) {
			return typ.Prefix, true
		}
	}
	return "", false
}

// ParseFileName accepts "{prefix}{id} - {name}.txt".
// The name is everything after the first hyphen, so it may contain hyphens.
func ParseFileName(name string) (FileName, bool) {
	m := fileNameRE.FindStringSubmatch(name)
	if m == nil {
		return FileName{}, false
	}
	id, err := strconv.Atoi(m[2])
	if err != nil {
		return FileName{}, false
	}
	objectName := strings.TrimSpace(m[3])
	if objectName == "" {
		return FileName{}, false
	}
	prefix := strings.ToLower(m[1])
	return FileName{
		Key:    prefix + strconv.Itoa(id),
		Prefix: prefix,
		ID:     id,
		Name:   objectName,
	}, true
}

// ParseObjectHeader reads one OBJECT line. The name is either the rest of the
// line or a quoted identifier:
//
//	OBJECT Codeunit 50001 Exchange Management
//	OBJECT Codeunit 50001 "Exchange Management"
func ParseObjectHeader(line string) (ObjectHeader, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
	m := objectHeaderRE.FindStringSubmatch(line)
	if m == nil {
		return ObjectHeader{}, false
	}
	id, err := strconv.Atoi(m[2])
	if err != nil {
		return ObjectHeader{}, false
	}
	name, ok := parseObjectName(m[3])
	if !ok {
		return ObjectHeader{}, false
	}
	return ObjectHeader{TypeName: m[1], ID: id, Name: name}, true
}

// parseObjectName returns the header name. A leading quote selects the quoted
// form, where a doubled quote is one literal quote.
func parseObjectName(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if raw[0] != '"' {
		return raw, true
	}
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		if raw[i] != '"' {
			b.WriteByte(raw[i])
			continue
		}
		if i+1 < len(raw) && raw[i+1] == '"' {
			b.WriteByte('"')
			i++
			continue
		}
		if strings.TrimSpace(raw[i+1:]) != "" {
			return "", false
		}
		name := strings.TrimSpace(b.String())
		if name == "" {
			return "", false
		}
		return name, true
	}
	return "", false
}

// DecodeText interprets object file bytes. A UTF-8 or UTF-16 BOM selects that
// encoding. When the bytes are not valid UTF-8, they are read as Windows-1252.
func DecodeText(b []byte) string {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	} else if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return decodeUTF16(b[2:], binary.LittleEndian)
	} else if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return decodeUTF16(b[2:], binary.BigEndian)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return decodeWindows1252(b)
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(units))
}

// windows1252 maps bytes 0x80-0x9F. Undefined bytes keep the corresponding
// Unicode control character.
var windows1252 = [32]rune{
	'€', '\u0081', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\u008d', 'Ž', '\u008f',
	'\u0090', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\u009d', 'ž', 'Ÿ',
}

func decodeWindows1252(b []byte) string {
	out := make([]rune, len(b))
	for i, c := range b {
		if c >= 0x80 && c <= 0x9F {
			out[i] = windows1252[c-0x80]
			continue
		}
		out[i] = rune(c)
	}
	return string(out)
}
