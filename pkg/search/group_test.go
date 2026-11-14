package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

func TestGroupOf(t *testing.T) {
	tests := []struct {
		res  string
		want Group
	}{
		{"2160p", Group4K},
		{"4k", Group4K},
		{"uhd", Group4K},
		{"1080p", Group1080p},
		{"1080i", Group1080p},
		{"720p", Group720p},
		{"576p", Group576p},
		{"480p", Group480p},
		{"", GroupUnknown},
		{"360p", GroupUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.res, func(t *testing.T) {
			assert.Equal(t, tt.want, GroupOf(matcher.ParsedMedia{Resolution: tt.res}))
		})
	}
}

func TestGroupKeyAndHeader(t *testing.T) {
	tests := []struct {
		group  Group
		key    string
		header string
	}{
		{Group4K, "2160p", "4K"},
		{Group1080p, "1080p", "1080p"},
		{Group720p, "720p", "720p"},
		{Group576p, "576p", "576p"},
		{Group480p, "480p", "480p"},
		{GroupUnknown, "unknown", "Unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.key, tt.group.Key())
		assert.Equal(t, tt.header, tt.group.Header())
	}
}

func TestCaps(t *testing.T) {
	wantDefault := map[Group]int{
		Group4K:      10,
		Group1080p:   10,
		Group720p:    5,
		GroupUnknown: 5,
	}
	assert.Equal(t, wantDefault, DefaultCaps())

	got, err := Caps(nil)
	require.NoError(t, err)
	assert.Equal(t, wantDefault, got)

	got, err = Caps(map[string]int{"1080p": 4})
	require.NoError(t, err)
	assert.Equal(t, map[Group]int{Group1080p: 4}, got)

	got, err = Caps(map[string]int{"480p": 5, "unknown": 0})
	require.NoError(t, err)
	assert.Equal(t, map[Group]int{Group480p: 5}, got)
	assert.Zero(t, got[GroupUnknown])

	got, err = Caps(map[string]int{"720p": 50})
	require.NoError(t, err)
	assert.Equal(t, map[Group]int{Group720p: 50}, got)
}

func TestCapsErrors(t *testing.T) {
	tests := []struct {
		name  string
		table map[string]int
		key   string
	}{
		{"unknown key", map[string]int{"1440p": 5}, "1440p"},
		{"negative cap", map[string]int{"720p": -1}, "720p"},
		{"over max cap", map[string]int{"720p": 51}, "720p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Caps(tt.table)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}
