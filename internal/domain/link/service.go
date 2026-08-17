// Package link turns a shared URL into the little preview a card shows —
// title, description, picture — by fetching the page and reading its Open
// Graph tags. It is deliberately paranoid: the server is being asked to make
// an outbound request on a stranger's behalf, so it refuses private addresses,
// caps how much it will read, and caches hard.
package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/html"

	"github.com/chaosapp/backend/internal/cache"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
)

const (
	// Pages are read only far enough to reach </head>; nobody puts og: tags
	// after a megabyte of markup.
	maxBody = 512 << 10
	// Whole-fetch budget. A slow site must not hold a request open.
	fetchTimeout = 6 * time.Second
	// Previews of a URL are the same for everyone, and stale metadata is a
	// non-event, so cache generously.
	cacheTTL = 24 * time.Hour
	// Except when the picture is a pre-signed URL that outlives the cache by
	// minutes rather than hours. Kept under the shortest lifetime we have seen
	// in the wild (GitHub issues five).
	signedCacheTTL = 3 * time.Minute
	// Some sites serve richer tags to crawlers than to unknown clients.
	userAgent = "Mozilla/5.0 (compatible; SpaceBot/1.0; +https://spacechatapp.com)"
)

// Preview is what a card renders. Every field is best-effort: a page with no
// metadata at all still yields a usable Host.
type Preview struct {
	URL         string `json:"url"`
	Host        string `json:"host"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
}

type Service interface {
	Preview(ctx context.Context, raw string) (*Preview, error)
}

type service struct {
	client *http.Client
	cache  cache.Store
	log    *zap.Logger
}

func New(c cache.Store, log *zap.Logger) Service {
	return &service{client: newGuardedClient(), cache: c, log: log}
}

// newGuardedClient refuses to connect to anything that is not a public
// address. The check runs on the resolved IP at dial time rather than on the
// hostname, which is the only placement that survives DNS rebinding — a name
// that resolved publicly a moment ago can resolve to 169.254.169.254 now.
func newGuardedClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 4 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublic(ip) {
				return fmt.Errorf("refusing to dial non-public address %s", host)
			}
			return nil
		},
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 4 * time.Second,
			DisableKeepAlives:   true,
		},
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many redirects")
			}
			// Redirects are re-dialed through the same Control hook, so a
			// public URL cannot bounce us inward — but the scheme still has
			// to stay something we are willing to speak.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

func isPublic(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsInterfaceLocalMulticast() && !ip.IsMulticast()
}

// normalise accepts what people actually paste — "example.com/x" as often as a
// full URL — and returns something safe to fetch.
func normalise(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.BadRequest("No link to preview")
	}
	if len(raw) > 2000 {
		return nil, apperrors.BadRequest("That link is too long")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, apperrors.BadRequest("That does not look like a link")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, apperrors.BadRequest("Only web links can be previewed")
	}
	u.Fragment = ""
	return u, nil
}

func (s *service) Preview(ctx context.Context, raw string) (*Preview, error) {
	u, err := normalise(raw)
	if err != nil {
		return nil, err
	}
	key := "link:preview:" + u.String()
	if hit, ok, err := s.cache.Get(ctx, key); err == nil && ok {
		var cached Preview
		if json.Unmarshal([]byte(hit), &cached) == nil {
			return &cached, nil
		}
	}

	// Host alone is a real answer: a link that will not load still deserves to
	// render as something better than raw text.
	preview := &Preview{URL: u.String(), Host: strings.TrimPrefix(u.Host, "www.")}
	if doc, err := s.fetch(ctx, u); err != nil {
		s.log.Debug("link preview fetch failed",
			zap.String("host", u.Host), zap.Error(err))
	} else {
		scrape(doc, u, preview)
	}

	if body, err := json.Marshal(preview); err == nil {
		_ = s.cache.Set(ctx, key, string(body), ttlFor(preview))
	}
	return preview, nil
}

// ttlFor shortens the cache window when the picture is behind a signed URL.
// GitHub, for one, hands out an og:image valid for five minutes; caching that
// for a day means every preview after the first renders with a dead image.
// Re-reading the page is cheap next to showing a broken card.
func ttlFor(p *Preview) time.Duration {
	if p.ImageURL == "" || !signedURL(p.ImageURL) {
		return cacheTTL
	}
	return signedCacheTTL
}

// signedURL spots the query parameters that pre-signed object URLs carry.
// Deliberately broad: a false positive only costs an extra fetch, while a
// false negative shows the reader a broken image for a day.
func signedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for key := range u.Query() {
		switch strings.ToLower(key) {
		case "x-amz-expires", "x-amz-signature", "x-amz-credential",
			"expires", "signature", "sig", "token", "se", "st":
			return true
		}
	}
	return false
}

func (s *service) fetch(ctx context.Context, u *url.URL) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" &&
		!strings.Contains(strings.ToLower(ct), "html") {
		return nil, fmt.Errorf("not html: %s", ct)
	}
	return html.Parse(io.LimitReader(resp.Body, maxBody))
}

// scrape walks the document once, collecting every meta tag, then resolves
// each field by preference: Open Graph, then Twitter cards, then the plain
// <title>/<meta name=description>. Collecting first matters — pages routinely
// put <meta name="description"> above og:description, and picking as we walk
// would let document order decide instead of priority.
func scrape(doc *html.Node, base *url.URL, out *Preview) {
	metas := map[string]string{}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if title == "" && n.FirstChild != nil {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				if key, content := metaOf(n); key != "" && content != "" {
					if _, seen := metas[key]; !seen {
						metas[key] = content
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := metas[k]; v != "" {
				return v
			}
		}
		return ""
	}
	out.Title = pick("og:title", "twitter:title")
	out.Description = pick("og:description", "twitter:description", "description")
	out.ImageURL = pick("og:image", "og:image:url", "twitter:image")
	out.SiteName = pick("og:site_name", "application-name")

	if out.Title == "" {
		out.Title = title
	}
	out.Title = clip(out.Title, 200)
	out.Description = clip(out.Description, 400)
	// og:image is routinely a relative path; the app needs something absolute.
	if out.ImageURL != "" {
		if ref, err := url.Parse(out.ImageURL); err == nil {
			abs := base.ResolveReference(ref)
			if abs.Scheme == "http" || abs.Scheme == "https" {
				out.ImageURL = abs.String()
			} else {
				out.ImageURL = ""
			}
		} else {
			out.ImageURL = ""
		}
	}
}

// metaOf returns the identifying key of a <meta> tag — og: tags use
// "property", the rest use "name" — along with its content.
func metaOf(n *html.Node) (string, string) {
	var key, content string
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "property", "name":
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(a.Val))
			}
		case "content":
			content = strings.TrimSpace(a.Val)
		}
	}
	return key, content
}

func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}
