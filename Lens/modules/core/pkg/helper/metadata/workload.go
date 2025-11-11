package metadata

const (
	WorkloadStatusRunning = "Running"
	WorkloadStatusPending = "Pending"
	WorkloadStatusDone    = "Done"
	WorkloadStatusDeleted = "Deleted"
	WorkloadStatusFailed  = "Failed"
)

var (
	workloadStatusColorMap = map[string]string{
		WorkloadStatusRunning: "green",
		WorkloadStatusDone:    "blue",
		WorkloadStatusDeleted: "gray",
		WorkloadStatusPending: "yellow",
		WorkloadStatusFailed:  "red",
	}
)

func GetWorkloadStatusColor(status string) string {
	return workloadStatusColorMap[status]
}
