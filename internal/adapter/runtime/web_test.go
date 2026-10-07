package runtimeadapter

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestWebFetcherRejectsPrivateURL(t *testing.T) {
	fetcher := NewWebFetcher(&http.Client{})
	pages, err := fetcher.Fetch(context.Background(), []string{"http://127.0.0.1:9090/private"})
	if err != nil || len(pages) != 1 || !strings.Contains(pages[0].Error, "non-public") {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
}

func TestWebFetcherRejectsRedirectToPrivateURL(t *testing.T) {
	fetcher := NewWebFetcher(&http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"http://localhost/private"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	fetcher.lookupIPs = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	pages, err := fetcher.Fetch(context.Background(), []string{"https://public.example/start"})
	if err != nil || len(pages) != 1 || !strings.Contains(pages[0].Error, "non-public") {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
}

func TestWebFetcherRejectsRedirectOutsideTaskDomain(t *testing.T) {
	requests := 0
	fetcher := NewWebFetcher(&http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example/page"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	fetcher.lookupIPs = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	pages, err := fetcher.FetchScoped(context.Background(), []string{"https://sports.example/start"}, []string{"sports.example"})
	if err != nil || requests != 1 || len(pages) != 1 || !strings.Contains(pages[0].Error, "outside task scope") {
		t.Fatalf("pages=%+v requests=%d err=%v", pages, requests, err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return r(request) }
