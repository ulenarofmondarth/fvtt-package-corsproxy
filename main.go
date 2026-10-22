package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Configuration with defaults
type Config struct {
	Port               string
	MaxRequestSize     int64
	RequestTimeout     time.Duration
	MaxRedirects       int
	AllowedOrigins     []string
	BlockedHosts       []string
	AllowedHosts       []string
	EnableVerboseLog   bool
	RateLimitPerMinute int
	// AllowPrivateNetworks lets the proxy reach loopback/private/link-local
	// addresses. Off by default: an open proxy that can reach them is an
	// SSRF hole (cloud metadata, internal admin panels).
	AllowPrivateNetworks bool
	// RequireAPIKey gates every proxied request behind one of APIKeys.
	// Off by default so the zero-config quick start keeps working.
	RequireAPIKey bool
	APIKeys       []string
	// DailyRequestLimit is a global (not per-IP) kill switch on total
	// proxied requests per rolling 24h window. 0 disables it.
	DailyRequestLimit int
	// TrustProxyHeaders makes getClientIP trust X-Forwarded-For. Off by
	// default: a client with no reverse proxy in front trivially spoofs it
	// to bypass RATE_LIMIT_PER_MINUTE. Only enable this behind a reverse
	// proxy/platform that actually sets or overwrites the header itself.
	TrustProxyHeaders bool
	// CacheTTL is how long an upstream answer is served from the response cache before it is
	// fetched or revalidated again. 0 turns the cache off.
	CacheTTL time.Duration
	// CacheNegativeTTL is the same for a 404 or 410 answer.
	CacheNegativeTTL time.Duration
	// CacheMaxStale is how long past CacheTTL an answer is still served when the upstream
	// refuses (a rate limit, a server error) or cannot be reached.
	CacheMaxStale time.Duration
	// CacheMaxBytes bounds the memory the cache uses; CacheMaxEntryBytes, one answer in it.
	CacheMaxBytes      int64
	CacheMaxEntryBytes int64
	// GitHubToken authenticates the proxy's requests to GitHub's API (see github.go).
	GitHubToken string
	// AllowedURLs narrow AllowedHosts to host and path patterns (see allowlist.go).
	AllowedURLs []string
	// URLAllowlistMode is "enforce", refusing unlisted URLs, or "report", only logging them.
	URLAllowlistMode string
	// LogUnlistedURLs logs each URL not in AllowedURLs, refused or reported.
	LogUnlistedURLs bool
	// ClientVersionHeader names a request header whose value is logged with unlisted URLs,
	// such as a client's version, and is never forwarded upstream.
	ClientVersionHeader string
}

var (
	config      Config
	rateLimiter = make(map[string]*RateLimit)
	rateMutex   sync.RWMutex

	dailyCount     int64
	dailyResetTime time.Time
	dailyMutex     sync.Mutex
)

// Set at build time: go build -ldflags "-X main.version=v1.2.6". Docker and
// release binaries carry the git tag; a plain `go run .` reports "dev".
var version = "dev"

type RateLimit struct {
	count     int
	resetTime time.Time
}

func main() {
	loadConfig()
	if err := setupURLRules(); err != nil {
		log.Fatalf("❌ %v", err)
	}

	http.HandleFunc("/", corsProxyHandler)
	http.HandleFunc("/health", healthCheckHandler)

	log.Printf("🚀 CORS Proxy %s starting on port %s", version, config.Port)
	log.Printf("📝 Usage: http://localhost:%s/?url=https://example.com", config.Port)
	log.Printf("⚙️  Max request size: %d MB", config.MaxRequestSize/(1024*1024))
	log.Printf("⏱️  Request timeout: %v", config.RequestTimeout)
	if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
		log.Printf("🌐 CORS: All origins allowed (*)")
	} else {
		log.Printf("🌐 CORS: Specific origins allowed: %v", config.AllowedOrigins)
	}
	if config.RateLimitPerMinute > 0 {
		log.Printf("🚦 Rate limit: %d requests/minute per IP", config.RateLimitPerMinute)
	}
	if len(config.AllowedHosts) > 0 {
		log.Printf("✅ Allowed hosts: %v", config.AllowedHosts)
	}
	if len(urlRules) > 0 {
		log.Printf("📜 Allowed URLs: %d pattern(s), %s", len(urlRules), config.URLAllowlistMode)
	}
	if len(config.BlockedHosts) > 0 {
		log.Printf("🚫 Blocked hosts: %v", config.BlockedHosts)
	}
	if config.RequireAPIKey {
		log.Printf("🔑 API key required (%d key(s) configured)", len(config.APIKeys))
	}
	if config.DailyRequestLimit > 0 {
		log.Printf("🛑 Daily request limit: %d", config.DailyRequestLimit)
	}
	if config.RateLimitPerMinute > 0 {
		if config.TrustProxyHeaders {
			log.Printf("🔗 Trusting X-Forwarded-For for rate limiting (make sure a reverse proxy actually sets it)")
		} else {
			log.Printf("🔗 Rate limiting by direct connection IP (set TRUST_PROXY_HEADERS=true if behind a reverse proxy)")
		}
	}
	warnOnOpenDefaults()
	if config.GitHubToken != "" {
		log.Printf("🐙 GitHub token set: requests to %s are authenticated", githubAPIHost)
	}
	setupCache()
	if responseCache != nil {
		log.Printf("🗄️  Response cache: %v (%v for 404s), served up to %v stale on upstream errors, %d MB (%d KB per answer)",
			config.CacheTTL, config.CacheNegativeTTL, config.CacheMaxStale, config.CacheMaxBytes/(1024*1024), config.CacheMaxEntryBytes/1024)
	}

	if err := http.ListenAndServe(":"+config.Port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func loadConfig() {
	config = Config{
		Port:                 getEnv("PORT", "8080"),
		MaxRequestSize:       getEnvInt64("MAX_REQUEST_SIZE", 10*1024*1024), // 10MB default
		RequestTimeout:       getEnvDuration("REQUEST_TIMEOUT", 30*time.Second),
		MaxRedirects:         getEnvInt("MAX_REDIRECTS", 10),
		AllowedOrigins:       getEnvList("ALLOWED_ORIGINS", "*"),
		BlockedHosts:         getEnvList("BLOCKED_HOSTS", ""),
		AllowedHosts:         getEnvList("ALLOWED_HOSTS", ""),
		EnableVerboseLog:     getEnvBool("VERBOSE_LOGGING", false),
		AllowPrivateNetworks: getEnvBool("ALLOW_PRIVATE_NETWORKS", false),
		RateLimitPerMinute:   getEnvInt("RATE_LIMIT_PER_MINUTE", 0), // 0 = disabled
		RequireAPIKey:        getEnvBool("REQUIRE_API_KEY", false),
		APIKeys:              getEnvList("API_KEYS", ""),
		DailyRequestLimit:    getEnvInt("DAILY_REQUEST_LIMIT", 0), // 0 = disabled
		TrustProxyHeaders:    getEnvBool("TRUST_PROXY_HEADERS", false),
		CacheTTL:             getEnvDuration("CACHE_TTL", 0), // 0 = disabled
		CacheNegativeTTL:     getEnvDuration("CACHE_NEGATIVE_TTL", 5*time.Minute),
		CacheMaxStale:        getEnvDuration("CACHE_MAX_STALE", 24*time.Hour),
		CacheMaxBytes:        getEnvInt64("CACHE_MAX_BYTES", 64*1024*1024),      // 64MB
		CacheMaxEntryBytes:   getEnvInt64("CACHE_MAX_ENTRY_BYTES", 2*1024*1024), // 2MB
		GitHubToken:          getEnv("GITHUB_TOKEN", ""),
		AllowedURLs:          getEnvList("ALLOWED_URLS", ""),
		URLAllowlistMode:     getEnv("URL_ALLOWLIST_MODE", urlModeEnforce),
		LogUnlistedURLs:      getEnvBool("LOG_UNLISTED_URLS", true),
		ClientVersionHeader:  getEnv("CLIENT_VERSION_HEADER", ""),
	}
}

// warnOnOpenDefaults logs a single startup warning listing which safety
// nets are off, so a wide-open deployment isn't silent about it.
func warnOnOpenDefaults() {
	var open []string
	if config.RateLimitPerMinute == 0 {
		open = append(open, "no per-IP rate limit (RATE_LIMIT_PER_MINUTE=0)")
	}
	if len(config.AllowedHosts) == 0 {
		open = append(open, "no destination allowlist (ALLOWED_HOSTS empty, any public host can be proxied)")
	}
	if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
		open = append(open, "any origin can call it (ALLOWED_ORIGINS=*)")
	}
	if !config.RequireAPIKey {
		open = append(open, "no API key required (REQUIRE_API_KEY=false)")
	}
	if len(open) == 0 {
		return
	}
	log.Printf("⚠️  Running with open defaults: %s. Fine for local dev; see README's Production checklist before exposing this publicly.", strings.Join(open, "; "))
}

// isValidAPIKey reports whether the request is authorized. Always true
// when RequireAPIKey is off (the zero-config default).
func isValidAPIKey(r *http.Request) bool {
	if !config.RequireAPIKey {
		return true
	}
	provided := r.Header.Get("X-API-Key")
	if provided == "" {
		provided = r.URL.Query().Get("apikey")
	}
	if provided == "" {
		return false
	}
	for _, key := range config.APIKeys {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) == 1 {
			return true
		}
	}
	return false
}

// checkDailyLimit enforces a global (all clients combined) request ceiling
// over a rolling 24h window, as a cost/abuse kill switch independent of the
// per-IP rate limiter. Always true when DailyRequestLimit is 0.
func checkDailyLimit() bool {
	if config.DailyRequestLimit <= 0 {
		return true
	}
	dailyMutex.Lock()
	defer dailyMutex.Unlock()
	now := time.Now()
	if dailyResetTime.IsZero() || now.After(dailyResetTime) {
		dailyCount = 0
		dailyResetTime = now.Add(24 * time.Hour)
	}
	if dailyCount >= int64(config.DailyRequestLimit) {
		return false
	}
	dailyCount++
	return true
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}

func getEnvList(key, defaultVal string) []string {
	val := getEnv(key, defaultVal)
	if val == "" {
		return []string{}
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, `{"status":"ok","version":"%s","timestamp":"%s"}`, version, time.Now().Format(time.RFC3339)); err != nil {
		log.Printf("Error writing health check response: %v", err)
	}
}

// allowedMethods are the methods the proxy answers: GET, and the preflight
// before one.
const allowedMethods = "GET, OPTIONS"

// Headers never forwarded to the proxy target. Go canonicalizes header
// names (textproto.CanonicalMIMEHeaderKey), so these must match that form
// exactly, not the wire form (e.g. "X-Api-Key", not "X-API-Key").
var nonForwardedHeaders = map[string]bool{
	"Host":          true,
	"Authorization": true,
	"Cookie":        true,
	"X-Api-Key":     true,
}

func corsProxyHandler(w http.ResponseWriter, r *http.Request) {
	// Determine which origin to allow based on request Origin header
	allowedOrigin := getAllowedOrigin(r)

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")

	// Add credentials header if not wildcard
	if allowedOrigin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only reading is proxied: nothing is ever sent to an upstream on a
	// caller's behalf. Refused before anything that spends a budget.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", allowedMethods)
		http.Error(w, `{"error":"Only GET requests are proxied"}`, http.StatusMethodNotAllowed)
		return
	}

	// Optional API key authentication. Checked before the daily limit so an
	// unauthenticated caller can't burn the shared global budget with
	// requests that were never going to be authorized anyway.
	if !isValidAPIKey(r) {
		http.Error(w, `{"error":"Missing or invalid API key"}`, http.StatusUnauthorized)
		return
	}

	// Global daily request ceiling (kill switch).
	if !checkDailyLimit() {
		http.Error(w, `{"error":"Daily request limit reached. Try again tomorrow."}`, http.StatusServiceUnavailable)
		return
	}

	// Rate limiting
	if config.RateLimitPerMinute > 0 {
		clientIP := getClientIP(r)
		if !checkRateLimit(clientIP) {
			http.Error(w, `{"error":"Rate limit exceeded. Please try again later."}`, http.StatusTooManyRequests)
			return
		}
	}

	// Get target URL from query parameter
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, `{"error":"Missing 'url' parameter. Usage: /?url=https://example.com"}`, http.StatusBadRequest)
		return
	}

	// Validate URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		http.Error(w, `{"error":"URL must start with http:// or https://"}`, http.StatusBadRequest)
		return
	}

	// Check allowed/blocked hosts
	if !isHostAllowed(targetURL) {
		http.Error(w, `{"error":"This host is not allowed"}`, http.StatusForbidden)
		if config.EnableVerboseLog {
			log.Printf("🚫 Blocked request to: %q", targetURL)
		}
		return
	}

	// Check the URL itself against ALLOWED_URLS. isHostAllowed has parsed it already.
	if target, _ := url.Parse(targetURL); !checkURL(r, target) {
		http.Error(w, `{"error":"This URL is not allowed"}`, http.StatusForbidden)
		return
	}

	fetch := func(extra http.Header) (*cachedResponse, error) { return fetchUpstream(r, targetURL, extra) }
	var resp *cachedResponse
	var err error
	cacheState := ""
	switch {
	case responseCache == nil:
		resp, err = fetch(nil)
	case cacheable(r):
		// The fetch is shared by every client asking for this answer, so it carries none of this
		// client's revalidation conditions, which are answered from what it gets.
		shared := withoutRevalidation(r)
		result := responseCache.get(cacheKey(targetURL, r.Header), func(extra http.Header) (*cachedResponse, error) {
			return fetchUpstream(shared, targetURL, extra)
		})
		resp, err, cacheState = result.response, result.err, result.state
	default:
		resp, err = fetch(nil)
		cacheState = cacheBypass
	}
	if err != nil {
		var upstreamErr *upstreamError
		if errors.As(err, &upstreamErr) {
			http.Error(w, upstreamErr.body, upstreamErr.status)
		} else {
			http.Error(w, `{"error":"Failed to fetch URL"}`, http.StatusBadGateway)
		}
		return
	}

	for key, values := range resp.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	// Proxied content must never run as a page on the proxy's origin
	// (phishing/XSS). fetch()/XHR callers are unaffected.
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Override CORS headers
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	if allowedOrigin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Say where the answer came from, readable by the client's script too.
	if cacheState != "" {
		w.Header().Set("X-Cache", cacheState)
		if cacheState != cacheMiss && cacheState != cacheBypass {
			w.Header().Set("Age", strconv.Itoa(int(resp.age(responseCache.now()).Seconds())))
		}
		w.Header().Add("Access-Control-Expose-Headers", "X-Cache, Age")
	}

	// Copy status code, unless the client already has this answer: then say so, without it.
	status, body := resp.status, resp.body
	if cacheState != "" && cacheState != cacheBypass && notModified(r, resp) {
		status, body = http.StatusNotModified, nil
		w.Header().Del("Content-Length")
	}
	w.WriteHeader(status)

	written, err := w.Write(body)
	if err != nil {
		log.Printf("Error writing response body: %v", err)
		return
	}

	if config.EnableVerboseLog {
		log.Printf("%s %q -> %d (%d bytes) %s client %q", r.Method, targetURL, status, written, cacheState, clientVersion(r))
	}
}

// upstreamError is a failure to get an answer from the upstream, with the response the client
// is sent for it.
type upstreamError struct {
	status int
	body   string
	err    error
}

func (e *upstreamError) Error() string { return e.err.Error() }
func (e *upstreamError) Unwrap() error { return e.err }

// errUpstreamTooLarge is an upstream answer over MAX_REQUEST_SIZE.
var errUpstreamTooLarge = errors.New("upstream response is too large")

// fetchUpstream sends the client's request to the target, with the extra headers added, and
// reads the whole answer.
func fetchUpstream(r *http.Request, targetURL string, extra http.Header) (*cachedResponse, error) {
	// A GET, with no body: the caller's body, if it sent one, is never forwarded.
	proxyReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, targetURL, http.NoBody)
	if err != nil {
		log.Printf("Error creating request: %q", err.Error())
		return nil, &upstreamError{http.StatusBadRequest, fmt.Sprintf(`{"error":"Invalid URL: %s"}`, err.Error()), err}
	}

	// Copy headers from the original request, except ones that shouldn't
	// reach an arbitrary caller-chosen target: Host (net/http sets it from
	// the target URL), Authorization/Cookie (credentials meant for us or
	// for whatever the caller had them set for, not for a third party we
	// didn't choose), and X-API-Key (if REQUIRE_API_KEY is on, this is the
	// proxy's own configured key — forwarding it would leak it to the target).
	clientHeader := http.CanonicalHeaderKey(config.ClientVersionHeader)
	for key, values := range r.Header {
		if nonForwardedHeaders[key] || key == clientHeader {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	for key, values := range extra {
		proxyReq.Header[key] = values
	}
	authenticateGitHub(proxyReq)

	// Make the request
	client := &http.Client{
		Timeout:   config.RequestTimeout,
		Transport: transport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			if !isHostAllowed(req.URL.String()) {
				return fmt.Errorf("redirect to a host that is not allowed")
			}
			if !checkURL(r, req.URL) {
				return fmt.Errorf("redirect to a URL that is not allowed")
			}
			// The token goes to GitHub's API only, wherever the redirect leads.
			authenticateGitHub(req)
			return nil
		},
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		if config.EnableVerboseLog {
			log.Printf("Error fetching URL %q: %q", targetURL, err.Error())
		}
		if errors.Is(err, errPrivateAddress) {
			return nil, &upstreamError{http.StatusForbidden, `{"error":"This host is not allowed"}`, err}
		}
		return nil, &upstreamError{http.StatusBadGateway, fmt.Sprintf(`{"error":"Failed to fetch URL: %s"}`, err.Error()), err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	// Refuse oversized responses instead of truncating them silently. The
	// Content-Length check is a fast path for responses that declare their
	// size; chunked/unknown-length responses (ContentLength == -1) have no
	// size to check up front, so read up to the cap (+1, to detect going
	// over it) before writing any status code or bytes to the client. That
	// way an oversized chunked response gets a clean 502 instead of a 200
	// that gets silently cut off mid-body.
	tooLarge := &upstreamError{http.StatusBadGateway, `{"error":"Upstream response is too large"}`, errUpstreamTooLarge}
	if resp.ContentLength > config.MaxRequestSize {
		return nil, tooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxRequestSize+1))
	if err != nil {
		log.Printf("Error reading upstream response: %v", err)
		return nil, &upstreamError{http.StatusBadGateway, `{"error":"Failed to read upstream response"}`, err}
	}
	if int64(len(body)) > config.MaxRequestSize {
		return nil, tooLarge
	}

	// Keep the response headers, except cookies: with credentials allowed, an
	// upstream could otherwise set cookies on the proxy's own domain.
	header := resp.Header.Clone()
	setsCookies := len(header.Values("Set-Cookie")) > 0
	header.Del("Set-Cookie")
	return &cachedResponse{status: resp.StatusCode, header: header, body: body, setsCookies: setsCookies}, nil
}

func getClientIP(r *http.Request) string {
	// Only trust X-Forwarded-For when explicitly told to (TRUST_PROXY_HEADERS).
	// Behind a platform proxy (Railway, Render, Fly) the rightmost entry is
	// the one the platform appended; entries to its left come from the
	// client. Without a trusted proxy actually setting/overwriting it, the
	// header is just client-supplied text — trusting it unconditionally lets
	// any caller spoof a new IP per request and bypass rate limiting
	// entirely.
	if config.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			return strings.TrimSpace(ips[len(ips)-1])
		}
	}
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func checkRateLimit(clientIP string) bool {
	rateMutex.Lock()
	defer rateMutex.Unlock()

	now := time.Now()
	// Drop expired windows once the map grows, so spoofed or rotating IPs
	// can't grow it without bound.
	if len(rateLimiter) > 10000 {
		for ip, entry := range rateLimiter {
			if now.After(entry.resetTime) {
				delete(rateLimiter, ip)
			}
		}
	}
	rl, exists := rateLimiter[clientIP]

	if !exists || now.After(rl.resetTime) {
		// Create new rate limit window
		rateLimiter[clientIP] = &RateLimit{
			count:     1,
			resetTime: now.Add(time.Minute),
		}
		return true
	}

	if rl.count >= config.RateLimitPerMinute {
		return false
	}

	rl.count++
	return true
}

// hostMatches reports whether host is rule or a subdomain of it
// ("example.com" matches "api.example.com", not "example.com.evil.net").
func hostMatches(host, rule string) bool {
	rule = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rule), "."))
	return rule != "" && (host == rule || strings.HasSuffix(host, "."+rule))
}

func isHostAllowed(targetURL string) bool {
	u, err := url.Parse(targetURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname()) // strips userinfo, port, IPv6 brackets

	for _, blocked := range config.BlockedHosts {
		if hostMatches(host, blocked) {
			return false
		}
	}
	if len(config.AllowedHosts) == 0 {
		return true
	}
	for _, allowed := range config.AllowedHosts {
		if hostMatches(host, allowed) {
			return true
		}
	}
	return false
}

// isPrivateIP covers loopback, RFC 1918, link-local (incl. cloud metadata
// 169.254.169.254), CGNAT, unspecified, multicast, and IPv6 ULA/link-local.
// IPv4-mapped IPv6 is unwrapped first.
func isPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		if v4[0] == 0 || (v4[0] == 100 && v4[1]&0xc0 == 64) { // 0.0.0.0/8, 100.64.0.0/10
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

var errPrivateAddress = errors.New("destination is a private or internal address")

// safeDialer checks the address actually being connected to, after DNS
// resolution, on every connection (including after redirects). That also
// defeats DNS rebinding, which a pre-request hostname check can't.
// One shared transport for every proxied request. Building a new
// http.Transport per request (as earlier versions did) meant zero connection
// reuse and an idle-socket leak: at 200 concurrent clients the proxy held
// ~28k sockets and ~1 GB of memory. Created lazily so tests that call the
// handlers without main() still get it after loadConfig().
var (
	upstreamTransport     *http.Transport
	upstreamTransportOnce sync.Once
)

func transport() *http.Transport {
	upstreamTransportOnce.Do(func() {
		upstreamTransport = &http.Transport{
			Proxy:                 nil,
			DialContext:           safeDialer().DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: config.RequestTimeout,
			MaxIdleConns:          512,
			MaxIdleConnsPerHost:   128,
			IdleConnTimeout:       90 * time.Second,
		}
	})
	return upstreamTransport
}

func safeDialer() *net.Dialer {
	d := &net.Dialer{Timeout: 10 * time.Second}
	// Checked per dial, not at construction: the dialer lives inside the shared
	// transport, so it must follow config changes (tests flip this at runtime).
	d.Control = func(network, address string, _ syscall.RawConn) error {
		if config.AllowPrivateNetworks {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || isPrivateIP(ip) {
			return errPrivateAddress
		}
		return nil
	}
	return d
}

func getAllowedOrigin(r *http.Request) string {
	// If wildcard, allow all
	if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
		return "*"
	}

	// Get the Origin header from the request
	requestOrigin := r.Header.Get("Origin")
	if requestOrigin == "" {
		// No Origin header, return first allowed origin or *
		if len(config.AllowedOrigins) > 0 {
			return config.AllowedOrigins[0]
		}
		return "*"
	}

	// Check if the request origin is in our allowed list
	for _, allowed := range config.AllowedOrigins {
		if allowed == "*" || requestOrigin == allowed {
			return requestOrigin
		}
	}

	// If not found in allowed list, return the first allowed origin
	// This will cause CORS to fail on the browser side
	if len(config.AllowedOrigins) > 0 {
		return config.AllowedOrigins[0]
	}
	return "*"
}
