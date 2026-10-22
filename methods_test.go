package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProxyRefusesEveryMethodButGet(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			testConfig()
			config.AllowPrivateNetworks = true
			u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("answer")) })

			rec := proxy(method, u.URL, nil)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != "GET, OPTIONS" {
				t.Errorf("Allow = %q, want GET, OPTIONS", got)
			}
			if method != http.MethodHead && !strings.Contains(rec.Body.String(), "Only GET requests are proxied") {
				t.Errorf("body = %q, want the reason", rec.Body.String())
			}
			expectCalls(t, u, 0)
		})
	}
}

func TestProxyNeverForwardsARequestBody(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	var length int64 = -1
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		length = r.ContentLength
		_, _ = w.Write([]byte("answer"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?url="+url.QueryEscape(u.URL), strings.NewReader("a body"))
	corsProxyHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if length != 0 {
		t.Errorf("upstream request Content-Length = %d, want no body", length)
	}
}

func TestPreflightOffersOnlyGet(t *testing.T) {
	testConfig()
	rec := proxy(http.MethodOptions, "https://example.com", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want GET, OPTIONS", got)
	}
}

func TestRefusedMethodsSpendNoBudget(t *testing.T) {
	testConfig()
	config.AllowPrivateNetworks = true
	config.DailyRequestLimit = 1
	dailyCount = 0
	t.Cleanup(func() {
		config.DailyRequestLimit = 0
		dailyCount = 0
	})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("answer")) })

	proxy(http.MethodPost, u.URL, nil)
	if rec := get(u.URL); rec.Code != http.StatusOK {
		t.Errorf("GET after a refused POST: status = %d, want 200 within the daily limit", rec.Code)
	}
}
