package geoip

import "context"

type Status struct {
	FilePath  string
	LoadedAt  int64
	UpdatedAt int64
	LastError string
	SHA256    string
}

type StatusUpdate struct {
	FilePath  string
	LoadedAt  int64
	UpdatedAt int64
	LastError string
	SHA256    string
}

type StatusRepository interface {
	LoadStatus(ctx context.Context) (Status, error)
	StoreStatus(ctx context.Context, update StatusUpdate) error
}
