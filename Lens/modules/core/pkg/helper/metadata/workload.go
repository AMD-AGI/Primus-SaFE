package metadata

const (
	WorkloadStatusRunning = "Running"
	WorkloadStatusDone    = "Done"
	WorkloadStatusDeleted = "Deleted"
)

var (
	workloadStatusColorMap = map[string]string{
		WorkloadStatusRunning: "green",
		WorkloadStatusDone:    "blue",
		WorkloadStatusDeleted: "gray",
	}
)

func GetWorkloadStatusColor(status string) string {
	return workloadStatusColorMap[status]
}
