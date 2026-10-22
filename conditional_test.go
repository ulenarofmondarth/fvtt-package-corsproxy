package main

import (
	"net/http"
	"testing"
	"time"
)

// A browser caches the proxy's answers and revalidates them with conditions of its own. The cache
// answers those itself, so revalidations no longer reach the upstream.

const lastModified = "Wed, 21 Oct 2026 07:28:00 GMT"

// releasesUpstream answers with a fixed ETag and Last-Modified, recording each request's conditions.
func releasesUpstream(t *testing.T, conditions *[]string) *countingUpstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		*conditions = append(*conditions, r.Header.Get("If-None-Match")+"|"+r.Header.Get("If-Modified-Since"))
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", lastModified)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("releases"))
	})
}

func TestCacheAnswersAClientsRevalidationItself(t *testing.T) {
	cases := map[string]struct {
		header http.Header
		status int
		body   string
	}{
		"a matching If-None-Match":            {http.Header{"If-None-Match": {`"v1"`}}, http.StatusNotModified, ""},
		"one of several in If-None-Match":     {http.Header{"If-None-Match": {`"v0", "v1"`}}, http.StatusNotModified, ""},
		"a weak If-None-Match":                {http.Header{"If-None-Match": {`W/"v1"`}}, http.StatusNotModified, ""},
		"If-None-Match: *":                    {http.Header{"If-None-Match": {"*"}}, http.StatusNotModified, ""},
		"an If-None-Match that differs":       {http.Header{"If-None-Match": {`"v0"`}}, http.StatusOK, "releases"},
		"If-Modified-Since its date":          {http.Header{"If-Modified-Since": {lastModified}}, http.StatusNotModified, ""},
		"If-Modified-Since a later date":      {http.Header{"If-Modified-Since": {"Thu, 22 Oct 2026 07:28:00 GMT"}}, http.StatusNotModified, ""},
		"If-Modified-Since an earlier date":   {http.Header{"If-Modified-Since": {"Tue, 20 Oct 2026 07:28:00 GMT"}}, http.StatusOK, "releases"},
		"an If-Modified-Since it cannot read": {http.Header{"If-Modified-Since": {"yesterday"}}, http.StatusOK, "releases"},
		// If-None-Match decides alone when both are sent.
		"a differing If-None-Match with a matching date": {
			http.Header{"If-None-Match": {`"v0"`}, "If-Modified-Since": {lastModified}}, http.StatusOK, "releases",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			withCache(t, cacheOptions{})
			var conditions []string
			u := releasesUpstream(t, &conditions)
			expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")

			rec := proxy(http.MethodGet, u.URL, c.header)

			expectResponse(t, rec, c.status, c.body, "HIT")
			expectCalls(t, u, 1)
			if got := rec.Header().Get("ETag"); got != `"v1"` {
				t.Errorf("ETag = %q, want the cached one", got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
			if c.status == http.StatusNotModified && rec.Header().Get("Content-Length") != "" {
				t.Errorf("Content-Length = %q on a 304, want none", rec.Header().Get("Content-Length"))
			}
		})
	}
}

func TestCacheFetchesForAClientsRevalidationWithoutItsConditions(t *testing.T) {
	withCache(t, cacheOptions{})
	var conditions []string
	u := releasesUpstream(t, &conditions)

	// Nothing is cached yet: the cache fetches the whole answer, keeps it, and answers the client's condition.
	rec := proxy(http.MethodGet, u.URL, http.Header{"If-None-Match": {`"v1"`}, "If-Modified-Since": {lastModified}})

	expectResponse(t, rec, http.StatusNotModified, "", "MISS")
	if len(conditions) != 1 || conditions[0] != "|" {
		t.Errorf("upstream conditions = %q, want none of the client's", conditions)
	}
	expectResponse(t, get(u.URL), http.StatusOK, "releases", "HIT")
	expectCalls(t, u, 1)
}

func TestCacheRevalidatesWithItsOwnConditionsForAClient(t *testing.T) {
	clock := withCache(t, cacheOptions{ttl: time.Minute})
	var conditions []string
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		conditions = append(conditions, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte("releases"))
	})
	get(u.URL)
	clock.Advance(2 * time.Minute)

	rec := proxy(http.MethodGet, u.URL, http.Header{"If-None-Match": {`"v0"`}})

	// The cache asks with its own ETag, not the client's, and gives the client the whole answer it lacks.
	expectResponse(t, rec, http.StatusOK, "releases", "REVALIDATED")
	if len(conditions) != 2 || conditions[1] != `"v1"` {
		t.Errorf("upstream If-None-Match = %q, want the cache's own", conditions)
	}
}

func TestCacheServesAMissingPageWhateverTheClientsConditions(t *testing.T) {
	withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"gone"`)
		http.NotFound(w, r)
	})
	get(u.URL)

	rec := proxy(http.MethodGet, u.URL, http.Header{"If-None-Match": {`"gone"`}})

	expectResponse(t, rec, http.StatusNotFound, "404 page not found\n", "HIT")
}

func TestWithoutTheCacheAClientsConditionsGoUpstream(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	responseCache = nil
	var conditions []string
	u := releasesUpstream(t, &conditions)

	proxy(http.MethodGet, u.URL, http.Header{"If-None-Match": {`"v1"`}})

	if len(conditions) != 1 || conditions[0] != `"v1"|` {
		t.Errorf("upstream conditions = %q, want the client's", conditions)
	}
}

func TestCacheSendsTheWholeAnswerWhenItHasNoETagToMatch(t *testing.T) {
	withCache(t, cacheOptions{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("releases")) })
	get(u.URL)

	for _, tag := range []string{`"v1"`, "*"} {
		rec := proxy(http.MethodGet, u.URL, http.Header{"If-None-Match": {tag}})
		expectResponse(t, rec, http.StatusOK, "releases", "HIT")
	}
}
