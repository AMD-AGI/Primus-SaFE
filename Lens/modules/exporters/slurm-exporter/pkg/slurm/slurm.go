package slurm

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func QueryJobsCLI() ([]model.SlurmJob, error) {
	// Use tab separator to avoid parsing issues caused by separators in names
	// Fields: id, partition, name, user, state(short), elapsed, nodes, reason/node, submit_time, account, qos, gpu(gres)
	format := "%i\t%P\t%j\t%u\t%t\t%M\t%D\t%R\t%V\t%a\t%q\t%b"
	output, err := runCmd("squeue", "-h", "-o", format)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	jobs := make([]model.SlurmJob, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 12 {
			continue
		}
		var id uint32
		var nodes uint32
		fmt.Sscanf(parts[0], "%d", &id)
		fmt.Sscanf(parts[6], "%d", &nodes)
		gpu := parts[11]
		var gpuCount uint32
		// Parse formats like "gres/gpu:8" or "gpu:8" or multi-TRES strings with commas
		if idx := strings.Index(gpu, "gpu:"); idx >= 0 {
			// Extract substring starting from gpu:
			sub := gpu[idx+4:]
			// Truncate to next delimiter (comma or whitespace)
			for i := 0; i < len(sub); i++ {
				if sub[i] < '0' || sub[i] > '9' {
					sub = sub[:i]
					break
				}
			}
			var n uint32
			fmt.Sscanf(sub, "%d", &n)
			gpuCount = n
		}
		jobs = append(jobs, model.SlurmJob{
			ID:         id,
			Partition:  parts[1],
			Name:       parts[2],
			User:       parts[3],
			State:      parts[4],
			Elapsed:    parts[5],
			Nodes:      nodes,
			Reason:     parts[7],
			SubmitTime: parts[8],
			Account:    parts[9],
			QOS:        parts[10],
			GPU:        gpu,
			GPUCount:   gpuCount,
		})
	}
	return jobs, nil
}
func QueryNodesCLI() ([]model.SlurmNode, error) {
	//  name, state, node_count, cpu, memory, features, gres, partition, arch, extend, load, mem_used
	format := "%n\t%t\t%D\t%C\t%m\t%f\t%G\t%P\t%T\t%e\t%l\t%M"
	output, err := runCmd("sinfo", "-h", "-N", "-o", format)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	nodes := make([]model.SlurmNode, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 12 {
			continue
		}
		var nodeCount uint32
		var memory uint32
		fmt.Sscanf(parts[2], "%d", &nodeCount)
		fmt.Sscanf(parts[4], "%d", &memory)
		nodes = append(nodes, model.SlurmNode{
			Name:      parts[0],
			State:     parts[1],
			NodeCount: nodeCount,
			CPU:       parts[3],
			Memory:    memory,
			Features:  parts[5],
			GRES:      parts[6],
			Partition: parts[7],
			Arch:      parts[8],
			Extend:    parts[9],
			Load:      parts[10],
			MemUsed:   parts[11],
		})
	}
	return nodes, nil
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
