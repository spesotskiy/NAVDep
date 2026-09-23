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
	// UnresolvedNames are name references that did not become an object key.
	// A numeric reference is never listed here, even when its file is absent.
	UnresolvedNames []UnresolvedName
}

// UnresolvedName is one name that build could not resolve.
// Reason is "missing" when no object of that type has the name, or "ambiguous"
// when more than one object of that type uses it. Callers are the objects that
// mentioned the name, each once.
type UnresolvedName struct {
	Prefix  string   `json:"prefix"`
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Reason  string   `json:"reason"`
	Callers []string `json:"callers"`
}

// Dependent is one object that references the requested key.
type Dependent struct {
	Key  string
	Name string
}

// UnusedObject is an accepted object that no other file references.
type UnusedObject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// UnusedGroup is one object type in unused.json.
// Names are "id - name", sorted by id.
type UnusedGroup struct {
	Count int      `json:"Count"`
	Names []string `json:"Names"`
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
	links, names := g.indexRefs()
	return g, Summary{
		Objects:         len(cat.Objects),
		Unresolved:      unresolvedCount(names),
		Links:           links,
		Warnings:        warnings,
		UnresolvedNames: names,
	}, nil
}

// indexRefs scans each cataloged object's source and records its compile-time references.
func (g *Graph) indexRefs() (links int, names []UnresolvedName) {
	keys := make([]string, 0, len(g.catalog.Objects))
	for key := range g.catalog.Objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ambiguous := ambiguousNames(g.catalog)
	hits := map[string]*unresolvedHit{}
	for _, key := range keys {
		obj := g.catalog.Objects[key]
		text := obj.source
		obj.source = ""
		g.catalog.Objects[key] = obj
		for _, ref := range ExtractRefs(text) {
			callee, ok := resolveRef(g.catalog, ref)
			if !ok {
				recordUnresolved(hits, ambiguous, ref, obj.Key)
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
	return links, unresolvedList(hits)
}

type unresolvedHit struct {
	prefix  string
	name    string
	reason  string
	callers map[string]struct{}
}

func ambiguousNames(cat *Catalog) map[string]struct{} {
	counts := map[string]int{}
	for _, obj := range cat.Objects {
		counts[obj.Prefix+"\x00"+strings.ToLower(obj.Name)]++
	}
	ambiguous := map[string]struct{}{}
	for id, n := range counts {
		if n > 1 {
			ambiguous[id] = struct{}{}
		}
	}
	return ambiguous
}

func recordUnresolved(hits map[string]*unresolvedHit, ambiguous map[string]struct{}, ref Ref, caller string) {
	if ref.Numeric {
		return
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" || ref.Prefix == "" {
		return
	}
	id := ref.Prefix + "\x00" + strings.ToLower(name)
	hit := hits[id]
	if hit == nil {
		reason := "missing"
		if _, ok := ambiguous[id]; ok {
			reason = "ambiguous"
		}
		hit = &unresolvedHit{
			prefix:  ref.Prefix,
			name:    name,
			reason:  reason,
			callers: map[string]struct{}{},
		}
		hits[id] = hit
	}
	hit.callers[caller] = struct{}{}
}

func unresolvedList(hits map[string]*unresolvedHit) []UnresolvedName {
	out := make([]UnresolvedName, 0, len(hits))
	for _, hit := range hits {
		callers := make([]string, 0, len(hit.callers))
		for caller := range hit.callers {
			callers = append(callers, caller)
		}
		sort.Slice(callers, func(i, j int) bool {
			return lessKey(callers[i], callers[j])
		})
		out = append(out, UnresolvedName{
			Prefix:  hit.prefix,
			Name:    hit.name,
			Count:   len(callers),
			Reason:  hit.reason,
			Callers: callers,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Prefix != out[j].Prefix {
			return out[i].Prefix < out[j].Prefix
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func unresolvedCount(names []UnresolvedName) int {
	n := 0
	for _, name := range names {
		n += name.Count
	}
	return n
}

// Unused returns accepted objects that no other file references, sorted by type
// prefix then numeric id. A reference by number or by a unique name counts.
// The object's own file does not. An ambiguous name does not mark either object used.
func (g *Graph) Unused() []UnusedObject {
	if g == nil || g.catalog == nil {
		return []UnusedObject{}
	}
	keys := make([]string, 0, len(g.catalog.Objects))
	for key := range g.catalog.Objects {
		if len(g.callers[key]) == 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return lessKey(keys[i], keys[j])
	})
	out := make([]UnusedObject, 0, len(keys))
	for _, key := range keys {
		obj := g.catalog.Objects[key]
		out = append(out, UnusedObject{
			Key:  obj.Key,
			Name: obj.Name,
		})
	}
	return out
}

// WriteUnusedLog writes objects to unused.json inside dir and returns that path.
// folder is the scanned folder recorded in the file. Each build replaces the file.
// Objects are grouped by type. A group name is the plural type, such as "codeunits".
// Each name is "id - name", sorted by id. Types with no unused objects are omitted.
func WriteUnusedLog(dir, folder string, objects []UnusedObject) (string, error) {
	doc := unusedDocument{
		Folder: folder,
		Unused: len(objects),
	}
	doc.fill(objects)
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	path := filepath.Join(dir, "unused.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// unusedDocument is unused.json. Group fields follow the object type table.
type unusedDocument struct {
	Folder     string       `json:"folder"`
	Unused     int          `json:"unused"`
	Codeunits  *UnusedGroup `json:"codeunits,omitempty"`
	Dataports  *UnusedGroup `json:"dataports,omitempty"`
	Forms      *UnusedGroup `json:"forms,omitempty"`
	MenuSuites *UnusedGroup `json:"menusuites,omitempty"`
	Pages      *UnusedGroup `json:"pages,omitempty"`
	Queries    *UnusedGroup `json:"queries,omitempty"`
	Reports    *UnusedGroup `json:"reports,omitempty"`
	Tables     *UnusedGroup `json:"tables,omitempty"`
	XMLPorts   *UnusedGroup `json:"xmlports,omitempty"`
}

func (d *unusedDocument) fill(objects []UnusedObject) {
	byPrefix := map[string][]unusedLabel{}
	for _, obj := range objects {
		prefix, id, ok := splitKey(obj.Key)
		if !ok {
			continue
		}
		byPrefix[prefix] = append(byPrefix[prefix], unusedLabel{
			id:    id,
			label: strconv.Itoa(id) + " - " + obj.Name,
		})
	}
	for _, typ := range objectTypes {
		labels := byPrefix[typ.Prefix]
		if len(labels) == 0 {
			continue
		}
		sort.Slice(labels, func(i, j int) bool {
			return labels[i].id < labels[j].id
		})
		names := make([]string, len(labels))
		for i, label := range labels {
			names[i] = label.label
		}
		d.setGroup(typ.Prefix, &UnusedGroup{Count: len(names), Names: names})
	}
}

type unusedLabel struct {
	id    int
	label string
}

func (d *unusedDocument) setGroup(prefix string, group *UnusedGroup) {
	switch prefix {
	case "c":
		d.Codeunits = group
	case "d":
		d.Dataports = group
	case "f":
		d.Forms = group
	case "m":
		d.MenuSuites = group
	case "p":
		d.Pages = group
	case "q":
		d.Queries = group
	case "r":
		d.Reports = group
	case "t":
		d.Tables = group
	case "x":
		d.XMLPorts = group
	}
}

// WriteUnresolvedLog writes names to unresolved.json inside dir and returns that path.
// folder is the scanned folder recorded in the file. Each build replaces the file.
func WriteUnresolvedLog(dir, folder string, sum Summary) (string, error) {
	names := sum.UnresolvedNames
	if names == nil {
		names = []UnresolvedName{}
	}
	doc := struct {
		Folder     string           `json:"folder"`
		Unresolved int              `json:"unresolved"`
		Names      []UnresolvedName `json:"names"`
	}{
		Folder:     folder,
		Unresolved: sum.Unresolved,
		Names:      names,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	path := filepath.Join(dir, "unresolved.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
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
