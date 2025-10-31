package collector

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector/amdsmi"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	gpuDeviceInfo        = []model.GPUInfo{}
	driverVersion        = ""
	cardMetrics          = []model.CardMetrics{}
	cardDriDeviceMapping = map[string]model.DriDevice{}
	driCardInfoMapping   = map[string]model.GPUInfo{}
)

func GetGpuDeviceInfo() []model.GPUInfo {
	return gpuDeviceInfo
}

func GetDriCardInfoMapping() map[string]model.DriDevice {
	return cardDriDeviceMapping
}

func GetDriverVersion() string {
	return driverVersion
}

func GetCardMetrics() []model.CardMetrics {
	return cardMetrics
}

func startRefreshGPUInfo(ctx context.Context) {
	singleCycle := func() {
		err := doRefreshDriverVersion(ctx)
		if err != nil {
			log.Errorf("Failed to refresh driver version: %v", err)
		}
		err = doRefreshDeviceInfo(ctx)
		if err != nil {
			log.Errorf("Failed to refresh device info: %v", err)
			//TODO Err Metrics
		}
		err = doRefreshCardMetrics(ctx)
		if err != nil {
			log.Errorf("Failed to refresh card metrics: %v", err)
		}
	}
	singleCycle()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			singleCycle()
		}
	}()
}

func doRefreshDeviceInfo(ctx context.Context) error {
	results, err := amdsmi.RunAmdSmiAndParse()
	if err != nil {
		return err
	}
	driDevices, err := loadAndParseDriDevices(ctx)
	if err == nil {
		newCardDeviceMapping := map[string]model.DriDevice{}
		newDriCardInfoMapping := map[string]model.GPUInfo{}
		for i := range results {
			if driDevice, ok := driDevices[results[i].Bus.BDF]; ok {
				driDevice.CardId = results[i].GPU
				results[i].DriDevice = *driDevice
				newDriCardInfoMapping[driDevice.Card] = results[i]
				newCardDeviceMapping[driDevice.Card] = *driDevice
			} else {
				log.Warnf("Failed to find dri device for bus: %s", results[i].Bus.BDF)
			}
		}
		driCardInfoMapping = newDriCardInfoMapping
		cardDriDeviceMapping = newCardDeviceMapping
	} else {
		log.Errorf("Failed to parse dri devices: %v", err)
	}
	gpuDeviceInfo = results

	return nil
}

func doRefreshDriverVersion(ctx context.Context) error {
	ver, err := amdsmi.GetDriverVersion()
	if err != nil {
		return err
	}
	driverVersion = ver
	return nil
}

func doRefreshCardMetrics(ctx context.Context) error {
	metrics, err := amdsmi.GetStateInfo()
	if err != nil {
		return err
	}
	cardMetrics = metrics
	return nil
}

func loadAndParseDriDevices(ctx context.Context) (map[string]*model.DriDevice, error) {
	basePath := "/hostdev/dri/by-path"
	deviceMap := map[string]*model.DriDevice{}

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		fullLink := filepath.Join(basePath, name)
		target, err := os.Readlink(fullLink)
		if err != nil {
			log.Errorf("Failed to read symlink: %s\n", fullLink)
			return nil
		}
		target = filepath.Clean(filepath.Join(basePath, target))
		hostTarget := strings.ReplaceAll(target, "hostdev", "dev")
		// Match pci-0000:xx:xx.x-card or pci-0000:xx:xx.x-render
		re := regexp.MustCompile(`^pci-(0000:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9])-?(card|render)?`)
		matches := re.FindStringSubmatch(name)
		if len(matches) != 3 {
			return nil
		}
		pciAddr := matches[1]
		role := matches[2] // "card" or "render"
		if _, exists := deviceMap[pciAddr]; !exists {
			deviceMap[pciAddr] = &model.DriDevice{PCIAddr: pciAddr}
		}
		if strings.HasPrefix(role, "card") {
			deviceMap[pciAddr].Card = hostTarget
			deviceMap[pciAddr].CardId, _ = strconv.Atoi(strings.TrimPrefix(role, "card"))
		} else if strings.HasPrefix(role, "render") {
			deviceMap[pciAddr].Render = hostTarget
		}
		return nil
	})

	if err != nil {
		log.Error("Error walking directory:", err)
		return nil, err
	}

	return deviceMap, nil
}
