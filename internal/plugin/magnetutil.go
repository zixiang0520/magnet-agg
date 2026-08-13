package plugin

import (
	"net/url"
	"regexp"
	"strings"
)

var btihRe = regexp.MustCompile(`(?i)btih:([a-fA-F0-9]{40}|[a-zA-Z2-7]{32})`)

// InfoHashFromMagnet extracts btih from a magnet URI.
func InfoHashFromMagnet(m string) string {
	m = strings.TrimSpace(m)
	if m == "" {
		return ""
	}
	if loc := btihRe.FindStringSubmatch(m); len(loc) > 1 {
		return strings.ToLower(loc[1])
	}
	if len(m) == 40 && isHex(m) {
		return strings.ToLower(m)
	}
	return ""
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// MagnetFromHash builds a basic magnet link.
func MagnetFromHash(hash, name string) string {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return ""
	}
	u := "magnet:?xt=urn:btih:" + hash
	if name != "" {
		u += "&dn=" + url.QueryEscape(name)
	}
	return u
}
