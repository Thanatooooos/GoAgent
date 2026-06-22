package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSampleArrayFromWrapper(t *testing.T) {
	payload := []byte(`{"samples":[{"name":"sample-1"},{"name":"sample-2"}]}`)

	rawSamples, err := ExtractSampleArray(payload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	if len(rawSamples) != 2 {
		t.Fatalf("ExtractSampleArray() len = %d, want 2", len(rawSamples))
	}
}

func TestExtractSampleArrayFromWrapperWithUTF8BOM(t *testing.T) {
	payload := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"samples":[{"name":"sample-1"}]}`)...)

	rawSamples, err := ExtractSampleArray(payload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	if len(rawSamples) != 1 {
		t.Fatalf("ExtractSampleArray() len = %d, want 1", len(rawSamples))
	}
}

func TestExtractSampleArrayFromPlainArray(t *testing.T) {
	payload := []byte(`[{"name":"sample-1"}]`)

	rawSamples, err := ExtractSampleArray(payload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	if len(rawSamples) != 1 {
		t.Fatalf("ExtractSampleArray() len = %d, want 1", len(rawSamples))
	}
}

func TestExtractSampleArrayRejectsMalformedPayload(t *testing.T) {
	if _, err := ExtractSampleArray([]byte(`{"samples":1}`)); err == nil {
		t.Fatal("ExtractSampleArray() expected error for malformed wrapper")
	}
}

func TestLoadRawSampleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "samples.json")
	want := []byte(`{"samples":[{"name":"sample-1"}]}`)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	got, err := LoadRawSampleFile(path)
	if err != nil {
		t.Fatalf("LoadRawSampleFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("LoadRawSampleFile() = %s, want %s", got, want)
	}
}

func TestExtractSampleArrayReturnsRawJSON(t *testing.T) {
	payload := []byte(`{"samples":[{"name":"sample-1","tags":["a"]}]}`)

	rawSamples, err := ExtractSampleArray(payload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	var sample map[string]any
	if err := json.Unmarshal(rawSamples[0], &sample); err != nil {
		t.Fatalf("json.Unmarshal(rawSamples[0]) error = %v", err)
	}
	if sample["name"] != "sample-1" {
		t.Fatalf("sample[name] = %v, want sample-1", sample["name"])
	}
}
