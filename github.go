package main

import (
	"net/http"
	"strings"
)

// GitHub allows 60 unauthenticated API requests an hour per IP address, and every client of the
// proxy shares its address. With GITHUB_TOKEN set, the proxy authenticates its requests to
// GitHub's API with the token instead, which raises the limit to 5,000 an hour.
//
// The token is the proxy's own, never a client's: clients' Authorization headers are not
// forwarded (see nonForwardedHeaders). It is sent to GitHub's API host only, and dropped from a
// redirect to any other host, such as a release download's.

// githubAPIHost is the host the token is sent to. A variable so that the tests can stand an
// httptest server in for it.
var githubAPIHost = "api.github.com"

// authenticateGitHub adds the token to a request for GitHub's API, and makes sure no request to
// another host carries it.
func authenticateGitHub(req *http.Request) {
	if config.GitHubToken != "" && strings.EqualFold(req.URL.Hostname(), githubAPIHost) {
		req.Header.Set("Authorization", "Bearer "+config.GitHubToken)
		return
	}
	req.Header.Del("Authorization")
}
