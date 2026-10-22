package main

import (
	"bytes"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// withURLRules sets ALLOWED_URLS and its mode for one test.
func withURLRules(t *testing.T, mode string, patterns ...string) {
	t.Helper()
	testConfig()
	config.AllowPrivateNetworks = true
	config.URLAllowlistMode = mode
	config.LogUnlistedURLs = true
	rules, err := parseURLRules(patterns)
	if err != nil {
		t.Fatal(err)
	}
	urlRules = rules
	t.Cleanup(func() { urlRules = nil })
}

// captureLog collects what the proxy logs during one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestURLRulesMatch(t *testing.T) {
	cases := []struct {
		pattern string
		url     string
		want    bool
	}{
		{"api.github.com/repos/*/*/releases", "https://api.github.com/repos/Mondarth/EMMA/releases?per_page=100", true},
		{"api.github.com/repos/*/*/releases", "https://api.github.com/repos/Mondarth/EMMA/issues", false},
		{"api.github.com/repos/*/*/releases", "https://api.github.com/repos/Mondarth/releases", false},
		{"api.github.com/repos/*/*/releases", "https://api.github.com/repos/a/b/releases/latest", false},
		{"api.github.com/repos/*/*/releases", "https://API.GitHub.com/repos/a/b/releases", true},
		{"api.github.com/repos/*/*/releases", "https://evil.example/repos/a/b/releases", false},
		{"github.com/*/*/releases/download/**", "https://github.com/a/b/releases/download/v1.0.0/module.json", true},
		{"github.com/*/*/releases/download/**", "https://github.com/a/b/releases/download", true},
		{"github.com/*/*/releases/download/**", "https://github.com/a/b/archive/main.zip", false},
		{"*.githubusercontent.com/**", "https://objects.githubusercontent.com/github-production-release-asset/1/2", true},
		{"*.githubusercontent.com/**", "https://githubusercontent.com/anything", false},
		{"*.githubusercontent.com/**", "https://githubusercontent.com.evil.net/anything", false},
		{"*/**/module.json", "https://example.test/packages/dice/module.json", true},
		{"*/**/module.json", "https://example.test/module.json", true},
		{"*/**/module.json", "https://example.test/packages/dice/module.json.exe", false},
		{"*/**/*.json", "https://example.test/dice/system.json", true},
		{"foundryvtt.com/releases", "https://foundryvtt.com/releases/", true},
		{"foundryvtt.com/releases", "https://foundryvtt.com/releases/14.368", false},
	}
	for _, c := range cases {
		rules, err := parseURLRules([]string{c.pattern})
		if err != nil {
			t.Fatalf("%s: %v", c.pattern, err)
		}
		urlRules = rules
		if got := urlListed(mustParse(t, c.url)); got != c.want {
			t.Errorf("%s against %s = %v, want %v", c.pattern, c.url, got, c.want)
		}
	}
	urlRules = nil
}

func TestURLRulesRefuseBadPatterns(t *testing.T) {
	for _, pattern := range []string{"", "/repos/*", "**/releases", "api.*.com/x", "api.github.com/[", "https://api.github.com/x"} {
		if _, err := parseURLRules([]string{pattern}); err == nil {
			t.Errorf("%q: no error, want one", pattern)
		}
	}
}

func TestNoURLRulesAllowsEveryURL(t *testing.T) {
	urlRules = nil
	if !urlListed(mustParse(t, "https://anything.example/at/all")) {
		t.Error("an empty ALLOWED_URLS refused a URL")
	}
}

func TestLoadConfigURLAllowlist(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		loadConfig()
		if len(config.AllowedURLs) != 0 || config.URLAllowlistMode != "enforce" || !config.LogUnlistedURLs || config.ClientVersionHeader != "" {
			t.Errorf("AllowedURLs = %v, mode = %q, log = %v, header = %q",
				config.AllowedURLs, config.URLAllowlistMode, config.LogUnlistedURLs, config.ClientVersionHeader)
		}
		if err := setupURLRules(); err != nil || urlRules != nil {
			t.Errorf("setupURLRules() = %v, rules = %v, want none", err, urlRules)
		}
	})

	t.Run("from the environment", func(t *testing.T) {
		t.Setenv("ALLOWED_URLS", "api.github.com/repos/*/*/releases, *.githubusercontent.com/**")
		t.Setenv("URL_ALLOWLIST_MODE", "report")
		t.Setenv("LOG_UNLISTED_URLS", "false")
		t.Setenv("CLIENT_VERSION_HEADER", "X-EMMA-Version")
		loadConfig()
		t.Cleanup(func() { urlRules = nil })

		if err := setupURLRules(); err != nil {
			t.Fatal(err)
		}
		if len(urlRules) != 2 || config.URLAllowlistMode != "report" || config.LogUnlistedURLs || config.ClientVersionHeader != "X-EMMA-Version" {
			t.Errorf("rules = %d, mode = %q, log = %v, header = %q",
				len(urlRules), config.URLAllowlistMode, config.LogUnlistedURLs, config.ClientVersionHeader)
		}
	})

	t.Run("refusing what it cannot use", func(t *testing.T) {
		t.Setenv("ALLOWED_URLS", "**/releases")
		loadConfig()
		if err := setupURLRules(); err == nil {
			t.Error("a bad pattern: no error")
		}

		t.Setenv("ALLOWED_URLS", "api.github.com/**")
		t.Setenv("URL_ALLOWLIST_MODE", "warn")
		loadConfig()
		if err := setupURLRules(); err == nil {
			t.Error("an unknown mode: no error")
		}
		urlRules = nil
	})
}

func TestUnlistedURLsAreRefused(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })
	withURLRules(t, "enforce", "127.0.0.1/repos/*/*/releases")
	config.ClientVersionHeader = "X-EMMA-Version"
	logged := captureLog(t)

	if rec := get(u.URL + "/repos/a/b/releases"); rec.Code != http.StatusOK {
		t.Errorf("a listed URL: status = %d, want 200", rec.Code)
	}
	rec := proxy(http.MethodGet, u.URL+"/repos/a/b/issues", http.Header{"X-Emma-Version": {"0.5.0"}})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This URL is not allowed") {
		t.Errorf("an unlisted URL: got %d %q, want 403", rec.Code, rec.Body.String())
	}
	expectCalls(t, u, 1)
	if !strings.Contains(logged.String(), `refused`) || !strings.Contains(logged.String(), `/repos/a/b/issues"`) ||
		!strings.Contains(logged.String(), `client "0.5.0"`) {
		t.Errorf("log = %q, want the refused URL with the client's version", logged.String())
	}
}

func TestReportModeLetsUnlistedURLsThroughAndLogsThem(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("issues")) })
	withURLRules(t, "report", "127.0.0.1/repos/*/*/releases")
	config.ClientVersionHeader = "X-EMMA-Version"
	logged := captureLog(t)

	rec := proxy(http.MethodGet, u.URL+"/repos/a/b/issues", http.Header{"X-Emma-Version": {"0.5.0"}})

	if rec.Code != http.StatusOK || rec.Body.String() != "issues" {
		t.Errorf("got %d %q, want the upstream's answer", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logged.String(), `allowed (report mode)`) || !strings.Contains(logged.String(), `client "0.5.0"`) {
		t.Errorf("log = %q, want the URL reported with the client's version", logged.String())
	}
}

func TestUnlistedURLsCanGoUnlogged(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("issues")) })
	withURLRules(t, "enforce", "127.0.0.1/repos/*/*/releases")
	config.LogUnlistedURLs = false
	logged := captureLog(t)

	get(u.URL + "/repos/a/b/issues")
	if logged.Len() != 0 {
		t.Errorf("log = %q, want nothing", logged.String())
	}
}

func TestRedirectsToUnlistedURLsAreNotFollowed(t *testing.T) {
	var reached []string
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, r.URL.Path)
		if r.URL.Path == "/download/module.json" {
			http.Redirect(w, r, "/elsewhere/payload", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("payload"))
	})
	withURLRules(t, "enforce", "127.0.0.1/download/*")
	captureLog(t)

	rec := get(u.URL + "/download/module.json")

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 for a redirect to an unlisted URL", rec.Code)
	}
	if len(reached) != 1 {
		t.Errorf("upstream reached at %v, want only the listed URL", reached)
	}
}

func TestClientVersionHeaderIsNotForwarded(t *testing.T) {
	var forwarded string
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		forwarded = r.Header.Get("X-EMMA-Version")
		_, _ = w.Write([]byte("releases"))
	})
	withURLRules(t, "enforce")
	config.ClientVersionHeader = "X-EMMA-Version"

	proxy(http.MethodGet, u.URL, http.Header{"X-Emma-Version": {"0.5.0"}})
	if forwarded != "" {
		t.Errorf("upstream got X-EMMA-Version %q, want none", forwarded)
	}
}

func TestLoggedValuesCannotForgeLogLines(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {})
	withURLRules(t, "report", "127.0.0.1/listed")
	config.ClientVersionHeader = "X-EMMA-Version"
	config.EnableVerboseLog = true
	t.Cleanup(func() { config.EnableVerboseLog = false })
	logged := captureLog(t)
	forging := http.Header{"X-Emma-Version": {"0.5.0\nforged"}}

	// A newline in the target: Go will not parse it, so it is blocked, and logged as blocked.
	get(u.URL + "/a\nforged")
	// A newline in the client's version, logged with an unlisted URL and with every request.
	proxy(http.MethodGet, u.URL+"/unlisted", forging)
	proxy(http.MethodGet, u.URL+"/listed", forging)

	if !strings.Contains(logged.String(), "forged") || strings.Count(logged.String(), `client "?"`) < 2 {
		t.Fatalf("log = %q, want the blocked URL quoted and the forged version twice as ?", logged.String())
	}
	stamp := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)
	for _, line := range strings.Split(strings.TrimSpace(logged.String()), "\n") {
		if !stamp.MatchString(line) {
			t.Errorf("log line %q was not started by the logger", line)
		}
	}
}

func TestClientVersionIsLoggedOnlyWhenItLooksLikeOne(t *testing.T) {
	cases := map[string]string{
		"0.5.0":                       `client "0.5.0"`,
		"1.2.3-beta.1+build_7":        `client "1.2.3-beta.1+build_7"`,
		strings.Repeat("9", 64):       `client "` + strings.Repeat("9", 64) + `"`,
		strings.Repeat("9", 65):       `client "?"`,
		"0.5.0 secret":                `client "?"`,
		"Bearer github_pat_something": `client "?"`,
		"-1":                          `client "?"`,
	}
	for value, want := range cases {
		u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {})
		withURLRules(t, "enforce", "127.0.0.1/listed")
		config.ClientVersionHeader = "X-EMMA-Version"
		logged := captureLog(t)

		proxy(http.MethodGet, u.URL+"/unlisted", http.Header{"X-Emma-Version": {value}})

		if !strings.Contains(logged.String(), want) {
			t.Errorf("%q: log = %q, want %s", value, logged.String(), want)
		}
	}
}

func TestClientVersionHeaderCannotBeACredential(t *testing.T) {
	for _, name := range []string{"Authorization", "proxy-authorization", "Cookie", "X-API-Key"} {
		t.Setenv("CLIENT_VERSION_HEADER", name)
		loadConfig()
		if err := setupURLRules(); err == nil {
			t.Errorf("CLIENT_VERSION_HEADER=%s: no error, want one, as it would log a credential", name)
		}
	}
	urlRules = nil
}
