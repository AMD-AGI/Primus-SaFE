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
	TaskTypeActiveDetection    = "active_detection"
	TaskTypeMetadataCollection = "metadata_collection"
	TaskTypeTensorBoardStream  = "tensorboard_stream"
	TaskTypeMetricCollection   = "metric_collection"
	TaskTypeLogCollection      = "log_collection"
	TaskTypeCheckpointMonitor  = "checkpoint_monitor"
	TaskTypeProfilerCollection = "profiler_collection"

	// Detection coordinator and sub-tasks
	TaskTypeDetectionCoordinator = "detection_coordinator"
	TaskTypeProcessProbe         = "detection_process_probe"
	TaskTypeLogDetection         = "detection_log_scan"
	TaskTypeImageProbe           = "detection_image_probe"
	TaskTypeLabelProbe           = "detection_label_probe"

	// Py-spy profiling task (executed by node-exporter on target node, dispatched by jobs module)
	TaskTypePySpySample = "pyspy_sample"
)

// Detection coverage source constants
const (
	DetectionSourceProcess = "process"
	DetectionSourceLog     = "log"
	DetectionSourceImage   = "image"
	DetectionSourceLabel   = "label"
	DetectionSourceWandb   = "wandb"
	DetectionSourceImport  = "import"
)

// Detection coverage status constants
const (
	DetectionStatusPending       = "pending"
	DetectionStatusCollecting    = "collecting"
	DetectionStatusCollected     = "collected"
	DetectionStatusFailed        = "failed"
	DetectionStatusNotApplicable = "not_applicable"
)

// Coordinator state constants
const (
	CoordinatorStateInit      = "init"
	CoordinatorStateWaiting   = "waiting"
	CoordinatorStateProbing   = "probing"
	CoordinatorStateAnalyzing = "analyzing"
	CoordinatorStateConfirmed = "confirmed"
	CoordinatorStateCompleted = "completed"
)

