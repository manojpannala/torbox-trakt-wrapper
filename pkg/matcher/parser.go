package matcher

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	extRegex              = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|mov|wmv|flv|webm|m4v|ts|m2ts|iso)$`)
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
)

func ParseMedia(rawName string) ParsedMedia {
	parsed := ParsedMedia{
		OriginalName: rawName,
		Type:         MediaTypeUnknown,
	}

	filename := filepath.Base(rawName)
	clean := extRegex.ReplaceAllString(filename, "")

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
	cleanLower := strings.ToLower(clean)

	if match := resolutionRegex.FindString(normalizedSpaced); match != "" {
		parsed.Resolution = normalizeTag(match)
	}
	if match := sourceRegex.FindString(clean); match != "" {
		parsed.Source = normalizeTag(match)
	} else if match := sourceRegex.FindString(normalizedSpaced); match != "" {
		parsed.Source = normalizeTag(match)
	}
	if match := codecRegex.FindString(clean); match != "" {
		parsed.Codec = normalizeTag(match)
	} else if match := codecRegex.FindString(normalizedSpaced); match != "" {
		parsed.Codec = normalizeTag(match)
	}

	if strings.Contains(cleanLower, "truehd") && strings.Contains(cleanLower, "atmos") {
		parsed.Audio = "truehdatmos"
	} else if match := audioRegex.FindString(clean); match != "" {
		parsed.Audio = normalizeTag(match)
	} else if match := audioRegex.FindString(normalizedSpaced); match != "" {
		parsed.Audio = normalizeTag(match)
	}

	if match := hdrRegex.FindString(clean); match != "" {
		parsed.HDR = normalizeTag(match)
	} else if match := hdrRegex.FindString(normalizedSpaced); match != "" {
		parsed.HDR = normalizeTag(match)
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
		parsed.CleanTitle = sanitizeTitle(titlePart)
	} else if loc := explicitSeasonEpRegex.FindStringSubmatchIndex(clean); len(loc) >= 6 {
		seasonStr := clean[loc[2]:loc[3]]
		epStr := clean[loc[4]:loc[5]]
		parsed.Season, _ = strconv.Atoi(seasonStr)
		parsed.Episode, _ = strconv.Atoi(epStr)
		parsed.Type = MediaTypeEpisode

		titlePart := clean[:loc[0]]
		parsed.CleanTitle = sanitizeTitle(titlePart)
	} else if loc := altSeasonEpRegex.FindStringSubmatchIndex(clean); len(loc) >= 6 {
		seasonStr := clean[loc[2]:loc[3]]
		epStr := clean[loc[4]:loc[5]]
		parsed.Season, _ = strconv.Atoi(seasonStr)
		parsed.Episode, _ = strconv.Atoi(epStr)
		parsed.Type = MediaTypeEpisode

		titlePart := clean[:loc[0]]
		parsed.CleanTitle = sanitizeTitle(titlePart)
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
			parsed.CleanTitle = sanitizeTitle(titlePart)
		}
	}

	if parsed.Type == MediaTypeUnknown {
		trimmed := stripTags(normalizedSpaced, false)

		if parts := strings.Split(trimmed, " - "); len(parts) >= 2 {
			epCandidate := strings.TrimSpace(parts[len(parts)-1])
			if match := animeEpRegex.FindStringSubmatch(epCandidate); len(match) > 1 {
				if epNum, err := strconv.Atoi(match[1]); err == nil && epNum > 0 {
					parsed.Type = MediaTypeEpisode
					parsed.Season = 1
					parsed.Episode = epNum
					titlePart := strings.Join(parts[:len(parts)-1], " - ")
					parsed.CleanTitle = sanitizeTitle(titlePart)
				}
			}
		}
	}

	if parsed.CleanTitle == "" {
		parsed.CleanTitle = sanitizeTitle(normalizedSpaced)
		if parsed.Type == MediaTypeUnknown {
			parsed.Type = MediaTypeMovie
		}
	}

	return parsed
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

func stripTags(s string, includeHDR bool) string {
	s = bracketedTagRegex.ReplaceAllString(s, " ")
	s = sceneTagsRegex.ReplaceAllString(s, " ")
	s = resolutionRegex.ReplaceAllString(s, " ")
	s = sourceRegex.ReplaceAllString(s, " ")
	s = codecRegex.ReplaceAllString(s, " ")
	s = audioRegex.ReplaceAllString(s, " ")
	if includeHDR {
		s = hdrRegex.ReplaceAllString(s, " ")
	}
	return s
}

func sanitizeTitle(title string) string {
	s := strings.ReplaceAll(title, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")

	s = stripTags(s, true)

	words := strings.Fields(s)
	result := strings.Join(words, " ")
	result = strings.Trim(result, " -_()[]{}")
	return result
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
