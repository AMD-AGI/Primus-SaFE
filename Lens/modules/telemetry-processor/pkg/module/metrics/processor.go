package metrics

import (
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompbmarshal"
)

func labelExist(labels []prompb.Label, labelName string) bool {
	for _, label := range labels {
		if label.Name == labelName {
			return true
		}
	}
	return false
}

func getName(labels []prompb.Label) string {
	for _, label := range labels {
		if label.Name == "__name__" {
			return label.Value
		}
	}
	return ""
}

func processTimeSeries(timeseries []prompb.TimeSeries) error {
	//log.Infof("Processing %d timeseries", len(timeseries)) TODO metrics
	newTimeseries := []prompbmarshal.TimeSeries{}
	newTsNames := map[string]bool{}
	for _, ts := range timeseries {
		podName, podUid := pods.GetPodLabelValue(ts.Labels)
		if podName == "" && podUid == "" {
			continue
		}
		workloads := pods.GetWorkloadsByPodName(podName)
		for _, workload := range workloads {
			if len(workload) < 2 {
				log.Errorf("workload cache for pod %s has less than 2 elements: %v", podName, workload)
				continue
			}
			workloadName := workload[0]
			workloadUid := workload[1]
			newLabels := []prompbmarshal.Label{}
			for i := range ts.Labels {
				label := ts.Labels[i]
				if label.Name == "__name__" {
					label.Value = fmt.Sprintf("workload_%s", label.Value)
					newTsNames[label.Value] = true
				}
				if label.Name == "job" {
					label.Value = fmt.Sprintf("primus-lens-telemetry-processor")
				}
				newLabels = append(newLabels, prompbmarshal.Label{Name: label.Name, Value: label.Value})
			}
			newSamples := []prompbmarshal.Sample{}
			for _, sample := range ts.Samples {
				newSample := prompbmarshal.Sample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				}
				if sample.Value < 0 {
					log.Warnf("Negative value found in timeseries for pod %s, workload %s: %v", podName, workloadName, sample)
					continue
				}
				newSamples = append(newSamples, newSample)
			}
			newTs := prompbmarshal.TimeSeries{
				Labels: append(newLabels, prompbmarshal.Label{Name: "pod_name", Value: podName},
					prompbmarshal.Label{Name: "pod_uid", Value: podUid},
					prompbmarshal.Label{Name: "workload_name", Value: workloadName},
					prompbmarshal.Label{Name: "workload_uid", Value: workloadUid}),
				Samples: newSamples,
			}
			newTimeseries = append(newTimeseries, newTs)
		}
	}
	if len(newTimeseries) > 0 {
		log.Infof("Pushing %d new timeseries to Prometheus", len(newTimeseries))
		log.Infof("timeseries names: %v", newTsNames)
		for i := range newTimeseries {
			ts := newTimeseries[i]
			err := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.PrometheusWrite.Push(ts)
			if err != nil {
				log.Errorf("Failed to push timeseries: %v", err)
			}
		}
	}
	return nil
}
