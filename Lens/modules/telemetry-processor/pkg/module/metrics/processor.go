// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
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
	newTimeseries := make([]prompb.TimeSeries, 0, len(timeseries)/4)
	newTsNames := map[string]bool{}
	for _, ts := range timeseries {
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
			newLabels := make([]prompb.Label, 0, len(ts.Labels)+4)
			for i := range ts.Labels {
				label := ts.Labels[i]
				if label.Name == "__name__" {
					label.Value = fmt.Sprintf("workload_%s", label.Value)
					newTsNames[label.Value] = true
				}
				if label.Name == "job" {
					label.Value = "primus-lens-telemetry-processor"
				}
				newLabels = append(newLabels, prompb.Label{Name: label.Name, Value: label.Value})
			}
			newLabels = append(newLabels,
				prompb.Label{Name: "pod_name", Value: podName},
				prompb.Label{Name: "pod_uid", Value: podUid},
				prompb.Label{Name: "workload_name", Value: workloadName},
				prompb.Label{Name: "workload_uid", Value: workloadUid},
			)

			newSamples := make([]prompb.Sample, 0, len(ts.Samples))
			filteredSampleCount := 0
			for _, sample := range ts.Samples {
				if sample.Value < 0 {
					filteredSampleCount++
					continue
				}
				newSamples = append(newSamples, prompb.Sample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				})
			}

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

			newTimeseries = append(newTimeseries, prompb.TimeSeries{
				Labels:  newLabels,
				Samples: newSamples,
			})

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
	if len(newTimeseries) == 0 {
		return nil
	}

	RecordActiveMetrics(newTsNames, len(newTimeseries))

	w := getBatchWriter()
	if w == nil {
		log.Errorf("BatchWriter not initialized, dropping %d relabeled series", len(newTimeseries))
		return nil
	}
	if err := w.WriteBatch(newTimeseries); err != nil {
		log.Errorf("BatchWriter failed for %d series: %v", len(newTimeseries), err)
	}
	return nil
}
