package search

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Lang int

const (
	LangNone Lang = iota
	LangTamil
	LangTelugu
	LangMalayalam
	LangHindi
	LangKannada
	LangEnglish
	LangMulti
)

var langInfo = []struct {
	label string
	name  string
	code  string
}{
	LangNone:      {"", "", ""},
	LangTamil:     {"TAM", "tamil", "ta"},
	LangTelugu:    {"TEL", "telugu", "te"},
	LangMalayalam: {"MAL", "malayalam", "ml"},
	LangHindi:     {"HIN", "hindi", "hi"},
	LangKannada:   {"KAN", "kannada", "kn"},
	LangEnglish:   {"ENG", "english", "en"},
	LangMulti:     {"MULTI", "multi", ""},
}

func (l Lang) Label() string {
	if l < 0 || int(l) >= len(langInfo) {
		return ""
	}
	return langInfo[l].label
}

func (l Lang) Name() string {
	if l < 0 || int(l) >= len(langInfo) {
		return ""
	}
	return langInfo[l].name
}

func (l Lang) Code() string {
	if l < 0 || int(l) >= len(langInfo) {
		return ""
	}
	return langInfo[l].code
}

var langWords = map[string]Lang{
	"tamil":     LangTamil,
	"tam":       LangTamil,
	"telugu":    LangTelugu,
	"tel":       LangTelugu,
	"malayalam": LangMalayalam,
	"mal":       LangMalayalam,
	"hindi":     LangHindi,
	"hin":       LangHindi,
	"kannada":   LangKannada,
	"kan":       LangKannada,
	"english":   LangEnglish,
	"eng":       LangEnglish,
	"multi":     LangMulti,
	"dual":      LangMulti,
}

var epMarkerRe = regexp.MustCompile(`^(?:s\d{1,2}(?:e\d{1,3})?|\d{1,2}x\d{1,3})$`)

// DetectLang reads the language tag from a raw release name. It looks only
// at the tail after the year (the last whole-word occurrence of year) or,
// with no year, after the first episode marker (S01, S01E02, 1x02); with
// neither, or when raw, it reads the whole name.
func DetectLang(name string, year int, raw bool) Lang {
	words := splitWords(name)
	if !raw {
		words = langTail(words, year)
	}
	found := LangNone
	for _, w := range words {
		l, ok := langWords[w]
		if !ok {
			continue
		}
		if l == LangMulti {
			return LangMulti
		}
		if found != LangNone && found != l {
			return LangMulti
		}
		found = l
	}
	return found
}

func langTail(words []string, year int) []string {
	if year > 0 {
		y := strconv.Itoa(year)
		for i := len(words) - 1; i >= 0; i-- {
			if words[i] == y {
				return words[i+1:]
			}
		}
	}
	for i, w := range words {
		if epMarkerRe.MatchString(w) {
			return words[i+1:]
		}
	}
	return words
}

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
