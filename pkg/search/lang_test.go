package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func TestDetectLang(t *testing.T) {
	tests := []struct {
		name string
		year int
		raw  bool
		want search.Lang
	}{
		{"Sample.Film.2014.1080p.WEB-DL.Tamil.x264-GRP", 2014, false, search.LangTamil},
		{"Sample.Film.2014.1080p.TAM.TEL.HIN.x264-GRP", 2014, false, search.LangMulti},
		{"Sample.Film.2014.Dual.Audio.1080p-GRP", 2014, false, search.LangMulti},
		{"Sample.Film.2014.1080p.Malayalam.Hindi-GRP", 2014, false, search.LangMulti},
		{"Sample.Film.2014.1080p.BluRay.x264-GRP", 2014, false, search.LangNone},
		{"Sample.Film.2014.1080p.ENG.x265-GRP", 2014, false, search.LangEnglish},
		{"Tamil.Sample.2014.1080p-GRP", 2014, false, search.LangNone},
		{"Sample.Film.2014.Hotel.1080p-GRP", 2014, false, search.LangNone},
		{"2012.Sample.2009.1080p.Kannada-GRP", 2009, false, search.LangKannada},
		{"Sample.Show.S01E02.1080p.Telugu-GRP", 0, false, search.LangTelugu},
		{"Hindi.Show.S01E02.720p-GRP", 0, false, search.LangNone},
		{"Tamil.Sample.1080p-GRP", 0, true, search.LangTamil},
		{"Sample.Film.1080p.Multi-GRP", 0, false, search.LangMulti},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, search.DetectLang(tt.name, tt.year, tt.raw))
		})
	}
}

func TestLangStrings(t *testing.T) {
	tests := []struct {
		lang  search.Lang
		label string
		name  string
		code  string
	}{
		{search.LangNone, "", "", ""},
		{search.LangTamil, "TAM", "tamil", "ta"},
		{search.LangTelugu, "TEL", "telugu", "te"},
		{search.LangMalayalam, "MAL", "malayalam", "ml"},
		{search.LangHindi, "HIN", "hindi", "hi"},
		{search.LangKannada, "KAN", "kannada", "kn"},
		{search.LangEnglish, "ENG", "english", "en"},
		{search.LangMulti, "MULTI", "multi", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.label, tt.lang.Label())
		assert.Equal(t, tt.name, tt.lang.Name())
		assert.Equal(t, tt.code, tt.lang.Code())
	}
}

func TestPrepare_SetsLangAndCountsFiltered(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "Sample Film", Year: 2014, Language: "ta"}
	rows := []search.Result{
		{Title: "Sample.Film.2014.1080p.Tamil-GRP"},
		{Title: "Unrelated.Film.2014.1080p.Hindi-GRP"},
		{Title: "Sample.Film.2014.720p.Multi-GRP"},
	}

	kept, filtered := search.Prepare(search.Request{}, w, rows)
	require.Len(t, kept, 2)
	assert.Equal(t, 1, filtered)
	assert.Equal(t, search.LangTamil, kept[0].Lang)
	assert.Equal(t, search.LangMulti, kept[1].Lang)

	_, byIDFiltered := search.Prepare(search.Request{ByID: true}, w, rows)
	assert.Equal(t, 0, byIDFiltered)

	_, rawFiltered := search.Prepare(search.Request{Raw: true}, w, rows)
	assert.Equal(t, 0, rawFiltered)
}
