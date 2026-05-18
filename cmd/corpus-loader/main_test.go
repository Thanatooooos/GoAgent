package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarkdownCorpusCollectsRecursiveMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "guide"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# root"), 0o600); err != nil {
		t.Fatalf("write root markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide", "intro.markdown"), []byte("# intro"), 0o600); err != nil {
		t.Fatalf("write nested markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide", "notes.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	files, err := loadMarkdownCorpus(root)
	if err != nil {
		t.Fatalf("loadMarkdownCorpus: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 markdown files, got %d", len(files))
	}
	if files[0].RelativePath != "README.md" {
		t.Fatalf("unexpected first relative path: %q", files[0].RelativePath)
	}
	if files[1].RelativePath != "guide/intro.markdown" {
		t.Fatalf("unexpected second relative path: %q", files[1].RelativePath)
	}
	if files[1].DocumentName != "guide__intro.markdown" {
		t.Fatalf("unexpected document name: %q", files[1].DocumentName)
	}
}

func TestNormalizeLoaderChunkStrategy(t *testing.T) {
	cases := map[string]string{
		"":                "markdown",
		"markdown":        "markdown",
		"structure_aware": "markdown",
		"fixed_size":      "fixed_size",
	}
	for input, want := range cases {
		got, err := normalizeLoaderChunkStrategy(input)
		if err != nil {
			t.Fatalf("normalizeLoaderChunkStrategy(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeLoaderChunkStrategy(%q)=%q want %q", input, got, want)
		}
	}
	if _, err := normalizeLoaderChunkStrategy("bad"); err == nil {
		t.Fatal("expected invalid chunk strategy to fail")
	}
}

func TestDefaultManifestPath(t *testing.T) {
	got := filepath.ToSlash(defaultManifestPath(filepath.Join("tmp", "repo docs"), "markdown"))
	want := "testdata/repo_docs_markdown_manifest.json"
	if got != want {
		t.Fatalf("defaultManifestPath mismatch: got %q want %q", got, want)
	}
}
