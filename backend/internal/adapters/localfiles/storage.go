// Package localfiles is the single-host private storage adapter.
package localfiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/files"
)

type Storage struct{ root *os.Root }

func Open(path string) (*Storage, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("FILE_STORAGE_DIR must be absolute")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("file storage directory must be private (0700)")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &Storage{root}, nil
}
func (s *Storage) Close() error { return s.root.Close() }
func (s *Storage) Put(key string, data []byte) error {
	if !security.ValidUUID(key) {
		return errors.New("invalid storage key")
	}
	f, err := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			_ = s.root.Remove(key)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Persist the directory entry before metadata can reference this file.
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	_ = dir.Close()
	if err != nil {
		return err
	}
	ok = true
	return nil
}
func (s *Storage) Read(key string) ([]byte, error) {
	if !security.ValidUUID(key) {
		return nil, errors.New("invalid storage key")
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > files.MaxBytes {
		return nil, errors.New("invalid stored file")
	}
	return io.ReadAll(io.LimitReader(f, files.MaxBytes+1))
}
