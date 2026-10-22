package main

import (
	"container/list"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The response cache keeps upstream answers in memory, shared by every client, so that a URL
// many clients ask for (a GitHub release list, a package manifest) is fetched upstream once per
// CACHE_TTL rather than once per request. It is off unless CACHE_TTL is set.
//
// Only what any client could have fetched itself is kept: the proxy never forwards credentials
// (see nonForwardedHeaders), so an answer depends on the URL and on the Accept and
// Accept-Encoding headers alone, which make up the key. Clients cannot make the proxy skip the
// cache: it is there to protect the upstreams' budgets, which every client shares.

// The X-Cache states.
const (
	cacheMiss        = "MISS"        // fetched upstream
	cacheHit         = "HIT"         // served from the cache, fresh
	cacheRevalidated = "REVALIDATED" // expired, and the upstream confirmed it unchanged (304)
	cacheStale       = "STALE"       // expired, served because the upstream refused or failed
	cacheBypass      = "BYPASS"      // a request the cache does not answer (a range, or If-Match and the like)
)

// cachedResponse is an upstream answer, as the client is sent it.
type cachedResponse struct {
	status int
	header http.Header
	body   []byte
	// setsCookies is whether the upstream set cookies, which the client is never sent. Such an
	// answer may be meant for one client only, so it is not kept.
	setsCookies bool
	// stored is when the upstream last gave or confirmed the answer.
	stored time.Time
}

// age is how long ago the upstream last gave or confirmed the answer.
func (c *cachedResponse) age(now time.Time) time.Duration {
	return now.Sub(c.stored)
}

// cacheEntry is a response in the cache's LRU list.
type cacheEntry struct {
	key      string
	response *cachedResponse
	size     int64
}

// flight is one upstream fetch, which every request for the same key waits on.
type flight struct {
	done    chan struct{}
	waiters int
	result  cacheResult
}

// cacheResult is the answer to a request through the cache.
type cacheResult struct {
	response *cachedResponse
	state    string
	err      error
}

// fetchFunc fetches the target upstream, with the extra headers (the conditions of a
// revalidation) added to the request.
type fetchFunc func(extra http.Header) (*cachedResponse, error)

type memoryCache struct {
	ttl           time.Duration
	negativeTTL   time.Duration
	maxStale      time.Duration
	maxBytes      int64
	maxEntryBytes int64
	now           func() time.Time

	mu       sync.Mutex
	size     int64
	entries  map[string]*list.Element
	lru      *list.List // most recently used at the front
	inflight map[string]*flight
}

// responseCache is nil while the cache is off.
var responseCache *memoryCache

func newResponseCache(ttl, negativeTTL, maxStale time.Duration, maxBytes, maxEntryBytes int64) *memoryCache {
	return &memoryCache{
		ttl:           ttl,
		negativeTTL:   negativeTTL,
		maxStale:      maxStale,
		maxBytes:      maxBytes,
		maxEntryBytes: maxEntryBytes,
		now:           time.Now,
		entries:       make(map[string]*list.Element),
		lru:           list.New(),
		inflight:      make(map[string]*flight),
	}
}

// setupCache turns the cache on if CACHE_TTL is set.
func setupCache() {
	if config.CacheTTL <= 0 {
		responseCache = nil
		return
	}
	responseCache = newResponseCache(config.CacheTTL, config.CacheNegativeTTL, config.CacheMaxStale,
		config.CacheMaxBytes, config.CacheMaxEntryBytes)
}

// cacheable reports whether the cache answers a request, which is always a GET. A range, or a
// request on the conditions If-Match, If-Unmodified-Since or If-Range, wants something other than
// the whole, current answer, and goes upstream. A revalidation (If-None-Match, If-Modified-Since),
// as a browser sends for an answer it has kept, the cache answers itself (see notModified).
func cacheable(r *http.Request) bool {
	for _, name := range []string{"Range", "If-Match", "If-Unmodified-Since", "If-Range"} {
		if r.Header.Get(name) != "" {
			return false
		}
	}
	return true
}

// cacheKey is the key of a GET for target with the request's headers.
func cacheKey(target string, header http.Header) string {
	return target + "\x00" + header.Get("Accept") + "\x00" + header.Get("Accept-Encoding")
}

// lifetime is how long a response stays fresh, or 0 if it must not be kept.
func (c *memoryCache) lifetime(resp *cachedResponse) time.Duration {
	var ttl time.Duration
	switch resp.status {
	case http.StatusOK:
		ttl = c.ttl
	case http.StatusNotFound, http.StatusGone:
		ttl = c.negativeTTL
	default:
		return 0
	}
	if resp.setsCookies || resp.header.Get("Vary") == "*" {
		return 0
	}
	for _, directive := range strings.Split(resp.header.Get("Cache-Control"), ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-store") {
			return 0
		}
	}
	return ttl
}

// refused reports whether an upstream status says "not now" rather than answering: a rate limit
// (GitHub's is a 403) or a server error. An expired answer is better than these.
func refused(status int) bool {
	return status == http.StatusForbidden || status == http.StatusTooManyRequests || status >= 500
}

// get answers a GET for key, from the cache or by fetching it. Requests for a key that is being
// fetched wait for that fetch rather than sending their own.
func (c *memoryCache) get(key string, fetch fetchFunc) cacheResult {
	c.mu.Lock()
	now := c.now()
	cached := c.lookup(key)
	if cached != nil && cached.age(now) < c.lifetime(cached) {
		c.mu.Unlock()
		return cacheResult{response: cached, state: cacheHit}
	}
	if f, ok := c.inflight[key]; ok {
		f.waiters++
		c.mu.Unlock()
		<-f.done
		return f.result
	}
	f := &flight{done: make(chan struct{})}
	c.inflight[key] = f
	c.mu.Unlock()

	f.result = c.refresh(key, cached, fetch)

	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	close(f.done)
	return f.result
}

// refresh fetches key upstream, revalidating the expired answer if there is one.
func (c *memoryCache) refresh(key string, expired *cachedResponse, fetch fetchFunc) cacheResult {
	conditions := http.Header{}
	if expired != nil {
		if etag := expired.header.Get("ETag"); etag != "" {
			conditions.Set("If-None-Match", etag)
		}
		if modified := expired.header.Get("Last-Modified"); modified != "" {
			conditions.Set("If-Modified-Since", modified)
		}
	}

	resp, err := fetch(conditions)
	now := c.now()
	usable := expired != nil && expired.age(now) < c.lifetime(expired)+c.maxStale
	switch {
	case err != nil:
		// The upstream could not be reached; a refusal by policy is passed on all the same.
		if usable && !errors.Is(err, errPrivateAddress) {
			return cacheResult{response: expired, state: cacheStale}
		}
		return cacheResult{err: err, state: cacheMiss}
	case resp.status == http.StatusNotModified && expired != nil:
		renewed := &cachedResponse{status: expired.status, header: expired.header, body: expired.body, stored: now}
		c.store(key, renewed)
		return cacheResult{response: renewed, state: cacheRevalidated}
	case refused(resp.status) && usable:
		return cacheResult{response: expired, state: cacheStale}
	}

	resp.stored = now
	if c.lifetime(resp) > 0 {
		c.store(key, resp)
	} else {
		c.remove(key)
	}
	return cacheResult{response: resp, state: cacheMiss}
}

// lookup finds key's answer and marks it used. The caller holds c.mu.
func (c *memoryCache) lookup(key string) *cachedResponse {
	element, ok := c.entries[key]
	if !ok {
		return nil
	}
	c.lru.MoveToFront(element)
	return element.Value.(*cacheEntry).response
}

// store keeps an answer, unless it is too large, evicting the least recently used to make room.
func (c *memoryCache) store(key string, resp *cachedResponse) {
	size := int64(len(key) + len(resp.body))
	for name, values := range resp.header {
		for _, value := range values {
			size += int64(len(name) + len(value))
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(key)
	if size > c.maxEntryBytes || size > c.maxBytes {
		return
	}
	for c.size+size > c.maxBytes {
		c.removeLocked(c.lru.Back().Value.(*cacheEntry).key)
	}
	c.entries[key] = c.lru.PushFront(&cacheEntry{key: key, response: resp, size: size})
	c.size += size
}

func (c *memoryCache) remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(key)
}

func (c *memoryCache) removeLocked(key string) {
	element, ok := c.entries[key]
	if !ok {
		return
	}
	c.lru.Remove(element)
	delete(c.entries, key)
	c.size -= element.Value.(*cacheEntry).size
}

// waiting is how many requests are waiting on the fetch of key, for the tests.
func (c *memoryCache) waiting(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.inflight[key]; ok {
		return f.waiters
	}
	return 0
}

// revalidationHeaders are the conditions on which a client asks for an answer only if it has
// changed. The cache answers them itself, so they never go upstream with a fetch it shares.
var revalidationHeaders = []string{"If-None-Match", "If-Modified-Since"}

// withoutRevalidation is a client's request without its revalidation conditions, for the cache to
// fetch on behalf of every client that asks for the same answer.
func withoutRevalidation(r *http.Request) *http.Request {
	clean := r.Clone(r.Context())
	for _, name := range revalidationHeaders {
		clean.Header.Del(name)
	}
	return clean
}

// notModified reports whether a client's revalidation conditions say it already has the answer, so
// it is sent 304 Not Modified. Only a 200 answer is compared. If-None-Match decides alone when it
// is sent, compared weakly, as for a GET; otherwise If-Modified-Since is compared with the answer's
// Last-Modified.
func notModified(r *http.Request, resp *cachedResponse) bool {
	if resp.status != http.StatusOK {
		return false
	}
	if tags := r.Header.Values("If-None-Match"); len(tags) > 0 {
		return etagMatches(strings.Join(tags, ","), resp.header.Get("ETag"))
	}
	since, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
	if err != nil {
		return false
	}
	modified, err := http.ParseTime(resp.header.Get("Last-Modified"))
	return err == nil && !modified.After(since)
}

// etagMatches reports whether a list of entity tags, or `*`, matches an answer's ETag, comparing
// weakly: `W/"v1"` matches `"v1"`.
func etagMatches(list, etag string) bool {
	if etag == "" {
		return false
	}
	weak := func(tag string) string { return strings.TrimPrefix(strings.TrimSpace(tag), "W/") }
	for _, tag := range strings.Split(list, ",") {
		if strings.TrimSpace(tag) == "*" || weak(tag) == weak(etag) {
			return true
		}
	}
	return false
}
