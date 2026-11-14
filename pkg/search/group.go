package search

import (
	"fmt"
	"maps"
	"slices"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

type Group int

const (
	Group4K Group = iota
	Group1080p
	Group720p
	Group576p
	Group480p
	GroupUnknown
)

var groupInfo = []struct {
	key    string
	header string
}{
	Group4K:      {"2160p", "4K"},
	Group1080p:   {"1080p", "1080p"},
	Group720p:    {"720p", "720p"},
	Group576p:    {"576p", "576p"},
	Group480p:    {"480p", "480p"},
	GroupUnknown: {"unknown", "Unknown"},
}

func (g Group) Key() string {
	if g < 0 || int(g) >= len(groupInfo) {
		return groupInfo[GroupUnknown].key
	}
	return groupInfo[g].key
}

func (g Group) Header() string {
	if g < 0 || int(g) >= len(groupInfo) {
		return groupInfo[GroupUnknown].header
	}
	return groupInfo[g].header
}

// GroupOf maps ParsedMedia.Resolution to its display group.
func GroupOf(p matcher.ParsedMedia) Group {
	switch p.Resolution {
	case "2160p", "4k", "uhd":
		return Group4K
	case "1080p", "1080i":
		return Group1080p
	case "720p":
		return Group720p
	case "576p":
		return Group576p
	case "480p":
		return Group480p
	default:
		return GroupUnknown
	}
}

const MaxCap = 50

// LateCap is the cap of the Late results section, and m's step there.
const LateCap = 10

// DefaultCaps is used when the config has no [search.resolutions] table.
func DefaultCaps() map[Group]int {
	return map[Group]int{
		Group4K:      10,
		Group1080p:   10,
		Group720p:    5,
		GroupUnknown: 5,
	}
}

// Caps validates the config table. A nil table gives DefaultCaps. A non-nil
// table is the whole list: groups missing from it, or with cap 0, are off.
func Caps(table map[string]int) (map[Group]int, error) {
	if table == nil {
		return DefaultCaps(), nil
	}
	byKey := make(map[string]Group, len(groupInfo))
	for i, info := range groupInfo {
		byKey[info.key] = Group(i)
	}
	out := make(map[Group]int, len(table))
	for _, k := range slices.Sorted(maps.Keys(table)) {
		g, ok := byKey[k]
		if !ok {
			return nil, fmt.Errorf("search.resolutions: %q is not a resolution group", k)
		}
		v := table[k]
		if v < 0 || v > MaxCap {
			return nil, fmt.Errorf("search.resolutions: %q must be between 0 and %d", k, MaxCap)
		}
		if v > 0 {
			out[g] = v
		}
	}
	return out, nil
}
