package schedule_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
)

type fakeFileStorage struct {
	uploadFn func(ctx context.Context, file port.FileUpload) (port.StoredFile, error)
}

func (f fakeFileStorage) Upload(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
	if f.uploadFn != nil {
		return f.uploadFn(ctx, file)
	}
	return port.StoredFile{}, nil
}

func (f fakeFileStorage) Delete(ctx context.Context, key string) error {
	return nil
}

func (f fakeFileStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func TestRemoteFileFetcherSkipsWhenHeadETagMatches(t *testing.T) {
	t.Parallel()

	var getCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodHead:
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Last-Modified", "Tue, 28 Apr 2026 00:00:00 GMT")
		case http.MethodGet:
			getCalled = true
			_, _ = w.Write([]byte("changed"))
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
	}))
	defer server.Close()

	fetcher := schedule.NewRemoteFileFetcher(schedule.RemoteFileFetcherOptions{
		HTTPClient: server.Client(),
		TempDir:    t.TempDir(),
	})

	result, err := fetcher.FetchIfChanged(context.Background(), server.URL+"/demo.md", `"v1"`, "", "old-hash", "fallback.md")
	if err != nil {
		t.Fatalf("FetchIfChanged() error = %v", err)
	}
	if result.Changed {
		t.Fatal("FetchIfChanged() should skip unchanged remote file")
	}
	if getCalled {
		t.Fatal("GET should not be called when HEAD validator matches")
	}
	if result.ContentHash != "old-hash" || result.ETag != `"v1"` {
		t.Fatalf("unexpected skipped result: %+v", result)
	}
}

func TestRemoteFileFetcherChangedDownloadsTempFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodHead {
			http.Error(w, "head unsupported", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("Content-Disposition", `attachment; filename="remote.md"`)
		w.Header().Set("ETag", `"v2"`)
		_, _ = w.Write([]byte("hello remote"))
	}))
	defer server.Close()

	fetcher := schedule.NewRemoteFileFetcher(schedule.RemoteFileFetcherOptions{
		HTTPClient: server.Client(),
		TempDir:    t.TempDir(),
	})

	result, err := fetcher.FetchIfChanged(context.Background(), server.URL+"/ignored-name", "", "", "", "fallback.md")
	if err != nil {
		t.Fatalf("FetchIfChanged() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("FetchIfChanged() should return changed result")
	}
	if result.FileName != "remote.md" || result.ContentType != "text/markdown" || result.Size != int64(len("hello remote")) {
		t.Fatalf("unexpected changed result: %+v", result)
	}
	if _, err := os.Stat(result.TempFile); err != nil {
		t.Fatalf("expected temp file to exist before Close(): %v", err)
	}
	tempFile := result.TempFile
	if err := result.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed, stat err=%v", err)
	}
}

func TestRemoteFileFetcherSkipsWhenContentHashMatches(t *testing.T) {
	t.Parallel()

	const content = "same content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	fetcher := schedule.NewRemoteFileFetcher(schedule.RemoteFileFetcherOptions{
		HTTPClient: server.Client(),
		TempDir:    t.TempDir(),
	})

	result, err := fetcher.FetchIfChanged(context.Background(), server.URL+"/demo.txt", "", "", "a636bd7cd42060a4d07fa1bfbcc010eb7794c2ba721e1e3e4c20335a15b66eaf", "")
	if err != nil {
		t.Fatalf("FetchIfChanged() error = %v", err)
	}
	if result.Changed {
		t.Fatal("FetchIfChanged() should skip unchanged content hash")
	}
	if result.TempFile != "" {
		t.Fatalf("skipped result should not keep temp file, got %q", result.TempFile)
	}
	if result.Message != "content hash unchanged" {
		t.Fatalf("unexpected skip message: %q", result.Message)
	}
}

func TestRemoteFileFetcherEnforcesSizeLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Length", "5")
		if req.Method == http.MethodGet {
			_, _ = w.Write([]byte("12345"))
		}
	}))
	defer server.Close()

	fetcher := schedule.NewRemoteFileFetcher(schedule.RemoteFileFetcherOptions{
		HTTPClient: server.Client(),
		MaxBytes:   4,
		TempDir:    t.TempDir(),
	})

	if _, err := fetcher.FetchIfChanged(context.Background(), server.URL, "", "", "", ""); err == nil {
		t.Fatal("FetchIfChanged() should reject files larger than MaxBytes")
	}
}

func TestRemoteFileFetcherFetchAndStoreUploadsDownloadedFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodHead {
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("stored body"))
	}))
	defer server.Close()

	var uploaded string
	storage := fakeFileStorage{
		uploadFn: func(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
			data, err := io.ReadAll(file.Body)
			if err != nil {
				t.Fatalf("ReadAll(upload body) error = %v", err)
			}
			uploaded = string(data)
			return port.StoredFile{
				Key:         file.Key,
				FileName:    file.FileName,
				ContentType: file.ContentType,
				Size:        file.Size,
			}, nil
		},
	}

	fetcher := schedule.NewRemoteFileFetcher(schedule.RemoteFileFetcherOptions{
		HTTPClient: server.Client(),
		Storage:    storage,
		TempDir:    t.TempDir(),
	})

	stored, err := fetcher.FetchAndStore(context.Background(), server.URL+"/remote.txt", "object-key", "")
	if err != nil {
		t.Fatalf("FetchAndStore() error = %v", err)
	}
	if uploaded != "stored body" {
		t.Fatalf("unexpected uploaded body: %q", uploaded)
	}
	if stored.Url != "object-key" || stored.OriginFileName != "remote.txt" || stored.Size != int64(len("stored body")) {
		t.Fatalf("unexpected stored file dto: %+v", stored)
	}
}
