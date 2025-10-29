package rdma

import (
	"encoding/json"
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"os/exec"
)

func GetRDMADevices() ([]model.RDMADevice, error) {
	cmd := exec.Command("rdma", "dev", "show", "-j")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run rdma command: %w", err)
	}

	var devices []model.RDMADevice
	if err := json.Unmarshal(output, &devices); err != nil {
		return nil, fmt.Errorf("failed to parse rdma output: %w", err)
	}
	return devices, nil
}
