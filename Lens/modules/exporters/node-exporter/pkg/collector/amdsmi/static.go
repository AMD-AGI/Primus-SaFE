package amdsmi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

var nsenterPrefix = []string{
	"--target",
	"1",
	"--mount",
	"--uts",
	"--ipc",
	"--net",
	"--pid",
	"--",
}

func ParseGPUInfoArray(jsonData []byte) ([]model.GPUInfo, error) {
	var gpus []model.GPUInfo
	err := json.Unmarshal(jsonData, &gpus)
	if err != nil {
		return nil, err
	}
	return gpus, nil
}

func RunAmdSmi() error {
	cmd := exec.Command("amd-smi", "static")

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Fail to execute amd-smi: %v, stderr: %s", err, errBuf.String())
	}
	fmt.Println(outBuf.String())
	return nil
}

func RunAmdSmiAndParse() ([]model.GPUInfo, error) {
	cmds := []string{}
	cmds = append(cmds, nsenterPrefix...)
	cmds = append(cmds, []string{
		"amd-smi", "static", "--json",
	}...)

	cmd := exec.Command("nsenter", cmds...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("Fail to execute amd-smi: %v, stderr: %s", err, errBuf.String())
	}
	jsonBytes := outBuf.Bytes()
	actualJsonBytes := extractJSON(string(jsonBytes))
	gpus, err := ParseGPUInfoArray([]byte(actualJsonBytes))
	if err != nil {
		return nil, fmt.Errorf("fail to parse json: %v", err)
	}

	return gpus, nil
}

func extractJSON(raw string) string {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start != -1 && end != -1 && end > start {
		return raw[start : end+1]
	}
	return ""
}

func GetDriverVersion() (string, error) {
	cmds := []string{}
	cmds = append(cmds, nsenterPrefix...)
	cmds = append(cmds, []string{
		"rocm-smi", "--showdriverversion", "--json",
	}...)
	cmd := exec.Command("nsenter", cmds...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("Fail to execute amd-smi: %v, stderr: %s", err, errBuf.String())
	}
	jsonBytes := outBuf.Bytes()
	result := &model.RocmSmiDriverVersion{}
	err = json.Unmarshal(jsonBytes, result)
	if err != nil {
		return "", fmt.Errorf("fail to parse json: %v", err)
	}
	return result.System.DriverVersion, nil
}

func GetStateInfo() ([]model.CardMetrics, error) {
	cmds := []string{}
	cmds = append(cmds, nsenterPrefix...)
	cmds = append(cmds, []string{
		"rocm-smi", "-t", "-f", "-P", "-u", "--showmemuse", "-b", "--json",
	}...)
	cmd := exec.Command("nsenter", cmds...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("Fail to execute amd-smi: %v, stderr: %s", err, errBuf.String())
	}
	jsonBytes := outBuf.Bytes()
	metricsMap := map[string]model.CardMetrics{}
	err = json.Unmarshal(jsonBytes, &metricsMap)
	if err != nil {
		return nil, fmt.Errorf("fail to parse json: %v", err)
	}
	result := []model.CardMetrics{}
	for key := range metricsMap {
		metrics := metricsMap[key]
		idStr := strings.TrimPrefix(key, "card")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			log.Errorf("fail to parse card: %v", err)
		}
		metrics.Gpu = id
		result = append(result, metrics)
	}
	return result, nil
}

// GetPowerInfo 获取所有 GPU 的功耗信息
func GetPowerInfo() ([]model.GPUPowerInfo, error) {
	cmds := []string{}
	cmds = append(cmds, nsenterPrefix...)
	cmds = append(cmds, []string{
		"amd-smi", "metric", "-p", "--json",
	}...)

	cmd := exec.Command("nsenter", cmds...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("Fail to execute amd-smi static -p: %v, stderr: %s", err, errBuf.String())
	}

	jsonBytes := outBuf.Bytes()
	actualJsonBytes := extractJSON(string(jsonBytes))

	var powerInfos []model.GPUPowerInfo
	err = json.Unmarshal([]byte(actualJsonBytes), &powerInfos)
	if err != nil {
		return nil, fmt.Errorf("fail to parse power info json: %v", err)
	}

	return powerInfos, nil
}
