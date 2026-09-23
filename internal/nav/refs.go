package nav

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Ref is one compile-time reference found in an object text file.
// A numeric ref has Numeric set and an id, so Key is the object key even when
// that object file is not in the folder. A name ref has Numeric false and Name
// set; the caller resolves it with Catalog.ResolveName. This function does not
// see the catalog, so it does not decide whether a name is unresolved.
type Ref struct {
	Prefix  string
	ID      int
	Name    string
	Numeric bool
}

// Key returns the prefix+id key of a numeric reference.
func (r Ref) Key() (string, bool) {
	if !r.Numeric || r.Prefix == "" {
		return "", false
	}
	return r.Prefix + strconv.Itoa(r.ID), true
}

type typeWord struct {
	word       string
	prefix     string
	glued      bool // Table18, Page22: the id may sit against the keyword
	numberOnly bool
}

// typeWords are the C/AL keywords that introduce an object reference.
// glued allows the property spelling Table18, Page22, and Form21.
// numberOnly keeps Table from accepting a name; SourceTable carries an id.
var typeWords = []typeWord{
	{word: "dataport", prefix: "d", glued: true},
	{word: "codeunit", prefix: "c"},
	{word: "xmlport", prefix: "x"},
	{word: "record", prefix: "t"},
	{word: "report", prefix: "r"},
	{word: "query", prefix: "q"},
	{word: "page", prefix: "p", glued: true},
	{word: "form", prefix: "f", glued: true},
	{word: "table", prefix: "t", glued: true, numberOnly: true},
}

// ExtractRefs returns the compile-time references in a NAV object text file.
// The object's own OBJECT header is ignored. Line comments and single-quoted
// C/AL strings are ignored; double quotes stay, because they are identifiers.
// TableData permission entries are ignored.
//
// Each reference is returned once, in source order. A body reference to the
// object's own id is still returned; dropping self-edges belongs to the graph.
func ExtractRefs(text string) []Ref {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = blankObjectHeader(text)
	text = maskCommentsAndStrings(text)
	text = maskTableData(text)
	c := &collector{}
	walkCode(text, func(i int) int {
		if next, ok := scanAt(text, i, c.add); ok {
			return next
		}
		return i
	})
	return c.refs
}

type collector struct {
	refs []Ref
	seen map[string]struct{}
}

func (c *collector) add(r Ref) {
	r.Prefix = strings.ToLower(r.Prefix)
	if r.Prefix == "" {
		return
	}
	var id string
	if r.Numeric {
		r.Name = ""
		id = r.Prefix + "#" + strconv.Itoa(r.ID)
	} else {
		r.Name = strings.TrimSpace(r.Name)
		r.ID = 0
		if r.Name == "" {
			return
		}
		id = r.Prefix + "~" + strings.ToLower(r.Name)
	}
	if c.seen == nil {
		c.seen = map[string]struct{}{}
	}
	if _, ok := c.seen[id]; ok {
		return
	}
	c.seen[id] = struct{}{}
	c.refs = append(c.refs, r)
}

// blankObjectHeader replaces the first OBJECT line with spaces so
// "OBJECT Codeunit 12 Name" is not a reference to c12.
func blankObjectHeader(text string) string {
	lineStart := 0
	for lineStart < len(text) {
		rel := strings.IndexByte(text[lineStart:], '\n')
		lineEnd := len(text)
		next := len(text)
		if rel >= 0 {
			lineEnd = lineStart + rel
			next = lineEnd + 1
		}
		trimmed := strings.TrimSpace(strings.TrimPrefix(text[lineStart:lineEnd], "\uFEFF"))
		if trimmed == "" {
			if rel < 0 {
				return text
			}
			lineStart = next
			continue
		}
		if !isObjectLine(trimmed) {
			return text
		}
		buf := []byte(text)
		for i := lineStart; i < lineEnd; i++ {
			buf[i] = ' '
		}
		return string(buf)
	}
	return text
}

// maskCommentsAndStrings drops // comments and single-quoted strings.
// Double-quoted identifiers are copied through, including doubled quotes.
func maskCommentsAndStrings(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(text) {
		if text[i] == '/' && i+1 < len(text) && text[i+1] == '/' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if text[i] == '\'' {
			i++
			for i < len(text) && text[i] != '\n' {
				if text[i] == '\'' {
					if i+1 < len(text) && text[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			b.WriteByte(' ')
			continue
		}
		if text[i] == '"' {
			b.WriteByte('"')
			i++
			for i < len(text) && text[i] != '\n' {
				if text[i] == '"' {
					if i+1 < len(text) && text[i+1] == '"' {
						b.WriteString(`""`)
						i += 2
						continue
					}
					b.WriteByte('"')
					i++
					break
				}
				b.WriteByte(text[i])
				i++
			}
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// maskTableData blanks permission entries such as TableData 81=rimd and
// TableData "Sales Header"=r. A permission does not force a recompile.
func maskTableData(text string) string {
	buf := []byte(text)
	walkCode(text, func(i int) int {
		end, ok := keywordEnd(text, i, "TableData", false)
		if !ok {
			return i
		}
		span := tableDataSpanEnd(text, end)
		if span <= i {
			return i
		}
		for j := i; j < span && j < len(buf); j++ {
			if buf[j] != '\n' {
				buf[j] = ' '
			}
		}
		return span
	})
	return string(buf)
}

func tableDataSpanEnd(s string, i int) int {
	i = skipHSpace(s, i)
	if i >= len(s) {
		return i
	}
	switch {
	case s[i] == '"':
		_, ni, ok := readQuoted(s, i)
		if !ok || ni <= i {
			return len(s)
		}
		i = ni
	case s[i] >= '0' && s[i] <= '9':
		_, ni, ok := readInt(s, i)
		if !ok {
			return i
		}
		i = ni
	default:
		_, ni, ok := readIdent(s, i)
		if !ok {
			return i
		}
		i = ni
	}
	j := skipHSpace(s, i)
	if j < len(s) && s[j] == '=' {
		j++
		j = skipHSpace(s, j)
		for j < len(s) {
			r, size := utf8.DecodeRuneInString(s[j:])
			if !unicode.IsLetter(r) {
				break
			}
			j += size
		}
		return j
	}
	return i
}

// walkCode visits indexes that are outside double-quoted identifiers.
// fn returns the index to continue from. A result <= i advances one rune.
func walkCode(text string, fn func(i int) int) {
	i := 0
	for i < len(text) {
		if text[i] == '"' {
			_, ni, _ := readQuoted(text, i)
			if ni <= i {
				i++
			} else {
				i = ni
			}
			continue
		}
		next := fn(i)
		if next <= i {
			_, size := utf8.DecodeRuneInString(text[i:])
			if size < 1 {
				size = 1
			}
			i += size
			continue
		}
		i = next
	}
}

func scanAt(s string, i int, add func(Ref)) (int, bool) {
	if next, ok := trySymbol(s, i, add); ok {
		return next, true
	}
	if next, ok := tryEvent(s, i, add); ok {
		return next, true
	}
	if next, ok := tryTableNo(s, i, add); ok {
		return next, true
	}
	if next, ok := tryRelation(s, i, add); ok {
		return next, true
	}
	if next, ok := tryTyped(s, i, add); ok {
		return next, true
	}
	return i, false
}

func trySymbol(s string, i int, add func(Ref)) (int, bool) {
	if !atWordStart(s, i) {
		return i, false
	}
	bestLen := -1
	bestPrefix := ""
	bestEnd := i
	for _, typ := range objectTypes {
		if typ.Symbol == "" {
			continue
		}
		end, ok := keywordEnd(s, i, typ.Symbol, false)
		if !ok || len(typ.Symbol) <= bestLen {
			continue
		}
		j := skipHSpace(s, end)
		if j+1 >= len(s) || s[j] != ':' || s[j+1] != ':' {
			continue
		}
		bestLen = len(typ.Symbol)
		bestPrefix = typ.Prefix
		bestEnd = j + 2
	}
	if bestLen < 0 {
		return i, false
	}
	return addTarget(s, bestEnd, bestPrefix, false, add)
}

// tryEvent matches [EventSubscriber(ObjectType::Codeunit, 80, ...)].
// The symbolic form ObjectType::Codeunit, CODEUNIT::"Sales-Post" is left for
// trySymbol. Tables use ObjectType::Table; the name form uses DATABASE::.
func tryEvent(s string, i int, add func(Ref)) (int, bool) {
	end, ok := keywordEnd(s, i, "ObjectType", false)
	if !ok {
		return i, false
	}
	j := skipSpace(s, end)
	if j+1 >= len(s) || s[j] != ':' || s[j+1] != ':' {
		return i, false
	}
	j = skipSpace(s, j+2)
	typeName, j, ok := readIdent(s, j)
	if !ok {
		return i, false
	}
	j = skipSpace(s, j)
	if j >= len(s) || s[j] != ',' {
		return i, false
	}
	j = skipSpace(s, j+1)
	id, next, ok := readInt(s, j)
	if !ok {
		return i, false
	}
	prefix, ok := PrefixForType(typeName)
	if !ok {
		return i, false
	}
	add(Ref{Prefix: prefix, ID: id, Numeric: true})
	return next, true
}

func tryTableNo(s string, i int, add func(Ref)) (int, bool) {
	end, ok := keywordEnd(s, i, "TableNo", false)
	if !ok {
		return i, false
	}
	j := skipHSpace(s, end)
	if j >= len(s) || s[j] != '=' {
		return i, false
	}
	j = skipHSpace(s, j+1)
	id, next, ok := readInt(s, j)
	if !ok {
		return i, false
	}
	add(Ref{Prefix: "t", ID: id, Numeric: true})
	return next, true
}

func tryRelation(s string, i int, add func(Ref)) (int, bool) {
	end, ok := keywordEnd(s, i, "TableRelation", false)
	if !ok {
		return i, false
	}
	j := skipSpace(s, end)
	if j >= len(s) || s[j] != '=' {
		return i, false
	}
	val, endVal := readRelationValue(s, j+1)
	parseTableRelation(val, add)
	return endVal, true
}

func tryTyped(s string, i int, add func(Ref)) (int, bool) {
	spec, end, ok := matchTypeWord(s, i)
	if !ok {
		return i, false
	}
	return addTarget(s, end, spec.prefix, spec.numberOnly, add)
}

func matchTypeWord(s string, i int) (typeWord, int, bool) {
	var best typeWord
	bestEnd := -1
	for _, spec := range typeWords {
		end, ok := keywordEnd(s, i, spec.word, spec.glued)
		if !ok {
			continue
		}
		if end > bestEnd {
			best = spec
			bestEnd = end
		}
	}
	if bestEnd < 0 {
		return typeWord{}, i, false
	}
	return best, bestEnd, true
}

func addTarget(s string, i int, prefix string, numberOnly bool, add func(Ref)) (int, bool) {
	numeric, id, name, next, ok := readRefTarget(s, i)
	if !ok {
		return i, false
	}
	if numberOnly && !numeric {
		return i, false
	}
	if numeric {
		add(Ref{Prefix: prefix, ID: id, Numeric: true})
	} else {
		add(Ref{Prefix: prefix, Name: name})
	}
	return next, true
}

// parseTableRelation reads targets from a TableRelation value, including
// IF / ELSE chains. CONST values inside conditions are not targets.
// WHERE filters are skipped. The table is the name before the field dot:
// Customer.No. and "Sales Header"."No.".
func parseTableRelation(val string, add func(Ref)) {
	i := 0
	for guard := 0; guard < 10000 && i < len(val); guard++ {
		start := i
		i = skipSpace(val, i)
		if i >= len(val) {
			return
		}
		if next, ok := matchKeyword(val, i, "IF"); ok {
			i = skipSpace(val, next)
			if i < len(val) && val[i] == '(' {
				i = skipBalanced(val, i)
			}
			if i == start {
				return
			}
			continue
		}
		numeric, id, name, next, ok := readRefTarget(val, i)
		if !ok {
			return
		}
		if numeric {
			add(Ref{Prefix: "t", ID: id, Numeric: true})
		} else {
			add(Ref{Prefix: "t", Name: name})
		}
		i = skipSpace(val, next)
		if i < len(val) && val[i] == '.' {
			i = skipField(val, i+1)
			i = skipSpace(val, i)
		}
		if next, ok := matchKeyword(val, i, "WHERE"); ok {
			i = skipSpace(val, next)
			if i < len(val) && val[i] == '(' {
				i = skipBalanced(val, i)
			}
			i = skipSpace(val, i)
		}
		if next, ok := matchKeyword(val, i, "ELSE"); ok {
			i = next
			continue
		}
		return
	}
}

func readRelationValue(s string, i int) (string, int) {
	start := i
	depth := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			_, ni, ok := readQuoted(s, i)
			if !ok || ni <= i {
				return s[start:], len(s)
			}
			i = ni
		case '(':
			depth++
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			i++
		case ';', '}':
			if depth == 0 {
				return s[start:i], i
			}
			i++
		default:
			i++
		}
	}
	return s[start:], i
}

func skipField(s string, i int) int {
	i = skipSpace(s, i)
	if i >= len(s) {
		return i
	}
	if s[i] == '"' {
		_, ni, ok := readQuoted(s, i)
		if !ok || ni <= i {
			return len(s)
		}
		return ni
	}
	_, ni, ok := readIdent(s, i)
	if !ok {
		return i
	}
	i = ni
	for i < len(s) && s[i] == '.' {
		j := i + 1
		if j >= len(s) || !isIdentStartAt(s, j) || isRelationKeyword(s, j) {
			return j
		}
		_, ni, ok = readIdent(s, j)
		if !ok {
			return j
		}
		i = ni
	}
	return i
}

func isRelationKeyword(s string, i int) bool {
	_, ok := matchKeyword(s, i, "ELSE")
	if ok {
		return true
	}
	_, ok = matchKeyword(s, i, "WHERE")
	if ok {
		return true
	}
	_, ok = matchKeyword(s, i, "IF")
	return ok
}

func skipBalanced(s string, i int) int {
	if i >= len(s) || s[i] != '(' {
		return i
	}
	depth := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			_, ni, ok := readQuoted(s, i)
			if !ok || ni <= i {
				return len(s)
			}
			i = ni
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return i
}

func readRefTarget(s string, i int) (numeric bool, id int, name string, next int, ok bool) {
	i = skipHSpace(s, i)
	if i >= len(s) {
		return false, 0, "", i, false
	}
	if s[i] == '"' {
		name, next, ok = readQuoted(s, i)
		if !ok || name == "" {
			return false, 0, "", i, false
		}
		return false, 0, name, next, true
	}
	if s[i] >= '0' && s[i] <= '9' {
		id, next, ok = readInt(s, i)
		if !ok {
			return false, 0, "", i, false
		}
		return true, id, "", next, true
	}
	name, next, ok = readIdent(s, i)
	if !ok {
		return false, 0, "", i, false
	}
	return false, 0, name, next, true
}

func readQuoted(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '"' {
		return "", i, false
	}
	var b strings.Builder
	i++
	for i < len(s) {
		if s[i] != '"' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '"' {
			b.WriteByte('"')
			i += 2
			continue
		}
		i++
		return strings.TrimSpace(b.String()), i, true
	}
	return "", i, false
}

func readIdent(s string, i int) (string, int, bool) {
	if !isIdentStartAt(s, i) {
		return "", i, false
	}
	start := i
	_, size := utf8.DecodeRuneInString(s[i:])
	i += size
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isIdentCont(r) {
			break
		}
		i += size
	}
	return s[start:i], i, true
}

func readInt(s string, i int) (int, int, bool) {
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return 0, i, false
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	n, err := strconv.Atoi(s[i:j])
	if err != nil {
		return 0, i, false
	}
	return n, j, true
}

func keywordEnd(s string, i int, kw string, allowDigit bool) (int, bool) {
	if !atWordStart(s, i) {
		return i, false
	}
	if i+len(kw) > len(s) || !strings.EqualFold(s[i:i+len(kw)], kw) {
		return i, false
	}
	end := i + len(kw)
	if end >= len(s) {
		return end, true
	}
	r, _ := utf8.DecodeRuneInString(s[end:])
	if unicode.IsDigit(r) {
		if allowDigit {
			return end, true
		}
		return i, false
	}
	if isIdentCont(r) {
		return i, false
	}
	return end, true
}

func matchKeyword(s string, i int, kw string) (int, bool) {
	return keywordEnd(s, i, kw, false)
}

func atWordStart(s string, i int) bool {
	if i <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return !isIdentCont(r)
}

func isIdentStartAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return isIdentStart(r)
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func skipSpace(s string, i int) int {
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			return i
		}
		i += size
	}
	return i
}

func skipHSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}
