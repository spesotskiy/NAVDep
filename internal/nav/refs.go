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

// Keyword lists grouped by first letter. Matching still uses keywordEnd, so a
// shorter word is not taken when a longer one continues the identifier.
var (
	proseByLetter     [26][]string
	symbolsByLetter   [26][]objectType
	typeWordsByLetter [26][]typeWord
)

func init() {
	for _, name := range proseProperties {
		proseByLetter[letterIndex(name[0])] = append(proseByLetter[letterIndex(name[0])], name)
	}
	for _, typ := range objectTypes {
		if typ.Symbol == "" {
			continue
		}
		symbolsByLetter[letterIndex(typ.Symbol[0])] = append(symbolsByLetter[letterIndex(typ.Symbol[0])], typ)
	}
	for _, spec := range typeWords {
		typeWordsByLetter[letterIndex(spec.word[0])] = append(typeWordsByLetter[letterIndex(spec.word[0])], spec)
	}
}

// ExtractRefs returns the compile-time references in a NAV object text file.
// The object's own OBJECT header is ignored. Line comments and single-quoted
// C/AL strings are ignored; double quotes stay, because they are identifiers.
// Tooltips, captions, descriptions, option text, and TextConst values are ignored.
// The RDLC tag <Report xmlns= is ignored.
// TableData permission entries are ignored.
//
// Each reference is returned once, in source order. A body reference to the
// object's own id is still returned; dropping self-edges belongs to the graph.
func ExtractRefs(text string) []Ref {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = blankObjectHeader(text)
	text = maskCommentsAndStrings(text)
	text = maskProse(text)
	text = maskRdlcReportTag(text)
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

// proseProperties are object properties whose values are display text.
// Longer names come first so CaptionML is tried before Caption.
var proseProperties = []string{
	"promotedactioncategoriesml",
	"additionalsearchtermsml",
	"requestfilterheadingml",
	"instructionaltextml",
	"optioncaptionml",
	"abouttitleml",
	"abouttextml",
	"descriptionml",
	"tooltipml",
	"captionml",
	"promotedactioncategories",
	"additionalsearchterms",
	"requestfilterheading",
	"instructionaltext",
	"optioncaption",
	"abouttitle",
	"abouttext",
	"description",
	"optionstring",
	"tooltip",
	"caption",
}

// maskProse blanks tooltips, captions, descriptions, option text, and TextConst values.
// Those are hardcoded display text, not compile-time object references.
func maskProse(text string) string {
	buf := []byte(text)
	walkCode(text, func(i int) int {
		if end, ok := proseSpan(text, i); ok {
			blankSpan(buf, i, end)
			return end
		}
		if end, ok := textConstSpan(text, i); ok {
			blankSpan(buf, i, end)
			return end
		}
		return i
	})
	return string(buf)
}

func proseSpan(s string, i int) (int, bool) {
	end, ok := matchProseKeyword(s, i)
	if !ok {
		return i, false
	}
	j := skipHSpace(s, end)
	if j >= len(s) || s[j] != '=' {
		return i, false
	}
	return skipProseValue(s, j+1), true
}

func matchProseKeyword(s string, i int) (int, bool) {
	if i >= len(s) {
		return i, false
	}
	idx := letterIndex(s[i])
	if idx < 0 {
		return i, false
	}
	best := -1
	for _, name := range proseByLetter[idx] {
		end, ok := keywordEnd(s, i, name, false)
		if ok && end > best {
			best = end
		}
	}
	if best < 0 {
		return i, false
	}
	return best, true
}

func skipProseValue(s string, i int) int {
	i = skipSpace(s, i)
	if i < len(s) && s[i] == '[' {
		i = skipBrackets(s, i)
		i = skipHSpace(s, i)
		if i < len(s) && s[i] == ';' {
			i++
		}
		return i
	}
	return skipToSemicolon(s, i)
}

func skipBrackets(s string, i int) int {
	if i >= len(s) || s[i] != '[' {
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
		case '\'':
			ni := skipSingleQuoted(s, i)
			if ni <= i {
				return len(s)
			}
			i = ni
		case '[':
			depth++
			i++
		case ']':
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

func skipToSemicolon(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case '"':
			_, ni, ok := readQuoted(s, i)
			if !ok || ni <= i {
				return len(s)
			}
			i = ni
		case '\'':
			ni := skipSingleQuoted(s, i)
			if ni <= i {
				return len(s)
			}
			i = ni
		case ';':
			return i + 1
		case '}':
			return i
		default:
			i++
		}
	}
	return i
}

func skipSingleQuoted(s string, i int) int {
	if i >= len(s) || s[i] != '\'' {
		return i
	}
	i++
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

func textConstSpan(s string, i int) (int, bool) {
	if i >= len(s) || (s[i] != 'T' && s[i] != 't') {
		return i, false
	}
	end, ok := keywordEnd(s, i, "TextConst", false)
	if !ok {
		return i, false
	}
	return skipToSemicolon(s, end), true
}

func blankSpan(buf []byte, from, to int) {
	if to > len(buf) {
		to = len(buf)
	}
	for j := from; j < to; j++ {
		if buf[j] != '\n' {
			buf[j] = ' '
		}
	}
}

// maskRdlcReportTag blanks the embedded layout tag <Report xmlns= so the
// word Report is not read as a report named xmlns.
func maskRdlcReportTag(text string) string {
	const needle = "<report xmlns="
	var buf []byte
	for i := 0; i+len(needle) <= len(text); i++ {
		if text[i] != '<' {
			continue
		}
		if !strings.EqualFold(text[i:i+len(needle)], needle) {
			continue
		}
		if buf == nil {
			buf = []byte(text)
		}
		blankSpan(buf, i, i+len(needle))
		i += len(needle) - 1
	}
	if buf == nil {
		return text
	}
	return string(buf)
}

// maskTableData blanks permission entries such as TableData 81=rimd and
// TableData "Sales Header"=r. A permission does not force a recompile.
func maskTableData(text string) string {
	buf := []byte(text)
	walkCode(text, func(i int) int {
		if i >= len(text) || (text[i] != 'T' && text[i] != 't') {
			return i
		}
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

// walkCode visits indexes that are outside double-quoted identifiers and that
// can start a keyword. fn returns the index to continue from. A result <= i
// skips the rest of the current identifier, which cannot contain another keyword.
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
		if !isASCIILetter(text[i]) || !atWordStart(text, i) {
			i++
			continue
		}
		next := fn(i)
		if next <= i {
			i++
			for i < len(text) && isASCIIIdentCont(text[i]) {
				i++
			}
			continue
		}
		i = next
	}
}

func scanAt(s string, i int, add func(Ref)) (int, bool) {
	idx := -1
	if i < len(s) {
		idx = letterIndex(s[i])
	}
	switch idx {
	case 'c' - 'a', 'd' - 'a', 'f' - 'a', 'o' - 'a', 'p' - 'a', 'q' - 'a', 'r' - 'a', 't' - 'a', 'x' - 'a':
	default:
		return i, false
	}
	if next, ok := trySymbol(s, i, add); ok {
		return next, true
	}
	if idx == 'o'-'a' {
		if next, ok := tryEvent(s, i, add); ok {
			return next, true
		}
	}
	if idx == 't'-'a' {
		if next, ok := tryTableNo(s, i, add); ok {
			return next, true
		}
		if next, ok := tryRelation(s, i, add); ok {
			return next, true
		}
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
	idx := letterIndex(s[i])
	if idx < 0 || len(symbolsByLetter[idx]) == 0 {
		return i, false
	}
	bestLen := -1
	bestPrefix := ""
	bestEnd := i
	for _, typ := range symbolsByLetter[idx] {
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
	numeric, id, name, next, ok := readRefTarget(s, end)
	if !ok {
		return i, false
	}
	if spec.numberOnly && !numeric {
		return i, false
	}
	// A number or a quoted name is a reference. An unquoted name is a reference
	// only in a declaration (`: Record Customer` or `: TEMPORARY Record Customer`).
	// That skips field names such as "Record ID", control names such as
	// "Name=Page Time Sheet", and option values such as "::Codeunit THEN".
	if !numeric && name != "" && !quotedAt(s, end) && !unquotedTypeNameAllowed(s, i) {
		return i, false
	}
	if numeric {
		add(Ref{Prefix: spec.prefix, ID: id, Numeric: true})
	} else {
		add(Ref{Prefix: spec.prefix, Name: name})
	}
	return next, true
}

func quotedAt(s string, i int) bool {
	i = skipHSpace(s, i)
	return i < len(s) && s[i] == '"'
}

func unquotedTypeNameAllowed(s string, keywordStart int) bool {
	j := keywordStart
	for j > 0 && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	if j >= 2 && s[j-1] == ':' && s[j-2] == ':' {
		return false
	}
	if j > 0 && s[j-1] == ':' {
		return true
	}
	wordEnd := j
	for j > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if !isIdentCont(r) {
			break
		}
		j -= size
	}
	return strings.EqualFold(s[j:wordEnd], "TEMPORARY")
}

func matchTypeWord(s string, i int) (typeWord, int, bool) {
	var best typeWord
	bestEnd := -1
	idx := -1
	if i < len(s) {
		idx = letterIndex(s[i])
	}
	if idx < 0 {
		return typeWord{}, i, false
	}
	for _, spec := range typeWordsByLetter[idx] {
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
	name, next, ok = readObjectName(s, i)
	if !ok {
		return false, 0, "", i, false
	}
	return false, 0, name, next, true
}

// readObjectName reads an unquoted object name. '/' and '-' stay inside the
// name so Country/Region and To-do are not split into Country and To.
func readObjectName(s string, i int) (string, int, bool) {
	start := i
	_, next, ok := readIdent(s, i)
	if !ok {
		return "", i, false
	}
	for next < len(s) && (s[next] == '/' || s[next] == '-') {
		_, after, ok := readIdent(s, next+1)
		if !ok {
			break
		}
		next = after
	}
	return s[start:next], next, true
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
	n := len(kw)
	if i+n > len(s) || !atWordStart(s, i) || !hasKeywordPrefix(s[i:i+n], kw) {
		return i, false
	}
	end := i + n
	if end >= len(s) {
		return end, true
	}
	c := s[end]
	if c < utf8.RuneSelf {
		if c >= '0' && c <= '9' {
			if allowDigit {
				return end, true
			}
			return i, false
		}
		if isASCIILetter(c) || c == '_' {
			return i, false
		}
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

func hasKeywordPrefix(s, kw string) bool {
	for i := 0; i < len(kw); i++ {
		a := s[i]
		b := kw[i]
		if a == b {
			continue
		}
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a == b && a < utf8.RuneSelf {
			continue
		}
		if a >= utf8.RuneSelf || b >= utf8.RuneSelf {
			return strings.EqualFold(s, kw)
		}
		return false
	}
	return true
}

func letterIndex(b byte) int {
	if b >= 'a' && b <= 'z' {
		return int(b - 'a')
	}
	if b >= 'A' && b <= 'Z' {
		return int(b - 'A')
	}
	return -1
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIIdentCont(b byte) bool {
	return isASCIILetter(b) || (b >= '0' && b <= '9') || b == '_'
}

func matchKeyword(s string, i int, kw string) (int, bool) {
	return keywordEnd(s, i, kw, false)
}

func atWordStart(s string, i int) bool {
	if i <= 0 {
		return true
	}
	prev := s[i-1]
	if prev < utf8.RuneSelf {
		return prev != '_' && !isASCIILetter(prev) && (prev < '0' || prev > '9')
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
