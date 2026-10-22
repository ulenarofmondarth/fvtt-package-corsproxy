package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testClock is a clock the tests move by hand.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// cacheOptions are the settings of the cache a test runs with.
type cacheOptions struct {
	ttl, negativeTTL, maxStale time.Duration
	maxBytes, maxEntryBytes    int64
}

// withCache turns the response cache on for one test, on a clock the test controls.
func withCache(t *testing.T, opts cacheOptions) *testClock {
	t.Helper()
	testConfig()
	config.AllowPrivateNetworks = true // the upstreams are httptest servers on loopback
	if opts.ttl == 0 {
		opts.ttl = time.Minute
	}
	if opts.negativeTTL == 0 {
		opts.negativeTTL = 10 * time.Second
	}
	if opts.maxStale == 0 {
		opts.maxStale = time.Hour
	}
	if opts.maxBytes == 0 {
		opts.maxBytes = 1 << 20
	}
	if opts.maxEntryBytes == 0 {
		opts.maxEntryBytes = 1 << 16
	}
	clock := &testClock{now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	responseCache = newResponseCache(opts.ttl, opts.negativeTTL, opts.maxStale, opts.maxBytes, opts.maxEntryBytes)
	responseCache.now = clock.Now
	t.Cleanup(func() { responseCache = nil })
	return clock
}

// countingUpstream is an upstream that counts the requests it is sent.
type countingUpstream struct {
	*httptest.Server
	calls atomic.Int32
}

func newUpstream(t *testing.T, handler http.HandlerFunc) *countingUpstream {
	t.Helper()
	u := &countingUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

// proxy sends a request for target through the proxy.
func proxy(method, target string, header http.Header) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/?url="+url.QueryEscape(target), nil)
	for name, values := range header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	corsProxyHandler(rec, req)
	return rec
}

func get(target string) *httptest.ResponseRecorder {
	return proxy(http.MethodGet, target, nil)
}

func expectResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, body, cacheState string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d", rec.Code, status)
	}
	if got := rec.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	if got := rec.Header().Get("X-Cache"); got != cacheState {
		t.Errorf("X-Cache = %q, want %q", got, cacheState)
	}
}

func expectCalls(t *testing.T, u *countingUpstream, want int32) {
	t.Helper()
	if got := u.calls.Load(); got != want {
		t.Errorf("upstream was sent %d requests, want %d", got, want)
	}
}

func TestLoadConfigCacheSettings(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		loadConfig()
		if config.CacheTTL != 0 {
			t.Errorf("CacheTTL = %v, want 0", config.CacheTTL)
		}
		if setupCache(); responseCache != nil {
			t.Error("the cache is on without CACHE_TTL")
		}
		if config.CacheMaxStale != 24*time.Hour || config.CacheNegativeTTL != 5*time.Minute {
			t.Errorf("CacheMaxStale = %v, CacheNegativeTTL = %v", config.CacheMaxStale, config.CacheNegativeTTL)
		}
		if config.CacheMaxBytes != 64<<20 || config.CacheMaxEntryBytes != 2<<20 {
			t.Errorf("CacheMaxBytes = %d, CacheMaxEntryBytes = %d", config.CacheMaxBytes, config.CacheMaxEntryBytes)
		}
	})

	t.Run("from the environment", func(t *testing.T) {
		t.Setenv("CACHE_TTL", "1h")
		t.Setenv("CACHE_MAX_STALE", "6h")
		t.Setenv("CACHE_NEGATIVE_TTL", "1m")
		t.Setenv("CACHE_MAX_BYTES", "1000")
		t.Setenv("CACHE_MAX_ENTRY_BYTES", "100")
		loadConfig()
		setupCache()
		t.Cleanup(func() { responseCache = nil })

		if responseCache == nil {
			t.Fatal("the cache is off with CACHE_TTL set")
		}
		if responseCache.ttl != time.Hour || responseCache.maxStale != 6*time.Hour || responseCache.negativeTTL != time.Minute {
			t.Errorf("cache lifetimes = %v, %v, %v", responseCache.ttl, responseCache.maxStale, responseCache.negativeTTL)
		}
		if responseCache.maxBytes != 1000 || responseCache.maxEntryBytes != 100 {
			t.Errorf("cache sizes = %d, %d", responseCache.maxBytes, responseCache.maxEntryBytes)
		}
	})
}

func TestCacheOffSendsEveryRequestUpstream(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	responseCache = nil
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })

	expectResponse(t, get(u.URL), http.StatusOK, "releases", "")
	expectResponse(t, get(u.URL), http.StatusOK, "releases", "")
	expectCalls(t, u, 2)
}

func TestCacheServesARepeatedRequestFromTheCache(t *testing.T) {
	clock := withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0"}]`))
	})

	first := get(u.URL)
	expectResponse(t, first, http.StatusOK, `[{"tag_name":"v1.0.0"}]`, "MISS")

	clock.Advance(30 * time.Second)
	second := get(u.URL)
	expectResponse(t, second, http.StatusOK, `[{"tag_name":"v1.0.0"}]`, "HIT")
	expectCalls(t, u, 1)

	if got := second.Header().Get("Age"); got != "30" {
		t.Errorf("Age = %q, want 30", got)
	}
	if got := second.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want the upstream's", got)
	}
	if got := second.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := second.Header().Get("Content-Security-Policy"); got != "sandbox; default-src 'none'" {
		t.Errorf("Content-Security-Policy = %q, want the sandbox", got)
	}
	if got := strings.Join(second.Header().Values("Access-Control-Expose-Headers"), ", "); !strings.Contains(got, "X-Cache") || !strings.Contains(got, "Age") {
		t.Errorf("Access-Control-Expose-Headers = %q, want X-Cache and Age exposed", got)
	}
}

func TestCacheKeysOnTheURLAndTheNegotiatedHeaders(t *testing.T) {
	withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RawQuery + "|" + r.Header.Get("Accept")))
	})

	expectResponse(t, get(u.URL+"?page=1"), http.StatusOK, "page=1|", "MISS")
	expectResponse(t, get(u.URL+"?page=2"), http.StatusOK, "page=2|", "MISS")
	json := http.Header{"Accept": {"application/json"}}
	expectResponse(t, proxy(http.MethodGet, u.URL+"?page=1", json), http.StatusOK, "page=1|application/json", "MISS")
	expectResponse(t, proxy(http.MethodGet, u.URL+"?page=1", json), http.StatusOK, "page=1|application/json", "HIT")
	expectResponse(t, get(u.URL+"?page=1"), http.StatusOK, "page=1|", "HIT")
	expectCalls(t, u, 3)
}

func TestCacheLeavesRequestsItCannotAnswerAlone(t *testing.T) {
	cases := map[string]struct {
		method string
		header http.Header
	}{
		"a POST":                       {http.MethodPost, nil},
		"a range request":              {http.MethodGet, http.Header{"Range": {"bytes=0-9"}}},
		"a client's If-None-Match":     {http.MethodGet, http.Header{"If-None-Match": {`"abc"`}}},
		"a client's If-Modified-Since": {http.MethodGet, http.Header{"If-Modified-Since": {"Wed, 21 Oct 2026 07:28:00 GMT"}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			withCache(t, cacheOptions{})
			u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("answer")) })

			expectResponse(t, proxy(c.method, u.URL, c.header), http.StatusOK, "answer", "BYPASS")
			expectResponse(t, proxy(c.method, u.URL, c.header), http.StatusOK, "answer", "BYPASS")
			expectCalls(t, u, 2)
		})
	}
}

func TestCacheDoesNotKeepResponsesItShouldNot(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"a server error": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("answer"))
		},
		"a rate-limit refusal": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("answer"))
		},
		"one that sets a cookie": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Set-Cookie", "session=1")
			_, _ = w.Write([]byte("answer"))
		},
		"one marked no-store": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = w.Write([]byte("answer"))
		},
		"one that varies on everything": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Vary", "*")
			_, _ = w.Write([]byte("answer"))
		},
		"one too large for an entry": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("answer" + strings.Repeat(".", 100)))
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			withCache(t, cacheOptions{maxEntryBytes: 100})
			u := newUpstream(t, handler)

			first := get(u.URL)
			second := get(u.URL)
			expectCalls(t, u, 2)
			for _, rec := range []*httptest.ResponseRecorder{first, second} {
				if got := rec.Header().Get("X-Cache"); got != "MISS" {
					t.Errorf("X-Cache = %q, want MISS", got)
				}
				if rec.Header().Get("Set-Cookie") != "" {
					t.Error("Set-Cookie reached the client")
				}
			}
		})
	}
}

func TestCacheKeepsAMissingPageForTheNegativeLifetime(t *testing.T) {
	clock := withCache(t, cacheOptions{negativeTTL: 10 * time.Second})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	expectResponse(t, get(u.URL), http.StatusNotFound, "404 page not found\n", "MISS")
	clock.Advance(9 * time.Second)
	expectResponse(t, get(u.URL), http.StatusNotFound, "404 page not found\n", "HIT")
	expectCalls(t, u, 1)

	clock.Advance(2 * time.Second)
	expectResponse(t, get(u.URL), http.StatusNotFound, "404 page not found\n", "MISS")
	expectCalls(t, u, 2)
}

func TestCacheRevalidatesAnExpiredResponse(t *testing.T) {
	validators := map[string]struct {
		set       func(http.Header)
		condition string
		matches   func(*http.Request) bool
	}{
		"by its ETag": {
			set:       func(h http.Header) { h.Set("ETag", `"v1"`) },
			condition: "If-None-Match",
			matches:   func(r *http.Request) bool { return r.Header.Get("If-None-Match") == `"v1"` },
		},
		"by its Last-Modified": {
			set:       func(h http.Header) { h.Set("Last-Modified", "Wed, 21 Oct 2026 07:28:00 GMT") },
			condition: "If-Modified-Since",
			matches: func(r *http.Request) bool {
				return r.Header.Get("If-Modified-Since") == "Wed, 21 Oct 2026 07:28:00 GMT"
			},
		},
	}
	for name, v := range validators {
		t.Run(name, func(t *testing.T) {
			clock := withCache(t, cacheOptions{ttl: time.Minute})
			u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				v.set(w.Header())
				if v.matches(r) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				_, _ = w.Write([]byte("releases"))
			})

			expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")
			clock.Advance(2 * time.Minute)
			revalidated := get(u.URL)
			expectResponse(t, revalidated, http.StatusOK, "releases", "REVALIDATED")
			expectCalls(t, u, 2)
			if got := revalidated.Header().Get("Age"); got != "0" {
				t.Errorf("Age = %q, want 0 once revalidated", got)
			}

			// Fresh again for another lifetime.
			clock.Advance(30 * time.Second)
			expectResponse(t, get(u.URL), http.StatusOK, "releases", "HIT")
			expectCalls(t, u, 2)
		})
	}
}

func TestCacheReplacesAResponseThatChanged(t *testing.T) {
	clock := withCache(t, cacheOptions{ttl: time.Minute})
	version := "v1"
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+version+`"`)
		if r.Header.Get("If-None-Match") == `"`+version+`"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(version))
	})

	expectResponse(t, get(u.URL), http.StatusOK, "v1", "MISS")
	version = "v2"
	clock.Advance(2 * time.Minute)
	expectResponse(t, get(u.URL), http.StatusOK, "v2", "MISS")
	expectResponse(t, get(u.URL), http.StatusOK, "v2", "HIT")
	expectCalls(t, u, 2)
}

func TestCacheServesStaleWhenTheUpstreamRefuses(t *testing.T) {
	refusals := map[string]int{
		"GitHub's rate limit (403)": http.StatusForbidden,
		"too many requests (429)":   http.StatusTooManyRequests,
		"a server error (500)":      http.StatusInternalServerError,
		"a bad gateway (502)":       http.StatusBadGateway,
		"unavailable (503)":         http.StatusServiceUnavailable,
	}
	for name, status := range refusals {
		t.Run(name, func(t *testing.T) {
			clock := withCache(t, cacheOptions{ttl: time.Minute, maxStale: time.Hour})
			refuse := false
			u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				if refuse {
					w.WriteHeader(status)
					_, _ = w.Write([]byte("refused"))
					return
				}
				_, _ = w.Write([]byte("releases"))
			})

			expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")
			refuse = true
			clock.Advance(30 * time.Minute)
			stale := get(u.URL)
			expectResponse(t, stale, http.StatusOK, "releases", "STALE")
			if got := stale.Header().Get("Age"); got != "1800" {
				t.Errorf("Age = %q, want 1800", got)
			}

			// Past the TTL and the stale allowance together, the refusal is passed on.
			clock.Advance(31 * time.Minute)
			expectResponse(t, get(u.URL), status, "refused", "MISS")
		})
	}
}

func TestCacheServesStaleWhenTheUpstreamCannotBeReached(t *testing.T) {
	clock := withCache(t, cacheOptions{ttl: time.Minute})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })

	expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")
	u.Close()
	clock.Advance(2 * time.Minute)
	expectResponse(t, get(u.URL), http.StatusOK, "releases", "STALE")
}

func TestCacheDoesNotServeStaleForAMissingPage(t *testing.T) {
	clock := withCache(t, cacheOptions{ttl: time.Minute})
	gone := false
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if gone {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("releases"))
	})

	expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")
	gone = true
	clock.Advance(2 * time.Minute)
	expectResponse(t, get(u.URL), http.StatusNotFound, "404 page not found\n", "MISS")
}

func TestCacheSendsConcurrentRequestsUpstreamOnce(t *testing.T) {
	withCache(t, cacheOptions{})
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		_, _ = w.Write([]byte("releases"))
	})

	const clients = 10
	results := make([]*httptest.ResponseRecorder, clients)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = get(u.URL)
	}()
	<-arrived // the first request is upstream; the rest arrive while it is
	for i := 1; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = get(u.URL)
		}()
	}
	// Let the waiting requests reach the cache before the upstream answers.
	for responseCache.waiting(cacheKey(u.URL, http.Header{})) < clients-1 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()

	expectCalls(t, u, 1)
	for i, rec := range results {
		if rec.Code != http.StatusOK || rec.Body.String() != "releases" {
			t.Errorf("client %d: %d %q", i, rec.Code, rec.Body.String())
		}
	}
}

func TestCacheEvictsTheLeastRecentlyUsed(t *testing.T) {
	// Room for two 100-byte bodies with their headers, not three.
	withCache(t, cacheOptions{maxBytes: 500, maxEntryBytes: 250})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(r.URL.Query().Get("page"), 100)))
	})
	page := func(n int) string { return u.URL + "?page=" + strconv.Itoa(n) }

	get(page(1))
	get(page(2))
	if got := get(page(1)).Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("page 1: X-Cache = %q, want HIT", got)
	}
	get(page(3)) // evicts page 2, the least recently used

	if got := get(page(1)).Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("page 1: X-Cache = %q, want HIT", got)
	}
	if got := get(page(3)).Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("page 3: X-Cache = %q, want HIT", got)
	}
	if got := get(page(2)).Header().Get("X-Cache"); got != "MISS" {
		t.Errorf("page 2: X-Cache = %q, want MISS", got)
	}
	if responseCache.size > responseCache.maxBytes {
		t.Errorf("cache holds %d bytes, over its %d", responseCache.size, responseCache.maxBytes)
	}
}

func TestCacheStillRequiresTheAPIKey(t *testing.T) {
	withCache(t, cacheOptions{})
	config.RequireAPIKey = true
	config.APIKeys = []string{"good-key"}
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })

	expectResponse(t, proxy(http.MethodGet, u.URL, http.Header{"X-Api-Key": {"good-key"}}), http.StatusOK, "releases", "MISS")
	if rec := get(u.URL); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a key: status = %d, want 401", rec.Code)
	}
	expectCalls(t, u, 1)
}

func TestCacheStillRefusesHostsThatAreNotAllowed(t *testing.T) {
	withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })
	get(u.URL)

	config.AllowedHosts = []string{"api.github.com"}
	if rec := get(u.URL); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a host that is not allowed", rec.Code)
	}
}

func TestCachePassesOnAFailureWithNothingToServe(t *testing.T) {
	withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {})
	u.Close()

	rec := get(u.URL)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Failed to fetch URL") {
		t.Errorf("got %d %q, want the proxy's 502", rec.Code, rec.Body.String())
	}
}
