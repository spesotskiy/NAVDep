package nav

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Object is one accepted object text file.
type Object struct {
	Key      string // prefix plus id, for example c12
	Prefix   string
	ID       int
	TypeName string // Table, Codeunit, ...
	Name     string // name from the OBJECT header
	Path     string
	source   string // decoded file text; cleared after references are indexed
}

// Catalog is the prefix+id index of one folder and a case-insensitive name index.
// A name shared by two objects of the same type does not resolve.
type Catalog struct {
	Objects map[string]Object
	byName  map[nameKey]nameHit
}

type nameKey struct {
	prefix string
	name   string
}

type nameHit struct {
	key       string
	ambiguous bool
}

// ReadCatalog indexes top-level "*.txt" object files in folder.
// Subfolders and names that are not "{prefix}{id} - {name}.txt" are ignored.
// A file whose OBJECT type or id disagrees with the file name is skipped and
// listed in warnings. When two files share a key, the first in name order is kept.
func ReadCatalog(folder string) (*Catalog, []string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	cat := &Catalog{
		Objects: map[string]Object{},
		byName:  map[nameKey]nameHit{},
	}
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		parsed, ok := ParseFileName(entry.Name())
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %s", entry.Name(), err))
			continue
		}
		text := DecodeText(data)
		header, kind, ok := readHeader(text)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: %s", entry.Name(), kind))
			continue
		}
		wantType, _ := TypeName(parsed.Prefix)
		if !strings.EqualFold(header.TypeName, wantType) {
			warnings = append(warnings, fmt.Sprintf("%s: OBJECT type %s does not match file type %s", entry.Name(), header.TypeName, wantType))
			continue
		}
		if header.ID != parsed.ID {
			warnings = append(warnings, fmt.Sprintf("%s: OBJECT id %d does not match file id %d", entry.Name(), header.ID, parsed.ID))
			continue
		}
		if _, exists := cat.Objects[parsed.Key]; exists {
			warnings = append(warnings, fmt.Sprintf("%s: duplicate object %s", entry.Name(), parsed.Key))
			continue
		}
		obj := Object{
			Key:      parsed.Key,
			Prefix:   parsed.Prefix,
			ID:       parsed.ID,
			TypeName: wantType,
			Name:     header.Name,
			Path:     filepath.Join(folder, entry.Name()),
			source:   text,
		}
		cat.Objects[obj.Key] = obj
		cat.addName(obj.Prefix, obj.Name, obj.Key)
	}
	return cat, warnings, nil
}

// readHeader returns the first non-empty line when it is an OBJECT header.
// kind is the warning text used when the file is skipped.
func readHeader(text string) (ObjectHeader, string, bool) {
	line, err := firstContentLine(text)
	if err != nil {
		return ObjectHeader{}, err.Error(), false
	}
	if line == "" || !isObjectLine(line) {
		return ObjectHeader{}, "missing OBJECT header", false
	}
	header, ok := ParseObjectHeader(line)
	if !ok {
		return ObjectHeader{}, "invalid OBJECT header", false
	}
	return header, "", true
}

func isObjectLine(line string) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && strings.EqualFold(fields[0], "OBJECT")
}

func firstContentLine(text string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		if line == "" {
			continue
		}
		return line, nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func (c *Catalog) addName(prefix, name, key string) {
	nk := nameKey{prefix: prefix, name: strings.ToLower(name)}
	if hit, ok := c.byName[nk]; ok {
		hit.ambiguous = true
		hit.key = ""
		c.byName[nk] = hit
		return
	}
	c.byName[nk] = nameHit{key: key}
}

// Object returns the catalog entry for a key. The key match is case-insensitive.
func (c *Catalog) Object(key string) (Object, bool) {
	if c == nil {
		return Object{}, false
	}
	canon, ok := CanonicalKey(key)
	if !ok {
		return Object{}, false
	}
	obj, ok := c.Objects[canon]
	return obj, ok
}

// ResolveName returns the object key for a case-insensitive name.
// typeOrPrefix is a filename prefix (c), an OBJECT type (Codeunit), or a
// C/AL symbol (CODEUNIT, DATABASE). ok is false when the name is unknown or
// when more than one object of that type uses it.
func (c *Catalog) ResolveName(typeOrPrefix, name string) (string, bool) {
	if c == nil {
		return "", false
	}
	prefix, ok := PrefixForType(typeOrPrefix)
	if !ok {
		return "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", false
	}
	hit, ok := c.byName[nameKey{prefix: prefix, name: name}]
	if !ok || hit.ambiguous {
		return "", false
	}
	return hit.key, true
}
