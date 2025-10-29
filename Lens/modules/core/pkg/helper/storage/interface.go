package storage

import "context"

type Query interface {
	Stat(ctx context.Context, name string) (float64, float64, float64, float64, error)
	Bandwidth(ctx context.Context, name string) (float64, float64, error)
}
