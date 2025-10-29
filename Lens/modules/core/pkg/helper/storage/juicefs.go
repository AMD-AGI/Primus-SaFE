package storage

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/prom"
)

type JuicefsQuery struct {
	clientSet *clientsets.StorageClientSet
}

func (j *JuicefsQuery) Stat(ctx context.Context, name string) (float64, float64, float64, float64, error) {
	storageUsageMetrics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`avg(juicefs_used_space{vol_name="%s"})`, name))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	inodesUsageMetrics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`avg(juicefs_used_inodes{vol_name="%s"})`, name))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	totalStorageMetrics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`avg(juicefs_total_space{vol_name="%s"})`, name))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	totalInodesMetrics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`avg(juicefs_total_inodes{vol_name="%s"})`, name))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	storageUsage := float64(0)
	if len(storageUsageMetrics) > 0 {
		storageUsage = float64(storageUsageMetrics[0].Value)
	}
	inodesUsage := float64(0)
	if len(inodesUsageMetrics) > 0 {
		inodesUsage = float64(inodesUsageMetrics[0].Value)
	}
	totalStorage := float64(0)
	if len(totalStorageMetrics) > 0 {
		totalStorage = float64(totalStorageMetrics[0].Value)
	}
	totalInodes := float64(0)
	if len(totalInodesMetrics) > 0 {
		totalInodes = float64(totalInodesMetrics[0].Value)
	}
	return storageUsage, inodesUsage, totalStorage, totalInodes, nil
}

func (j *JuicefsQuery) Bandwidth(ctx context.Context, name string) (float64, float64, error) {
	readMetaics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`sum(rate(juicefs_fuse_read_size_bytes_sum{vol_name="%s"}[1m]))`, name))
	if err != nil {
		return 0, 0, err
	}
	writeMetaics, err := prom.QueryInstant(ctx, j.clientSet, fmt.Sprintf(`sum(rate(juicefs_fuse_write_size_bytes_sum{vol_name="%s"}[1m]))`, name))
	if err != nil {
		return 0, 0, err
	}
	read := float64(0)
	if len(readMetaics) > 0 {
		read = float64(readMetaics[0].Value)
	}
	write := float64(0)
	if len(writeMetaics) > 0 {
		write = float64(writeMetaics[0].Value)
	}
	return read, write, nil
}
