package test

import (
	"net/url"
	"reflect"
	"testing"

	aihttp "local/rag-project/internal/infra-ai/http"
)

func TestJoinURL(t *testing.T) {
	r := aihttp.NewModelUrlResolver()
	tests := []struct {
		base   string
		path   string
		expect string
	}{
		{"http://a.com", "b", "http://a.com/b"},
		{"http://a.com/", "/b", "http://a.com/b"},
		{"http://a.com/x", "y", "http://a.com/x/y"},
		{"http://a.com/x/", "/y", "http://a.com/x/y"},
	}
	for _, tt := range tests {
		got := r.JoinURL(tt.base, tt.path)
		if got != tt.expect {
			t.Errorf("JoinURL(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.expect)
		}
	}
}

func TestResolveURL(t *testing.T) {
	r := aihttp.NewModelUrlResolver()
	provider := "http://api.com"
	endpoints := map[string]string{"chat": "/v1/chat", "embed": "embed"}
	// candidateURL 优先
	url1, err := r.ResolveURL(provider, endpoints, "http://custom", "chat")
	if err != nil || url1 != "http://custom" {
		t.Errorf("ResolveURL candidateURL failed: got %v, %v", url1, err)
	}
	// provider+endpoint
	url2, err := r.ResolveURL(provider, endpoints, "", "chat")
	if err != nil || url2 != "http://api.com/v1/chat" {
		t.Errorf("ResolveURL provider+endpoint failed: got %v, %v", url2, err)
	}
	// endpoint 不存在
	_, err = r.ResolveURL(provider, endpoints, "", "notfound")
	if err == nil {
		t.Error("ResolveURL should fail for missing endpoint")
	}
}

func TestParseURL(t *testing.T) {
	r := aihttp.NewModelUrlResolver()
	u, err := r.ParseURL("http://a.com/x?y=1")
	if err != nil {
		t.Fatalf("ParseURL failed: %v", err)
	}
	if u.Host != "a.com" || u.Path != "/x" {
		t.Errorf("ParseURL got wrong host/path: %v %v", u.Host, u.Path)
	}
}

func TestBuildURL(t *testing.T) {
	r := aihttp.NewModelUrlResolver()
	params := map[string]string{"a": "1", "b": "2"}
	got, err := r.BuildURL("http://a.com", "/x", params)
	if err != nil {
		t.Fatalf("BuildURL failed: %v", err)
	}
	u, _ := url.Parse(got)
	if u.Path != "/x" {
		t.Errorf("BuildURL wrong path: %v", u.Path)
	}
	q := u.Query()
	if !reflect.DeepEqual(q.Get("a"), "1") || !reflect.DeepEqual(q.Get("b"), "2") {
		t.Errorf("BuildURL wrong query: %v", u.RawQuery)
	}
}
