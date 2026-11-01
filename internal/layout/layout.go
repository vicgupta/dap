// Package layout is the card layout stored for the local UI.
// Home and the decision inbox each remember a view, a density, and
// the pin, collapse, order, and tile-size choices for that view.
package layout

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// Grid is the default Home arrangement.
	Grid = "grid"
	// List is a single column. It is the default inbox arrangement.
	List = "list"
	// Board groups cards into columns.
	Board = "board"
	// Split is the inbox list plus a detail pane.
	Split = "split"

	// Comfortable is the default spacing.
	Comfortable = "comfortable"
	// Compact tightens padding and type.
	Compact = "compact"

	// Small, Medium, and Large are dashboard tile spans.
	Small  = "small"
	Medium = "medium"
	Large  = "large"
)

// View is one screen's layout.
type View struct {
	View      string            `json:"view"`
	Density   string            `json:"density"`
	Order     []string          `json:"order,omitempty"`
	Pinned    []string          `json:"pinned,omitempty"`
	Collapsed []string          `json:"collapsed,omitempty"`
	Sizes     map[string]string `json:"sizes,omitempty"`
}

// Settings is the document stored in config.json under "layout".
type Settings struct {
	Home  View `json:"home"`
	Inbox View `json:"inbox"`
}

// Default is grid + comfortable on Home, list + comfortable in the inbox.
func Default() Settings {
	return Settings{
		Home:  View{View: Grid, Density: Comfortable},
		Inbox: View{View: List, Density: Comfortable},
	}
}

// PluginDensity is the density plugin pages follow. Tiles live on Home,
// so plugins use the Home density.
func (s Settings) PluginDensity() string {
	if s.Home.Density == Compact {
		return Compact
	}
	return Comfortable
}

// Normalize checks a settings document. Empty fields become the defaults.
// An unknown view, density, or tile size is an error.
func Normalize(in Settings) (Settings, error) {
	return normalize(in, true)
}

// Stored coerces a document read from disk. Unknown values become defaults
// so a hand-edited config cannot blank the UI.
func Stored(in Settings) Settings {
	out, _ := normalize(in, false)
	return out
}

func normalize(in Settings, strict bool) (Settings, error) {
	home, err := normalizeView(in.Home, map[string]bool{Grid: true, List: true, Board: true}, Grid, strict)
	if err != nil {
		return Settings{}, fmt.Errorf("home: %w", err)
	}
	inbox, err := normalizeView(in.Inbox, map[string]bool{List: true, Grid: true, Board: true, Split: true}, List, strict)
	if err != nil {
		return Settings{}, fmt.Errorf("inbox: %w", err)
	}
	return Settings{Home: home, Inbox: inbox}, nil
}

func normalizeView(v View, allowed map[string]bool, fallback string, strict bool) (View, error) {
	view := strings.ToLower(strings.TrimSpace(v.View))
	if view == "" {
		view = fallback
	}
	if !allowed[view] {
		if strict {
			return View{}, fmt.Errorf("view %q is not one of %s", v.View, names(allowed))
		}
		view = fallback
	}
	density := strings.ToLower(strings.TrimSpace(v.Density))
	if density == "" {
		density = Comfortable
	}
	if density != Comfortable && density != Compact {
		if strict {
			return View{}, fmt.Errorf("density must be comfortable or compact")
		}
		density = Comfortable
	}
	sizes, err := normalizeSizes(v.Sizes, strict)
	if err != nil {
		return View{}, err
	}
	return View{
		View:      view,
		Density:   density,
		Order:     cleanIDs(v.Order),
		Pinned:    cleanIDs(v.Pinned),
		Collapsed: cleanIDs(v.Collapsed),
		Sizes:     sizes,
	}, nil
}

func names(allowed map[string]bool) string {
	out := make([]string, 0, len(allowed))
	for k := range allowed {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func cleanIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "\r\n\t") || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == 200 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSizes(in map[string]string, strict bool) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]string{}
	for _, k := range keys {
		if len(out) == 200 {
			break
		}
		id := strings.TrimSpace(k)
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "\r\n\t") {
			if strict {
				return nil, fmt.Errorf("tile size key %q is invalid", k)
			}
			continue
		}
		size, err := NormalizeSize(in[k])
		if err != nil {
			if strict {
				return nil, fmt.Errorf("tile size for %s: %w", id, err)
			}
			continue
		}
		out[id] = size
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// NormalizeSize accepts small, medium, or large.
func NormalizeSize(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case Small, Medium, Large:
		return s, nil
	default:
		return "", fmt.Errorf("must be small, medium, or large")
	}
}

// Rank orders small < medium < large. Unknown is 0.
func Rank(size string) int {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case Small:
		return 1
	case Medium:
		return 2
	case Large:
		return 3
	default:
		return 0
	}
}

// ResolveSize picks the span to render. user wins when it is at least min.
// Otherwise preferred is used, still clamped to min. Empty values fall back
// to medium and a floor of small.
func ResolveSize(user, preferred, min string) string {
	floor := Small
	if Rank(min) > 0 {
		floor = strings.ToLower(strings.TrimSpace(min))
	}
	size := strings.ToLower(strings.TrimSpace(user))
	if Rank(size) == 0 {
		size = strings.ToLower(strings.TrimSpace(preferred))
	}
	if Rank(size) == 0 {
		size = Medium
	}
	if Rank(size) < Rank(floor) {
		return floor
	}
	return size
}

// ChosenDensity reads a query value, then a fallback. Invalid text uses the fallback.
func ChosenDensity(query, fallback string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == Comfortable || q == Compact {
		return q
	}
	f := strings.ToLower(strings.TrimSpace(fallback))
	if f == Compact {
		return Compact
	}
	return Comfortable
}
