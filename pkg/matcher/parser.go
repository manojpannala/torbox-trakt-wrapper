package matcher

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// knownExtensions are checked as a plain suffix instead of a regex, which measurably cut per-call overhead.
var knownExtensions = []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".m2ts", ".ts", ".iso"}

var (
	groupPrefixRegex      = regexp.MustCompile(`^\[([a-zA-Z0-9_\-\.\s]+)\]\s*`)
	groupSuffixRegex      = regexp.MustCompile(`-([a-zA-Z0-9_]+)(?:\.[a-zA-Z0-9]+)?$`)
	seasonEpRegex         = regexp.MustCompile(`(?i)[sS](\d{1,2})[eE](\d{1,3})(?:(?:-(?:[eE])?|[eE])(\d{1,3}))?`)
	altSeasonEpRegex      = regexp.MustCompile(`(?i)(?:^|[\s._\-\[])(\d{1,2})x(\d{1,3})(?:[\s._\-\]]|$)`)
	explicitSeasonEpRegex = regexp.MustCompile(`(?i)Season[.\s_-]*(\d{1,2})[.\s_-]*(?:Episode|Ep|E)[.\s_-]*(\d{1,3})`)
	animeEpRegex          = regexp.MustCompile(`(?i)(?:^|[\s._\-])(?:e|ep|episode|#)?\s*(\d{1,3})(?:v\d)?(?:[\s._\-\]]|$)`)
	yearRegex             = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)
	resolutionRegex       = regexp.MustCompile(`(?i)\b(2160p|4k|uhd|1080p|1080i|720p|576p|480p)\b`)
	sourceRegex           = regexp.MustCompile(`(?i)\b(remux|blu-?ray|bluray|bd-?rip|brrip|web-?dl|web-?rip|webrip|webdl|hdtv|dvd-?rip|dvd)\b`)
	codecRegex            = regexp.MustCompile(`(?i)\b(x265|x264|h\.?265|h\.?264|hevc|avc|av1|xvid|divx|10bit|8bit)\b`)
	audioRegex            = regexp.MustCompile(`(?i)\b(truehd(?:\.?atmos)?|atmos|dts-?hd(?:\.?ma)?|dts-?ma|dts|ddp\s*5[._]1|dd\+?\s*5[._]1|dd\s*5[._]1|eac3|ac3|flac|aac(?:\s*5[._]1|\s*2[._]0)?|7[._]1|5[._]1|2[._]0)\b`)
	hdrRegex              = regexp.MustCompile(`(?i)\b(hdr10\+|hdr10|hdr|dv|dovi|dolby\s*vision|hlg)\b`)
	sceneTagsRegex        = regexp.MustCompile(`(?i)\b(proper|repack|extended|unrated|directors\.cut|director's\.cut|imax|multi|dual|complete|internal|subbed|dubbed|amzn|nf|dsnp|hmax|atvp|apple|criterion)\b`)
	bracketedTagRegex     = regexp.MustCompile(`\[[a-zA-Z0-9_\-\.\s]+\]`)
	sitePrefixRegex       = regexp.MustCompile(`(?i)^\s*www\.[a-z0-9][a-z0-9\-]*(?:\.[a-z0-9\-]+)+\s*(?:[-\x{2013}|:]\s*)?`)
	fullWidthBannerRegex  = regexp.MustCompile(`\x{3010}[^\x{3011}]*\x{3011}`)
	anyBracketedRegex     = regexp.MustCompile(`\[[^\]]*\]`)
	latinRunRegex         = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9'&:,.!?\- ]{2,}`)
	trailingYearRegex     = regexp.MustCompile(`[\s._\-]\(?(19\d\d|20\d\d)\)?$`)

	// *CS regexes drop the (?i) of their counterparts, matched against an ASCII-lowered copy instead: (?i) pays for Unicode case-fold tables a pre-lowered match doesn't need.
	resolutionRegexCS = regexp.MustCompile(`\b(2160p|4k|uhd|1080p|1080i|720p|576p|480p)\b`)
	sourceRegexCS     = regexp.MustCompile(`\b(remux|blu-?ray|bluray|bd-?rip|brrip|web-?dl|web-?rip|webrip|webdl|hdtv|dvd-?rip|dvd)\b`)
	codecRegexCS      = regexp.MustCompile(`\b(x265|x264|h\.?265|h\.?264|hevc|avc|av1|xvid|divx|10bit|8bit)\b`)
	audioRegexCS      = regexp.MustCompile(`\b(truehd(?:\.?atmos)?|atmos|dts-?hd(?:\.?ma)?|dts-?ma|dts|ddp\s*5[._]1|dd\+?\s*5[._]1|dd\s*5[._]1|eac3|ac3|flac|aac(?:\s*5[._]1|\s*2[._]0)?|7[._]1|5[._]1|2[._]0)\b`)
	hdrRegexCS        = regexp.MustCompile(`\b(hdr10\+|hdr10|hdr|dv|dovi|dolby\s*vision|hlg)\b`)
)

// tagHints tells stripTags which categories ParseMedia found nowhere in the full name, so a substring can't have them either.
type tagHints struct {
	resolution, source, codec, audio, hdr bool
}

func ParseMedia(rawName string) ParsedMedia {
	parsed := ParsedMedia{
		OriginalName: rawName,
		Type:         MediaTypeUnknown,
	}

	filename := filepath.Base(rawName)
	clean := stripKnownExtension(filename)
	clean = stripSiteMarkers(clean)

	if match := groupPrefixRegex.FindStringSubmatch(clean); len(match) > 1 {
		groupCandidate := strings.TrimSpace(match[1])
		if !resolutionRegex.MatchString(groupCandidate) && !codecRegex.MatchString(groupCandidate) {
			parsed.ReleaseGroup = groupCandidate
			clean = groupPrefixRegex.ReplaceAllString(clean, "")
		}
	}
	if parsed.ReleaseGroup == "" {
		if match := groupSuffixRegex.FindStringSubmatch(clean); len(match) > 1 {
			groupCandidate := match[1]
			candLower := strings.ToLower(groupCandidate)
			if !sourceRegex.MatchString(groupCandidate) &&
				!codecRegex.MatchString(groupCandidate) &&
				!audioRegex.MatchString(groupCandidate) &&
				!hdrRegex.MatchString(groupCandidate) &&
				!resolutionRegex.MatchString(groupCandidate) &&
				candLower != "dl" && candLower != "rip" && candLower != "web" {
				parsed.ReleaseGroup = groupCandidate
				clean = groupSuffixRegex.ReplaceAllString(clean, "")
			}
		}
	}

	normalizedSpaced := normalizeDelimiters(clean)

	// Only '_' changes a tag's word-boundary outcome vs. clean; '.' doesn't.
	hasUnderscore := strings.IndexByte(clean, '_') >= 0

	cleanASCIILower := toLowerASCII(clean)
	normalizedASCIILower := toLowerASCII(normalizedSpaced)

	if mayContainAny(cleanASCIILower, "2160p", "4k", "uhd", "1080p", "1080i", "720p", "576p", "480p") {
		if match := resolutionRegexCS.FindString(normalizedASCIILower); match != "" {
			parsed.Resolution = normalizeTag(match)
		}
	}
	if mayContainAny(cleanASCIILower, "remux", "bluray", "blu-ray", "bdrip", "bd-rip", "brrip", "webdl", "web-dl", "webrip", "web-rip", "hdtv", "dvd") {
		if match := sourceRegexCS.FindString(cleanASCIILower); match != "" {
			parsed.Source = normalizeTag(match)
		} else if hasUnderscore {
			if match := sourceRegexCS.FindString(normalizedASCIILower); match != "" {
				parsed.Source = normalizeTag(match)
			}
		}
	}
	if mayContainAny(cleanASCIILower, "x265", "x264", "h265", "h264", "h.265", "h.264", "hevc", "avc", "av1", "xvid", "divx", "10bit", "8bit") {
		if match := codecRegexCS.FindString(cleanASCIILower); match != "" {
			parsed.Codec = normalizeTag(match)
		} else if hasUnderscore {
			if match := codecRegexCS.FindString(normalizedASCIILower); match != "" {
				parsed.Codec = normalizeTag(match)
			}
		}
	}

	if strings.Contains(cleanASCIILower, "truehd") && strings.Contains(cleanASCIILower, "atmos") {
		parsed.Audio = "truehdatmos"
	} else if mayContainAny(cleanASCIILower, "truehd", "atmos", "dts", "dd", "ac3", "flac", "aac") ||
		hasDigitPair(clean, '7', '1') || hasDigitPair(clean, '5', '1') || hasDigitPair(clean, '2', '0') {
		if match := audioRegexCS.FindString(cleanASCIILower); match != "" {
			parsed.Audio = normalizeTag(match)
		} else if hasUnderscore {
			if match := audioRegexCS.FindString(normalizedASCIILower); match != "" {
				parsed.Audio = normalizeTag(match)
			}
		}
	}

	if mayContainAny(cleanASCIILower, "hdr", "dv", "dovi", "dolby", "hlg") {
		if match := hdrRegexCS.FindString(cleanASCIILower); match != "" {
			parsed.HDR = normalizeTag(match)
		} else if hasUnderscore {
			if match := hdrRegexCS.FindString(normalizedASCIILower); match != "" {
				parsed.HDR = normalizeTag(match)
			}
		}
	}

	hints := tagHints{
		resolution: parsed.Resolution != "",
		source:     parsed.Source != "",
		codec:      parsed.Codec != "",
		audio:      parsed.Audio != "",
		hdr:        parsed.HDR != "",
	}

	if loc := seasonEpRegex.FindStringSubmatchIndex(clean); len(loc) >= 6 {
		seasonStr := clean[loc[2]:loc[3]]
		epStr := clean[loc[4]:loc[5]]
		parsed.Season, _ = strconv.Atoi(seasonStr)
		parsed.Episode, _ = strconv.Atoi(epStr)
		if len(loc) >= 8 && loc[6] >= 0 && loc[7] >= 0 {
			epEndStr := clean[loc[6]:loc[7]]
			parsed.EpisodeEnd, _ = strconv.Atoi(epEndStr)
		}
		parsed.Type = MediaTypeEpisode

		titlePart := clean[:loc[0]]
		parsed.CleanTitle = sanitizeTitle(titlePart, hints)
	} else if strings.Contains(cleanASCIILower, "season") {
		if loc := explicitSeasonEpRegex.FindStringSubmatchIndex(clean); len(loc) >= 6 {
			seasonStr := clean[loc[2]:loc[3]]
			epStr := clean[loc[4]:loc[5]]
			parsed.Season, _ = strconv.Atoi(seasonStr)
			parsed.Episode, _ = strconv.Atoi(epStr)
			parsed.Type = MediaTypeEpisode

			titlePart := clean[:loc[0]]
			parsed.CleanTitle = sanitizeTitle(titlePart, hints)
		}
	}
	if parsed.Type == MediaTypeUnknown {
		if loc := altSeasonEpRegex.FindStringSubmatchIndex(clean); len(loc) >= 6 {
			seasonStr := clean[loc[2]:loc[3]]
			epStr := clean[loc[4]:loc[5]]
			parsed.Season, _ = strconv.Atoi(seasonStr)
			parsed.Episode, _ = strconv.Atoi(epStr)
			parsed.Type = MediaTypeEpisode

			titlePart := clean[:loc[0]]
			parsed.CleanTitle = sanitizeTitle(titlePart, hints)
		}
	}

	if parsed.Type == MediaTypeUnknown {
		yearMatches := yearRegex.FindAllStringIndex(normalizedSpaced, -1)
		currentYear := time.Now().Year()

		var matchedYear int
		var yearIndex = -1

		for i := len(yearMatches) - 1; i >= 0; i-- {
			yIdx := yearMatches[i]
			yVal, _ := strconv.Atoi(normalizedSpaced[yIdx[0]:yIdx[1]])

			if yVal >= 1900 && yVal <= currentYear+2 {
				matchedYear = yVal
				yearIndex = yIdx[0]
				break
			}
		}

		if matchedYear > 0 && yearIndex >= 0 {
			parsed.Year = matchedYear
			parsed.Type = MediaTypeMovie

			titlePart := normalizedSpaced[:yearIndex]
			parsed.CleanTitle = sanitizeTitle(titlePart, hints)
		}
	}

	if parsed.Type == MediaTypeUnknown {
		trimmed := stripTags(normalizedSpaced, false, hints)

		if parts := strings.Split(trimmed, " - "); len(parts) >= 2 {
			epCandidate := strings.TrimSpace(parts[len(parts)-1])
			if match := animeEpRegex.FindStringSubmatch(epCandidate); len(match) > 1 {
				if epNum, err := strconv.Atoi(match[1]); err == nil && epNum > 0 {
					parsed.Type = MediaTypeEpisode
					parsed.Season = 1
					parsed.Episode = epNum
					titlePart := strings.Join(parts[:len(parts)-1], " - ")
					parsed.CleanTitle = sanitizeTitle(titlePart, hints)
				}
			}
		}
	}

	if parsed.Type == MediaTypeEpisode && parsed.Year == 0 {
		if title, year := splitTrailingYear(parsed.CleanTitle); year > 0 {
			parsed.CleanTitle = title
			parsed.Year = year
		}
	}

	if parsed.CleanTitle == "" {
		parsed.CleanTitle = sanitizeTitle(normalizedSpaced, hints)
		if parsed.Type == MediaTypeUnknown {
			parsed.Type = MediaTypeMovie
		}
	}

	return parsed
}

// stripSiteMarkers removes the indexer decoration some uploaders prepend to a
// filename: a leading host name, a full-width bracketed banner, and bracketed
// tags written in a non-Latin script. The input is returned untouched when
// stripping would leave nothing behind.
func stripSiteMarkers(s string) string {
	out := s
	if hasSitePrefixMarker(out) {
		out = sitePrefixRegex.ReplaceAllString(out, "")
	}
	if strings.ContainsRune(out, fullWidthBannerOpen) {
		out = fullWidthBannerRegex.ReplaceAllString(out, " ")
	}
	if strings.IndexByte(out, '[') >= 0 {
		out = anyBracketedRegex.ReplaceAllStringFunc(out, func(tag string) string {
			if hasNonLatinScript(tag) {
				return " "
			}
			return tag
		})
	}

	out = strings.TrimSpace(out)
	if out == "" {
		return s
	}
	return out
}

func stripKnownExtension(s string) string {
	for _, ext := range knownExtensions {
		if hasSuffixFoldASCII(s, ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

// hasSuffixFoldASCII: suffix must already be lowercase.
func hasSuffixFoldASCII(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	off := len(s) - len(suffix)
	for i := 0; i < len(suffix); i++ {
		c := s[off+i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != suffix[i] {
			return false
		}
	}
	return true
}

const fullWidthBannerOpen = '【'

// hasSitePrefixMarker is a superset check for sitePrefixRegex's leading `^\s*www\.`.
func hasSitePrefixMarker(s string) bool {
	s = strings.TrimLeft(s, " \t\n\r\f\v")
	return len(s) >= 4 &&
		s[0]|0x20 == 'w' && s[1]|0x20 == 'w' && s[2]|0x20 == 'w' && s[3] == '.'
}

// toLowerASCII lowercases only ASCII letters, so it can't corrupt a multi-byte UTF-8 rune.
func toLowerASCII(s string) string {
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// mayContainAny: a regex needing one of words can't match if none are in asciiLower.
func mayContainAny(asciiLower string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(asciiLower, w) {
			return true
		}
	}
	return false
}

// hasDigitPair matches audioRegex's bare channel-count form, e.g. 7.1/7_1.
func hasDigitPair(s string, a, b byte) bool {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == a && (s[i+1] == '.' || s[i+1] == '_') && s[i+2] == b {
			return true
		}
	}
	return false
}

// splitTrailingYear pulls a release year off the end of a title, where it is
// metadata rather than part of the name. Trakt stores the year separately, so
// leaving it in the title costs a match.
func splitTrailingYear(title string) (string, int) {
	match := trailingYearRegex.FindStringSubmatch(title)
	if match == nil {
		return title, 0
	}

	year, err := strconv.Atoi(match[1])
	if err != nil || year < 1900 || year > time.Now().Year()+2 {
		return title, 0
	}

	remainder := strings.TrimSpace(strings.TrimSuffix(title, match[0]))
	if remainder == "" {
		return title, 0
	}
	return remainder, year
}

// preferLatinScript keeps the longest Latin run of a title that carries both an
// original-script and a Latin name, a common dual-title convention. Titles with
// no Latin run are left alone, so a single-script name survives intact.
func preferLatinScript(title string) string {
	if !hasNonLatinScript(title) {
		return title
	}

	longest := ""
	for _, run := range latinRunRegex.FindAllString(title, -1) {
		run = strings.TrimSpace(run)
		if len(run) > len(longest) {
			longest = run
		}
	}
	if longest == "" {
		return title
	}
	return longest
}

func hasNonLatinScript(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func normalizeDelimiters(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == '.' || b == '_' {
			if b == '.' && i > 0 && i+1 < len(s) && (s[i-1] >= '0' && s[i-1] <= '9' || s[i-1] == 'H' || s[i-1] == 'h') && (s[i+1] >= '0' && s[i+1] <= '9') {
				sb.WriteByte(b)
			} else {
				sb.WriteByte(' ')
			}
		} else {
			sb.WriteByte(b)
		}
	}
	return sb.String()
}

func stripTags(s string, includeHDR bool, hints tagHints) string {
	if strings.IndexByte(s, '[') >= 0 {
		s = bracketedTagRegex.ReplaceAllString(s, " ")
	}
	if mayContainAny(toLowerASCII(s), "proper", "repack", "extended", "unrated", "cut", "imax", "multi", "dual",
		"complete", "internal", "subbed", "dubbed", "amzn", "nf", "dsnp", "hmax", "atvp", "apple", "criterion") &&
		sceneTagsRegex.MatchString(s) {
		s = sceneTagsRegex.ReplaceAllString(s, " ")
	}
	if hints.resolution {
		s = resolutionRegex.ReplaceAllString(s, " ")
	}
	if hints.source {
		s = sourceRegex.ReplaceAllString(s, " ")
	}
	if hints.codec {
		s = codecRegex.ReplaceAllString(s, " ")
	}
	if hints.audio {
		s = audioRegex.ReplaceAllString(s, " ")
	}
	if includeHDR && hints.hdr {
		s = hdrRegex.ReplaceAllString(s, " ")
	}
	return s
}

func sanitizeTitle(title string, hints tagHints) string {
	s := strings.ReplaceAll(title, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")

	s = stripTags(s, true, hints)

	words := strings.Fields(s)
	result := strings.Join(words, " ")
	result = strings.Trim(result, " -_()[]{}")
	return preferLatinScript(result)
}

func normalizeTag(tag string) string {
	t := strings.TrimSpace(tag)
	t = strings.ToLower(t)
	t = strings.ReplaceAll(t, ".", "")
	t = strings.ReplaceAll(t, "_", "")
	t = strings.ReplaceAll(t, "-", "")
	t = strings.ReplaceAll(t, " ", "")
	return t
}
