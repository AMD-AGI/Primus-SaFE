package metrics

import (
	"fmt"
	"time"

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
		// Check if this metric needs debugging
		needDebug := shouldDebug(ts.Labels)
		metricName := getName(ts.Labels)
		labelMap := labelsToMap(ts.Labels)

		podName, podUid := pods.GetPodLabelValue(ts.Labels)
		if podName == "" && podUid == "" {
			if needDebug {
				recordDebug(DebugRecord{
					Timestamp:   time.Now(),
					MetricName:  metricName,
					Labels:      labelMap,
					PodName:     podName,
					PodUID:      podUid,
					Status:      "filtered",
					Reason:      "missing_pod_info: podName and podUid are both empty",
					SampleCount: len(ts.Samples),
				})
			}
			continue
		}

		workloads := pods.GetWorkloadsByPodName(podName)
		if len(workloads) == 0 {
			if needDebug {
				recordDebug(DebugRecord{
					Timestamp:   time.Now(),
					MetricName:  metricName,
					Labels:      labelMap,
					PodName:     podName,
					PodUID:      podUid,
					Status:      "filtered",
					Reason:      fmt.Sprintf("no_workload_found: no workload mapping found for pod %s", podName),
					SampleCount: len(ts.Samples),
				})
			}
			continue
		}

		for _, workload := range workloads {
			if len(workload) < 2 {
				log.Errorf("workload cache for pod %s has less than 2 elements: %v", podName, workload)
				if needDebug {
					recordDebug(DebugRecord{
						Timestamp:   time.Now(),
						MetricName:  metricName,
						Labels:      labelMap,
						PodName:     podName,
						PodUID:      podUid,
						Status:      "filtered",
						Reason:      fmt.Sprintf("invalid_workload_cache: workload cache has less than 2 elements: %v", workload),
						SampleCount: len(ts.Samples),
					})
				}
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
					label.Value = "primus-lens-telemetry-processor"
				}
				newLabels = append(newLabels, prompbmarshal.Label{Name: label.Name, Value: label.Value})
			}
			newSamples := []prompbmarshal.Sample{}
			filteredSampleCount := 0
			for _, sample := range ts.Samples {
				newSample := prompbmarshal.Sample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				}
				if sample.Value < 0 {
					filteredSampleCount++
					continue
				}
				newSamples = append(newSamples, newSample)
			}

			// If all samples are filtered, record and skip
			if len(newSamples) == 0 {
				if needDebug {
					recordDebug(DebugRecord{
						Timestamp:   time.Now(),
						MetricName:  metricName,
						Labels:      labelMap,
						PodName:     podName,
						PodUID:      podUid,
						Status:      "filtered",
						Reason:      fmt.Sprintf("all_samples_negative: %d samples were filtered due to negative values", filteredSampleCount),
						SampleCount: len(ts.Samples),
					})
				}
				continue
			}

			newTs := prompbmarshal.TimeSeries{
				Labels: append(newLabels, prompbmarshal.Label{Name: "pod_name", Value: podName},
					prompbmarshal.Label{Name: "pod_uid", Value: podUid},
					prompbmarshal.Label{Name: "workload_name", Value: workloadName},
					prompbmarshal.Label{Name: "workload_uid", Value: workloadUid}),
				Samples: newSamples,
			}
			newTimeseries = append(newTimeseries, newTs)

			// Record successful pass
			if needDebug {
				reason := fmt.Sprintf("passed: successfully relabeled with workload %s (uid: %s), %d samples kept",
					workloadName, workloadUid, len(newSamples))
				if filteredSampleCount > 0 {
					reason += fmt.Sprintf(", %d samples filtered due to negative values", filteredSampleCount)
				}
				recordDebug(DebugRecord{
					Timestamp:   time.Now(),
					MetricName:  metricName,
					Labels:      labelMap,
					PodName:     podName,
					PodUID:      podUid,
					Status:      "passed",
					Reason:      reason,
					SampleCount: len(ts.Samples),
				})
			}
		}
	}
	if len(newTimeseries) > 0 {
		// Record active metrics to cache, reduce log output
		RecordActiveMetrics(newTsNames, len(newTimeseries))
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
