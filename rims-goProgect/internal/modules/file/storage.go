// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrObjectNotFound is returned when a storage backend cannot locate an object.
var ErrObjectNotFound = errors.New("object not found")

// Storage abstracts the file object backend. v1 ships with LocalStorage; a
// future MinIO/S3 implementation can slot in without changing callers.
type Storage interface {
	// Save writes the reader to the backend under the given object key.
	Save(ctx context.Context, objectKey string, r io.Reader) error
	// Open returns a ReadCloser for reading an existing object.
	Open(ctx context.Context, objectKey string) (io.ReadCloser, error)
	// Delete removes an object. Idempotent: missing objects are not an error.
	Delete(ctx context.Context, objectKey string) error
	// PublicURL returns the externally reachable URL for public objects.
	// For private objects the caller should route through the download handler
	// instead of calling this.
	PublicURL(objectKey string) string
}

// LocalStorage implements Storage by writing files to a base directory on the
// local filesystem. Public URLs are served via a gin.Static mount registered
// in router.go.
type LocalStorage struct {
	baseDir      string // absolute or relative directory on disk
	publicPrefix string // URL prefix for static serving, e.g. "/uploads"
}

// NewLocalStorage creates a LocalStorage instance. The base directory is
// created if it does not already exist.
func NewLocalStorage(baseDir, publicPrefix string) (*LocalStorage, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("local storage: baseDir is required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("local storage: create base dir: %w", err)
	}
	if publicPrefix == "" {
		publicPrefix = "/uploads"
	}
	return &LocalStorage{
		baseDir:      baseDir,
		publicPrefix: strings.TrimRight(publicPrefix, "/"),
	}, nil
}

// BaseDir returns the on-disk base directory so router.go can register the
// static handler without reaching into the config again.
func (s *LocalStorage) BaseDir() string { return s.baseDir }

// PublicPrefix returns the URL prefix used for serving public objects.
func (s *LocalStorage) PublicPrefix() string { return s.publicPrefix }

func (s *LocalStorage) absPath(objectKey string) (string, error) {
	// objectKey uses forward slashes. Reject any ".." segment or absolute
	// paths outright — path.Clean would otherwise absorb leading "../" at
	// the virtual root and silently write inside baseDir, which is safe but
	// surprising. Fail loudly so callers fix their key generation.
	if objectKey == "" {
		return "", fmt.Errorf("invalid object key: empty")
	}
	for _, seg := range strings.Split(objectKey, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid object key: contains ..")
		}
	}
	clean := path.Clean("/" + objectKey)
	if clean == "/" {
		return "", fmt.Errorf("invalid object key: %s", objectKey)
	}
	return filepath.Join(s.baseDir, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

// Save writes the reader to disk under baseDir/objectKey.
func (s *LocalStorage) Save(_ context.Context, objectKey string, r io.Reader) error {
	abs, err := s.absPath(objectKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("local storage: mkdir: %w", err)
	}
	f, err := os.Create(abs)
	if err != nil {
		return fmt.Errorf("local storage: create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("local storage: write: %w", err)
	}
	return nil
}

// Open returns a ReadCloser backed by the on-disk file.
func (s *LocalStorage) Open(_ context.Context, objectKey string) (io.ReadCloser, error) {
	abs, err := s.absPath(objectKey)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("local storage: open: %w", err)
	}
	return f, nil
}

// Delete removes the file from disk. Missing files are treated as success.
func (s *LocalStorage) Delete(_ context.Context, objectKey string) error {
	abs, err := s.absPath(objectKey)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local storage: delete: %w", err)
	}
	return nil
}

// PublicURL returns the static-served URL for public objects.
func (s *LocalStorage) PublicURL(objectKey string) string {
	return s.publicPrefix + "/" + strings.TrimLeft(objectKey, "/")
}
