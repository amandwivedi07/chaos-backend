package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local stores files on disk; the HTTP layer serves them at /uploads/*.
type Local struct {
	dir     string
	baseURL string
}

var _ Storage = (*Local)(nil)

func NewLocal(dir, baseURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	return &Local{dir: dir, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

func (l *Local) Put(_ context.Context, key string, r io.Reader, _ string) (string, error) {
	// Keys are server-generated; Clean guards against traversal regardless.
	clean := filepath.Clean("/" + key)
	path := filepath.Join(l.dir, clean)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return l.baseURL + "/uploads" + clean, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(l.dir, filepath.Clean("/"+key)))
}

// List walks the prefix directory. A missing directory is not an error: it
// just means nothing has been stored yet.
func (l *Local) List(_ context.Context, prefix string) ([]Object, error) {
	root := filepath.Join(l.dir, filepath.Clean("/"+prefix))
	var out []Object
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // vanished mid-walk; nothing to report
		}
		rel, err := filepath.Rel(l.dir, path)
		if err != nil {
			return nil
		}
		out = append(out, Object{Key: filepath.ToSlash(rel), Modified: info.ModTime()})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}
