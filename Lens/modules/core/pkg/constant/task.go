package constant

// Task status constants
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusPaused    = "paused"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// Task type constants
const (
	TaskTypeDetection          = "detection"
	TaskTypeMetadataCollection = "metadata_collection"
	TaskTypeTensorBoardStream  = "tensorboard_stream"
	TaskTypeMetricCollection   = "metric_collection"
	TaskTypeLogCollection      = "log_collection"
	TaskTypeCheckpointMonitor  = "checkpoint_monitor"
)

