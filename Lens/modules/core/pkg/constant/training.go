package constant

const (
	TrainingEventStartTrain = "StartTrain"
	TrainingPerformance     = "Performance"
)

// Training data source constants - matches database enum training_data_source
const (
	DataSourceLog        = "log"        // Parsed from application logs
	DataSourceWandB      = "wandb"      // From W&B API
	DataSourceTensorFlow = "tensorflow" // From TensorFlow/TensorBoard
)
