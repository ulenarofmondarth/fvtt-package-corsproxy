package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// ALLOWED_URLS narrows ALLOWED_HOSTS to the URLs a client is known to need, so that a proxy
// whose API key ships inside a client is of little use for anything else. Each pattern is a
// host and a path, without the scheme or the query:
//
//	api.github.com/repos/*/*/releases   the release list of any repository
//	github.com/*/*/releases/download/** anything under a release's downloads
//	*.githubusercontent.com/**          any subdomain, any path
//	*/**/module.json                    a module.json on any host
//
// In the host, `*.` matches one or more labels before the rest, and `*` alone any host. In
// the path, `*` matches one segment, `**` any number of them (none included), and a segment
// may hold glob characters, as in `*.json`. The query string is ignored, as is a trailing
// slash.
//
// URL_ALLOWLIST_MODE=report lets unlisted URLs through and only logs them, to find out what a
// client asks for before enforcing a list.

// urlRule is one ALLOWED_URLS pattern.
type urlRule struct {
	// host is the host, lower case, without a leading `*.`; empty for any host.
	host string
	// subdomains is whether the host had a leading `*.`.
	subdomains bool
	path       []string
}

// urlRules are the parsed ALLOWED_URLS; none means any URL on an allowed host.
var urlRules []urlRule

// The URL_ALLOWLIST_MODE values.
const (
	urlModeEnforce = "enforce"
	urlModeReport  = "report"
)

// setupURLRules parses ALLOWED_URLS and checks URL_ALLOWLIST_MODE and CLIENT_VERSION_HEADER.
func setupURLRules() error {
	if credentialHeaders[http.CanonicalHeaderKey(config.ClientVersionHeader)] {
		return fmt.Errorf("CLIENT_VERSION_HEADER must not be %q, which carries credentials that would be logged", config.ClientVersionHeader)
	}
	if config.URLAllowlistMode != urlModeEnforce && config.URLAllowlistMode != urlModeReport {
		return fmt.Errorf("URL_ALLOWLIST_MODE must be %q or %q, not %q", urlModeEnforce, urlModeReport, config.URLAllowlistMode)
	}
	rules, err := parseURLRules(config.AllowedURLs)
	if err != nil {
		return err
	}
	urlRules = rules
	return nil
}

// parseURLRules parses ALLOWED_URLS patterns.
func parseURLRules(patterns []string) ([]urlRule, error) {
	var rules []urlRule
	for _, pattern := range patterns {
		rule, err := parseURLRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("ALLOWED_URLS pattern %q: %w", pattern, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseURLRule(pattern string) (urlRule, error) {
	if strings.Contains(pattern, "://") {
		return urlRule{}, errors.New("leave out the scheme")
	}
	host, rest, _ := strings.Cut(pattern, "/")
	host = strings.ToLower(host)
	rule := urlRule{path: segments(rest)}
	switch {
	case host == "":
		return urlRule{}, errors.New("no host")
	case host == "*":
	case strings.HasPrefix(host, "*.") && !strings.Contains(host[2:], "*"):
		rule.host, rule.subdomains = host[2:], true
	case strings.Contains(host, "*"):
		return urlRule{}, errors.New("a host may only start with `*.`, or be `*`")
	default:
		rule.host = host
	}
	for _, segment := range rule.path {
		if _, err := path.Match(segment, ""); err != nil {
			return urlRule{}, fmt.Errorf("segment %q: %w", segment, err)
		}
	}
	return rule, nil
}

// segments splits a path into its segments, ignoring empty ones (leading, trailing, doubled
// slashes).
func segments(p string) []string {
	var out []string
	for _, segment := range strings.Split(p, "/") {
		if segment != "" {
			out = append(out, segment)
		}
	}
	return out
}

// urlListed reports whether a URL matches ALLOWED_URLS, or whether there is no list.
func urlListed(u *url.URL) bool {
	if len(urlRules) == 0 {
		return true
	}
	host := strings.ToLower(u.Hostname())
	path := segments(u.EscapedPath())
	for _, rule := range urlRules {
		if rule.matchesHost(host) && matchSegments(rule.path, path) {
			return true
		}
	}
	return false
}

func (r urlRule) matchesHost(host string) bool {
	switch {
	case r.host == "":
		return true
	case r.subdomains:
		return strings.HasSuffix(host, "."+r.host)
	default:
		return host == r.host
	}
}

// matchSegments matches path segments against a pattern's, `**` taking any number of them.
func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(segments); skip++ {
			if matchSegments(pattern[1:], segments[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	matched, _ := path.Match(pattern[0], segments[0]) // the pattern was checked when parsed
	return matched && matchSegments(pattern[1:], segments[1:])
}

// checkURL decides on a URL the proxy is asked to fetch, or redirected to: whether to fetch it,
// logging it if it is not listed.
func checkURL(r *http.Request, u *url.URL) bool {
	if urlListed(u) {
		return true
	}
	allowed := config.URLAllowlistMode == urlModeReport
	if config.LogUnlistedURLs {
		action := "refused"
		if allowed {
			action = "allowed (report mode)"
		}
		log.Printf("📜 Not in ALLOWED_URLS, %s: %q client %q", action, u.String(), clientVersion(r))
	}
	return allowed
}

// credentialHeaders carry credentials, so cannot be CLIENT_VERSION_HEADER, whose value is logged.
var credentialHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"X-Api-Key":           true,
}

// versionLike is what a client's version is logged as: a word of at most 64 letters, digits
// and `.+_-`, starting with a letter or digit, such as `0.5.0` or `1.2.3-beta.1+build_7`.
var versionLike = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}$`)

// clientVersion is the client's CLIENT_VERSION_HEADER, for the logs: empty without one, and
// `?` for a value that does not look like a version. Nothing else from the header is logged,
// so neither a credential sent in it by mistake nor a forged log line can reach the log.
func clientVersion(r *http.Request) string {
	if config.ClientVersionHeader == "" {
		return ""
	}
	if value := r.Header.Get(config.ClientVersionHeader); versionLike.MatchString(value) {
		return value
	}
	return "?"
}
