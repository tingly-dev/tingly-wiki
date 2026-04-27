package storage

import (
	"context"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// ListOptions controls listing behavior
type ListOptions struct {
	// Type filters by page type
	Type *schema.PageType

	// Prefix filters by path prefix
	Prefix string

	// Limit is max results
	Limit int

	// Tags filters by tags
	Tags []string
}

// Storage defines the persistence layer
type Storage interface {
	// ReadPage reads a page from storage
	ReadPage(ctx context.Context, path string) (*schema.Page, error)

	// WritePage writes a page to storage
	WritePage(ctx context.Context, page *schema.Page) error

	// DeletePage removes a page from storage
	DeletePage(ctx context.Context, path string) error

	// ListPages lists all pages
	ListPages(ctx context.Context, opts *ListOptions) ([]*schema.Page, error)

	// ReadSource reads a raw source document
	ReadSource(ctx context.Context, id string) (*schema.Source, error)

	// WriteSource writes a raw source document
	WriteSource(ctx context.Context, source *schema.Source) error

	// Close closes the storage
	Close() error
}
