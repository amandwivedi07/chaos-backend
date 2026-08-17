// Package storage defines the file-storage port with local and S3 adapters.
package storage

import (
	"context"
	"io"
	"time"
)

// Object is one stored file, as seen by the orphan sweeper.
type Object struct {
	Key      string
	Modified time.Time
}

// Storage is the port services use to persist files.
type Storage interface {
	// Put stores the object under key and returns its public URL.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	// List enumerates stored objects under a prefix. Used by the orphan
	// sweeper to find files no card points at any more.
	List(ctx context.Context, prefix string) ([]Object, error)
}
