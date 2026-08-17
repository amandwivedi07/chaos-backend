package utils

import (
	"net/url"
	"strings"
)

// storedAvatarSize is what we persist for a Google picture.
//
// Google hands us a URL ending "=s96-c" — a 96x96 thumbnail, which is what it
// considers a profile icon. The app draws faces into tiles hundreds of points
// wide, where a 96px source is stretched past ten times its size and looks
// blurred. The size is a request parameter rather than a property of the
// stored image, so asking for a larger one costs nothing and needs no
// re-upload.
//
// 1024 is deliberately generous: it is stored once and read by app builds we
// cannot change, including every copy already installed. Newer builds rewrite
// this to the exact size they are about to draw, so the extra bytes are only
// ever paid by older ones.
const storedAvatarSize = "1024"

// NormalizeAvatarURL upgrades a Google profile picture URL to a usable
// resolution. Anything we host ourselves, and anything unparseable, is
// returned untouched.
func NormalizeAvatarURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(u.Hostname(), "googleusercontent.com") {
		return raw
	}

	// The size directive is the last '='-separated segment of the path, e.g.
	// ".../ACg8ocIv...=s96-c". Replace whatever is there with our own. The
	// "-c" suffix keeps Google's centre crop, which matches the cover fit the
	// app draws with; without it a non-square original comes back letterboxed.
	path := raw
	suffix := ""
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		path, suffix = raw[:i], raw[i:]
	}
	if i := strings.LastIndex(path, "="); i >= 0 {
		path = path[:i]
	}
	return path + "=s" + storedAvatarSize + "-c" + suffix
}
