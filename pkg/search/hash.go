package search

import (
	"encoding/base32"
	"encoding/hex"
	"net/url"
	"strings"
)

// NormalizeHash returns a BitTorrent v1 infohash as lowercase 40-character
// hex, decoding the base32 form magnets sometimes carry. Anything else is "".
func NormalizeHash(s string) string {
	s = strings.TrimSpace(s)
	switch len(s) {
	case 40:
		if _, err := hex.DecodeString(s); err == nil {
			return strings.ToLower(s)
		}
	case 32:
		if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(s)); err == nil && len(b) == 20 {
			return hex.EncodeToString(b)
		}
	}
	return ""
}

// HashFromMagnet reads the btih infohash out of a magnet URI.
func HashFromMagnet(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if rest, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:"); ok {
			if h := NormalizeHash(xt[len(xt)-len(rest):]); h != "" {
				return h
			}
		}
	}
	return ""
}

// Magnet builds a hash-only magnet; TorBox finds peers for it itself.
func Magnet(hash, name string) string {
	m := "magnet:?xt=urn:btih:" + hash
	if name != "" {
		m += "&dn=" + url.QueryEscape(name)
	}
	return m
}
