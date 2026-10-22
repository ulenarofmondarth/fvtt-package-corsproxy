package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// withGitHubToken sets GITHUB_TOKEN for one test, with the upstream standing in for
// api.github.com.
func withGitHubToken(t *testing.T, token string, upstream *httptest.Server) {
	t.Helper()
	config.GitHubToken = token
	previous := githubAPIHost
	githubAPIHost = hostnameOf(t, upstream.URL)
	t.Cleanup(func() {
		config.GitHubToken = ""
		githubAPIHost = previous
	})
}

func hostnameOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname()
}

// recordingUpstream records the Authorization header of every request it is sent.
func recordingUpstream(t *testing.T, seen *[]string) *countingUpstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("releases"))
	})
}

func TestLoadConfigGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "github_pat_example")
	loadConfig()
	t.Cleanup(func() { config.GitHubToken = "" })

	if config.GitHubToken != "github_pat_example" {
		t.Errorf("GitHubToken = %q, want the environment's", config.GitHubToken)
	}
	if githubAPIHost != "api.github.com" {
		t.Errorf("githubAPIHost = %q, want api.github.com", githubAPIHost)
	}
}

func TestGitHubTokenAuthenticatesRequestsToGitHubsAPI(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	u := recordingUpstream(t, &seen)
	withGitHubToken(t, "github_pat_example", u.Server)

	if rec := get(u.URL + "/repos/octocat/Hello-World/releases"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(seen) != 1 || seen[0] != "Bearer github_pat_example" {
		t.Errorf("Authorization = %q, want the token as a bearer token", seen)
	}
}

func TestGitHubTokenIsNotSentToOtherHosts(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	u := recordingUpstream(t, &seen)
	config.GitHubToken = "github_pat_example"
	previous := githubAPIHost
	githubAPIHost = "api.github.invalid" // the upstream is some other host
	t.Cleanup(func() {
		config.GitHubToken = ""
		githubAPIHost = previous
	})

	get(u.URL)
	if len(seen) != 1 || seen[0] != "" {
		t.Errorf("Authorization = %q, want none for a host other than GitHub's API", seen)
	}
}

func TestGitHubTokenIsNotSentWithoutOne(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	u := recordingUpstream(t, &seen)
	withGitHubToken(t, "", u.Server)

	get(u.URL)
	if len(seen) != 1 || seen[0] != "" {
		t.Errorf("Authorization = %q, want none", seen)
	}
}

func TestGitHubTokenReplacesNothingTheClientSends(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	u := recordingUpstream(t, &seen)
	withGitHubToken(t, "", u.Server)

	// A client's own Authorization is never forwarded, token or not.
	proxy(http.MethodGet, u.URL, http.Header{"Authorization": {"Bearer someone-elses"}})
	if len(seen) != 1 || seen[0] != "" {
		t.Errorf("Authorization = %q, want the client's dropped", seen)
	}
}

func TestGitHubTokenIsNotFollowedToAnotherHost(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	elsewhere := recordingUpstream(t, &seen)
	// The same server by another name: the token is for one host name, not one address.
	elsewhereURL := strings.Replace(elsewhere.URL, "127.0.0.1", "localhost", 1)
	github := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhereURL+"/download", http.StatusFound)
	})
	withGitHubToken(t, "github_pat_example", github.Server)

	if rec := get(github.URL + "/repos/octocat/Hello-World/zipball"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after the redirect", rec.Code)
	}
	if len(seen) != 1 || seen[0] != "" {
		t.Errorf("Authorization after a redirect to another host = %q, want none", seen)
	}
}

func TestGitHubTokenIsKeptOnARedirectWithinGitHubsAPI(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var seen []string
	github := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.URL.Path == "/repos/old-name/module/releases" {
			// A renamed repository answers with a redirect to its new address.
			http.Redirect(w, r, "/repositories/42/releases", http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte("releases"))
	})
	withGitHubToken(t, "github_pat_example", github.Server)

	get(github.URL + "/repos/old-name/module/releases")
	if len(seen) != 2 || seen[1] != "Bearer github_pat_example" {
		t.Errorf("Authorization = %q, want the token on both requests", seen)
	}
}

func TestCacheKeepsGitHubsAuthenticatedAnswers(t *testing.T) {
	// GitHub marks every authenticated answer `Cache-Control: private`. The token only reads
	// public repositories, and clients never send their own credentials, so the answer is the
	// same for every client and the cache keeps it.
	withCache(t, cacheOptions{ttl: time.Minute})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=60, s-maxage=60")
		_, _ = w.Write([]byte("releases"))
	})
	withGitHubToken(t, "github_pat_example", u.Server)

	expectResponse(t, get(u.URL), http.StatusOK, "releases", "MISS")
	expectResponse(t, get(u.URL), http.StatusOK, "releases", "HIT")
	expectCalls(t, u, 1)
}

// withHostsAt sends every upstream connection to the server, whatever host it is for, so the
// tests can use host names that do not resolve.
func withHostsAt(t *testing.T, server *httptest.Server) {
	t.Helper()
	address := server.Listener.Addr().String()
	saved := transport()
	upstreamTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	t.Cleanup(func() { upstreamTransport = saved })
}

func TestGitHubTokenIsNotFollowedToASubdomain(t *testing.T) {
	// Go itself keeps Authorization on a redirect to a subdomain of the original host. The token
	// is for GitHub's API host alone.
	testConfig()
	config.AllowPrivateNetworks = true
	seen := map[string]string{}
	var port int
	server := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Host] = r.Header.Get("Authorization")
		if r.URL.Path == "/upload" {
			http.Redirect(w, r, "http://uploads.api.github.test:"+strconv.Itoa(port)+"/elsewhere", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("done"))
	})
	withHostsAt(t, server.Server)
	port = server.Listener.Addr().(*net.TCPAddr).Port
	config.GitHubToken = "github_pat_example"
	previous := githubAPIHost
	githubAPIHost = "api.github.test"
	t.Cleanup(func() {
		config.GitHubToken = ""
		githubAPIHost = previous
	})

	rec := get("http://api.github.test:" + strconv.Itoa(port) + "/upload")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after the redirect", rec.Code)
	}
	if got := seen["api.github.test:"+strconv.Itoa(port)]; got != "Bearer github_pat_example" {
		t.Errorf("GitHub's API: Authorization = %q, want the token", got)
	}
	if got, ok := seen["uploads.api.github.test:"+strconv.Itoa(port)]; !ok || got != "" {
		t.Errorf("the subdomain: Authorization = %q (reached: %v), want none", got, ok)
	}
}
