package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceFetchReturnsReadablePages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Example</title></head>
				<body>
					<article>Go generics let you write reusable functions with type parameters.</article>
				</body>
			</html>
		`))
	}))
	defer server.Close()

	service := NewService(server.Client())
	output, err := service.Fetch(context.Background(), []string{server.URL})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if output.SuccessCount != 1 || output.FailCount != 0 {
		t.Fatalf("unexpected counters: %+v", output)
	}
	if len(output.Pages) != 1 || !strings.Contains(output.Pages[0].Text, "type parameters") {
		t.Fatalf("unexpected page output: %+v", output.Pages)
	}
	if !strings.Contains(output.CombinedText, server.URL) {
		t.Fatalf("expected combined text to include url, got %q", output.CombinedText)
	}
}

func TestServiceFetchRejectsEmptyURLs(t *testing.T) {
	service := NewService(nil)
	output, err := service.Fetch(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty urls")
	}
	if !output.Degraded || output.DegradeReason != "urls are required" {
		t.Fatalf("unexpected degraded output: %+v", output)
	}
}

func TestServiceFetchMarksPartialFailureAsDegraded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Readable page content for testing.</body></html>`))
	}))
	defer server.Close()

	service := NewService(server.Client())
	output, err := service.Fetch(context.Background(), []string{server.URL, "mailto:test@example.com"})
	if err != nil {
		t.Fatalf("Fetch() unexpected error = %v", err)
	}
	if !output.Degraded || output.FailCount != 1 || output.SuccessCount != 1 {
		t.Fatalf("unexpected partial failure output: %+v", output)
	}
}

func TestExtractTextCleansBoilerplateAndDuplicateLines(t *testing.T) {
	text := extractText(`
		<html>
			<head><title>Ignored</title></head>
			<body>
				<header>
					<nav>Home</nav>
					<nav>Privacy Policy</nav>
					<nav>Contact Us</nav>
				</header>
				<article>
					<p>Go generics let you write reusable functions with type parameters.</p>
					<p>Go generics let you write reusable functions with type parameters.</p>
					<p>Type parameters help reduce repeated code while preserving type safety.</p>
				</article>
				<footer>
					<p>All rights reserved.</p>
					<p>Accept cookies</p>
				</footer>
			</body>
		</html>
	`)

	if strings.Contains(strings.ToLower(text), "privacy policy") {
		t.Fatalf("expected boilerplate to be removed, got %q", text)
	}
	if strings.Contains(strings.ToLower(text), "all rights reserved") {
		t.Fatalf("expected footer boilerplate to be removed, got %q", text)
	}
	if strings.Count(text, "Go generics let you write reusable functions with type parameters.") != 1 {
		t.Fatalf("expected duplicate body line to be removed, got %q", text)
	}
	if !strings.Contains(text, "Type parameters help reduce repeated code while preserving type safety.") {
		t.Fatalf("expected meaningful content to remain, got %q", text)
	}
	if !strings.Contains(text, "\n\n") {
		t.Fatalf("expected paragraph separation to be preserved, got %q", text)
	}
}
