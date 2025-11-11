package rdma

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func GetRdmaClusterStat(ctx context.Context, clientSet *clientsets.StorageClientSet) (model.RdmaClusterStat, error) {
	rdmaRxMetrics, err := prom.QueryInstant(ctx, clientSet, "rate(sum(rdma_stat_rx_bytes{}[1m]))")
	if err != nil {
		return model.RdmaClusterStat{}, err
	}
	rdmaTxMetrics, err := prom.QueryInstant(ctx, clientSet, "rate(sum(rdma_stat_tx_bytes{}[1m]))")
	if err != nil {
		return model.RdmaClusterStat{}, err
	}
	rx := float64(0)
	if len(rdmaRxMetrics) > 0 {
		rx = float64(rdmaRxMetrics[0].Value)
	}
	tx := float64(0)
	if len(rdmaTxMetrics) > 0 {
		tx = float64(rdmaTxMetrics[0].Value)
	}
	return model.RdmaClusterStat{
		TotalTx: tx,
		TotalRx: rx,
	}, nil
}
