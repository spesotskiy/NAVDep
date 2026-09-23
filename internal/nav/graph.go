package nav

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// keyPattern is a lowercase type prefix plus a numeric id, as in c12 or t81.
var keyPattern = regexp.MustCompile(`^[cdfmprqtx][0-9]+$`)

// Summary is the short result of a build.
type Summary struct {
	Objects    int
	Unresolved int
	Links      int
}

// Dependent is one object that references the requested key.
type Dependent struct {
	Key  string
	Name string
}

// Graph is the reverse map from callee key to the objects that reference it.
// Build currently validates the folder and returns an empty map. Catalog and
// reference scanning fill callers and names.
type Graph struct {
	callers map[string]map[string]struct{}
	names   map[string]string
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

// Build checks that folder is a directory and returns an empty reverse map.
// A later pass replaces the map contents with scanned references.
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
	g := &Graph{
		callers: map[string]map[string]struct{}{},
		names:   map[string]string{},
	}
	return g, Summary{}, nil
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
