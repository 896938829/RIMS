// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalStorageSaveUsesOwnerOnlyTemporaryFileAndReplacesAtomically(t *testing.T) {
	baseDir := t.TempDir()
	storage, err := NewLocalStorage(baseDir, "")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	const objectKey = "documents/report.txt"
	target := filepath.Join(baseDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reader := newPausedReader([]byte("new "), []byte("content"))
	done := make(chan error, 1)
	go func() {
		done <- storage.Save(context.Background(), objectKey, reader)
	}()

	waitForSignal(t, reader.firstRead)
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("directory entries during Save = %d, want target and one temporary file", len(entries))
	}

	var temporary os.DirEntry
	for _, entry := range entries {
		if entry.Name() != filepath.Base(target) {
			temporary = entry
		}
	}
	if temporary == nil {
		t.Fatal("temporary file was not created beside target")
	}
	info, err := temporary.Info()
	if err != nil {
		t.Fatalf("temporary.Info() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("temporary file permissions = %04o, want 0600", got)
	}
	assertFileContent(t, target, "old content")

	close(reader.resume)
	if err := <-done; err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	assertFileContent(t, target, "new content")
	assertNoTemporaryFiles(t, filepath.Dir(target), filepath.Base(target))
}

func TestLocalStorageSaveCancellationStopsCopyAndCleansTemporaryFile(t *testing.T) {
	baseDir := t.TempDir()
	storage, err := NewLocalStorage(baseDir, "")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	const objectKey = "documents/report.txt"
	target := filepath.Join(baseDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	reader := newPausedReader([]byte("partial"), []byte("must not be read"))
	done := make(chan error, 1)
	go func() {
		done <- storage.Save(ctx, objectKey, reader)
	}()

	waitForSignal(t, reader.firstRead)
	cancel()
	close(reader.resume)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
	if reader.secondRead {
		t.Error("Save() read from source after context cancellation")
	}
	assertFileContent(t, target, "old content")
	assertNoTemporaryFiles(t, filepath.Dir(target), filepath.Base(target))
}

func TestLocalStorageSaveCopyFailurePreservesTargetAndCleansTemporaryFile(t *testing.T) {
	baseDir := t.TempDir()
	storage, err := NewLocalStorage(baseDir, "")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	const objectKey = "documents/report.txt"
	target := filepath.Join(baseDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	copyErr := errors.New("source read failed")
	err = storage.Save(context.Background(), objectKey, &errorReader{
		data: []byte("partial content"),
		err:  copyErr,
	})
	if !errors.Is(err, copyErr) {
		t.Fatalf("Save() error = %v, want wrapped copy error", err)
	}
	assertFileContent(t, target, "old content")
	assertNoTemporaryFiles(t, filepath.Dir(target), filepath.Base(target))
}

type pausedReader struct {
	first      []byte
	second     []byte
	firstRead  chan struct{}
	resume     chan struct{}
	readCount  int
	secondRead bool
}

func newPausedReader(first, second []byte) *pausedReader {
	return &pausedReader{
		first:     first,
		second:    second,
		firstRead: make(chan struct{}),
		resume:    make(chan struct{}),
	}
}

func (r *pausedReader) Read(p []byte) (int, error) {
	r.readCount++
	switch r.readCount {
	case 1:
		close(r.firstRead)
		<-r.resume
		return copy(p, r.first), nil
	case 2:
		r.secondRead = true
		return copy(p, r.second), nil
	default:
		return 0, io.EOF
	}
}

type errorReader struct {
	data []byte
	err  error
	done bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(p, r.data), nil
	}
	return 0, r.err
}

func waitForSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Save to start copying")
	}
}

func assertFileContent(t *testing.T, filename, want string) {
	t.Helper()
	got, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", filename, err)
	}
	if string(got) != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func assertNoTemporaryFiles(t *testing.T, dir, targetName string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != targetName {
			t.Errorf("unexpected leftover file %q", entry.Name())
		}
	}
}
