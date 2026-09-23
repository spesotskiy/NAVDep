package nav

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// keyPattern is a lowercase type prefix plus a numeric id, as in c12 or t81.
var keyPattern = regexp.MustCompile(`^[` + prefixLetters + `][0-9]+$`)

// Summary is the short result of a build.
type Summary struct {
	Objects    int
	Unresolved int
	Links      int
	// Warnings are object files that were skipped, for example because the
	// OBJECT id does not match the file name. The build still replaces the map.
	Warnings []string
}

// Dependent is one object that references the requested key.
type Dependent struct {
	Key  string
	Name string
}

// Graph is the reverse map from callee key to the objects that reference it.
type Graph struct {
	callers map[string]map[string]struct{}
	names   map[string]string
	catalog *Catalog
}

// CanonicalKey returns the lowercase object key, or false when raw is not a
// type prefix plus an id.
func CanonicalKey(raw string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if !keyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

// Build catalogs NAV object text files in folder (top level only) and returns
// the reverse map from each referenced object to the objects that reference it.
// A numeric reference is a key even when that file is absent. A name that does
// not resolve is counted as unresolved and adds no edge. An object is not a
// dependent of itself. Each caller is stored once.
func Build(folder string) (*Graph, Summary, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return nil, Summary{}, errors.New("build requires a folder")
	}
	info, err := os.Stat(folder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Summary{}, fmt.Errorf("folder not found: %s", folder)
		}
		return nil, Summary{}, err
	}
	if !info.IsDir() {
		return nil, Summary{}, fmt.Errorf("not a folder: %s", folder)
	}
	cat, warnings, err := ReadCatalog(folder)
	if err != nil {
		return nil, Summary{}, err
	}
	g := &Graph{
		callers: map[string]map[string]struct{}{},
		names:   make(map[string]string, len(cat.Objects)),
		catalog: cat,
	}
	for key, obj := range cat.Objects {
		g.names[key] = obj.Name
	}
	links, unresolved := g.indexRefs(&warnings)
	return g, Summary{
		Objects:    len(cat.Objects),
		Unresolved: unresolved,
		Links:      links,
		Warnings:   warnings,
	}, nil
}

// indexRefs reads each cataloged file and records its compile-time references.
func (g *Graph) indexRefs(warnings *[]string) (links, unresolved int) {
	keys := make([]string, 0, len(g.catalog.Objects))
	for key := range g.catalog.Objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		obj := g.catalog.Objects[key]
		data, err := os.ReadFile(obj.Path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s: %s", filepath.Base(obj.Path), err))
			continue
		}
		for _, ref := range ExtractRefs(DecodeText(data)) {
			callee, ok := resolveRef(g.catalog, ref)
			if !ok {
				unresolved++
				continue
			}
			if callee == obj.Key {
				continue
			}
			if g.addCaller(callee, obj.Key) {
				links++
			}
		}
	}
	return links, unresolved
}

// resolveRef turns a numeric reference into its key, or a name into a catalog key.
func resolveRef(cat *Catalog, ref Ref) (string, bool) {
	if ref.Numeric {
		return ref.Key()
	}
	return cat.ResolveName(ref.Prefix, ref.Name)
}

// addCaller records caller as a dependent of callee. It reports whether the edge is new.
func (g *Graph) addCaller(callee, caller string) bool {
	set := g.callers[callee]
	if set == nil {
		set = map[string]struct{}{}
		g.callers[callee] = set
	}
	if _, ok := set[caller]; ok {
		return false
	}
	set[caller] = struct{}{}
	return true
}

// Dependents returns the callers of key, sorted by type prefix then numeric id.
// Each caller appears once. The key match is case-insensitive.
func (g *Graph) Dependents(key string) []Dependent {
	if g == nil {
		return nil
	}
	canon, ok := CanonicalKey(key)
	if !ok {
		return nil
	}
	set := g.callers[canon]
	out := make([]Dependent, 0, len(set))
	for caller := range set {
		out = append(out, Dependent{Key: caller, Name: g.names[caller]})
	}
	sort.Slice(out, func(i, j int) bool {
		return lessKey(out[i].Key, out[j].Key)
	})
	return out
}

// FormatDependents prints callers as JSON grouped by type prefix.
// Ids of one type are joined by "|", in type order then numeric id.
// Callers c11, c12, t17, and t81 become {"c":"11|12","t":"17|81"}.
// No callers becomes {}.
func FormatDependents(deps []Dependent) string {
	grouped := map[string][]string{}
	for _, d := range deps {
		prefix, id, ok := splitKey(d.Key)
		if !ok {
			continue
		}
		grouped[prefix] = append(grouped[prefix], strconv.Itoa(id))
	}
	out := make(map[string]string, len(grouped))
	for prefix, ids := range grouped {
		out[prefix] = strings.Join(ids, "|")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func lessKey(a, b string) bool {
	pa, ia, oka := splitKey(a)
	pb, ib, okb := splitKey(b)
	if oka && okb {
		if pa != pb {
			return pa < pb
		}
		if ia != ib {
			return ia < ib
		}
	}
	return a < b
}

func splitKey(key string) (string, int, bool) {
	if key == "" {
		return "", 0, false
	}
	i := 0
	for i < len(key) && key[i] >= 'a' && key[i] <= 'z' {
		i++
	}
	if i == 0 || i == len(key) {
		return "", 0, false
	}
	id, err := strconv.Atoi(key[i:])
	if err != nil {
		return "", 0, false
	}
	return key[:i], id, true
}
