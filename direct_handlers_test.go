package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadLocalFiles(t *testing.T) {
	logger := NewLogger(LevelDebug)
	tmpDir := t.TempDir()

	// Create a dummy file
	dummyFilePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(dummyFilePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	// Create a symlink to the dummy file
	symlinkPath := filepath.Join(tmpDir, "symlink.txt")
	if err := os.Symlink(dummyFilePath, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create a file outside the base directory
	outsideFilePath := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideFilePath, []byte("world"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	config := &Config{
		FileReadBaseDir: tmpDir,
		MaxFileSize:     1024,
	}

	ctx := context.WithValue(context.Background(), loggerKey, logger)

	t.Run("valid file", func(t *testing.T) {
		files, err := readLocalFiles(ctx, []string{"test.txt"}, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}
		if string(files[0].Content) != "hello" {
			t.Errorf("unexpected content: got %s, want hello", string(files[0].Content))
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		_, err := readLocalFiles(ctx, []string{"../test.txt"}, config)
		if err == nil {
			t.Fatal("expected an error for path traversal")
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		_, err := readLocalFiles(ctx, []string{dummyFilePath}, config)
		if err == nil {
			t.Fatal("expected an error for absolute path")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		files, err := readLocalFiles(ctx, []string{"symlink.txt"}, config)
		if err != nil {
			t.Fatalf("unexpected error for symlink: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}
		if string(files[0].Content) != "hello" {
			t.Errorf("unexpected content: got %s, want hello", string(files[0].Content))
		}
	})

	t.Run("no base dir", func(t *testing.T) {
		noBaseDirConfig := &Config{}
		_, err := readLocalFiles(ctx, []string{"test.txt"}, noBaseDirConfig)
		if err == nil {
			t.Fatal("expected an error when FileReadBaseDir is not set")
		}
	})

	t.Run("partial success", func(t *testing.T) {
		files, err := readLocalFiles(ctx, []string{"test.txt", "nonexistent.txt"}, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}
		if string(files[0].Content) != "hello" {
			t.Errorf("unexpected content: got %s, want hello", string(files[0].Content))
		}
	})
}

func TestFetchFromGitHub(t *testing.T) {
	logger := NewLogger(LevelDebug)
	config := &Config{}
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	s := &GeminiServer{config: config}

	t.Run("invalid repo url", func(t *testing.T) {
		_, errs := fetchFromGitHub(ctx, s, "invalid-url", "", []string{"file.go"})
		if len(errs) == 0 {
			t.Fatal("expected an error for invalid repo url")
		}
	})
}
