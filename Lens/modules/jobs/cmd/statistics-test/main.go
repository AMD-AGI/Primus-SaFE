package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
)

// Command represents the test command type
type Command string

const (
	CmdGpuAllocation    Command = "gpu-allocation"
	CmdLabelAggregation Command = "label-aggregation"
	CmdListWorkloads    Command = "list-workloads"
	CmdHelp             Command = "help"
)

// Config holds the CLI configuration
type Config struct {
	// Database configuration
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// Cluster configuration
	ClusterName string

	// Command to run
	Command string

	// Time range configuration
	Hour      string // Format: 2006-01-02T15:04
	StartTime string // Format: 2006-01-02T15:04
	EndTime   string // Format: 2006-01-02T15:04

	// Label aggregation configuration
	LabelKeys      string // Comma-separated list of label keys
	AnnotationKeys string // Comma-separated list of annotation keys
	DefaultValue   string

	// Namespace filter
	Namespace string

	// Output configuration
	OutputJSON bool
	Verbose    bool
}

func main() {
	config := parseFlags()

	if config.Command == string(CmdHelp) || config.Command == "" {
		printHelp()
		return
	}

	// Initialize database
	if err := initDatabase(config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Execute command
	switch Command(config.Command) {
	case CmdGpuAllocation:
		runGpuAllocationTest(ctx, config)
	case CmdLabelAggregation:
		runLabelAggregationTest(ctx, config)
	case CmdListWorkloads:
		runListWorkloadsTest(ctx, config)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", config.Command)
		printHelp()
		os.Exit(1)
	}
}

func parseFlags() *Config {
	config := &Config{}

	// Database flags
	flag.StringVar(&config.DBHost, "db-host", "localhost", "Database host")
	flag.IntVar(&config.DBPort, "db-port", 5432, "Database port")
	flag.StringVar(&config.DBName, "db-name", "primus_lens", "Database name")
	flag.StringVar(&config.DBUser, "db-user", "postgres", "Database user")
	flag.StringVar(&config.DBPassword, "db-password", "", "Database password")
	flag.StringVar(&config.DBSSLMode, "db-ssl-mode", "require", "Database SSL mode")

	// Cluster flags
	flag.StringVar(&config.ClusterName, "cluster", "default", "Cluster name for ClusterManager")

	// Command flags
	flag.StringVar(&config.Command, "cmd", "", "Command to run: gpu-allocation, label-aggregation, list-workloads, help")

	// Time range flags
	flag.StringVar(&config.Hour, "hour", "", "Hour to process (format: 2006-01-02T15:04)")
	flag.StringVar(&config.StartTime, "start", "", "Start time (format: 2006-01-02T15:04)")
	flag.StringVar(&config.EndTime, "end", "", "End time (format: 2006-01-02T15:04)")

	// Label aggregation flags
	flag.StringVar(&config.LabelKeys, "label-keys", "", "Comma-separated list of label keys to aggregate")
	flag.StringVar(&config.AnnotationKeys, "annotation-keys", "", "Comma-separated list of annotation keys to aggregate")
	flag.StringVar(&config.DefaultValue, "default-value", "unknown", "Default value for missing labels/annotations")

	// Filter flags
	flag.StringVar(&config.Namespace, "namespace", "", "Namespace filter (optional)")

	// Output flags
	flag.BoolVar(&config.OutputJSON, "json", false, "Output in JSON format")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")

	flag.Parse()

	return config
}

func printHelp() {
	fmt.Println(`Statistics Test CLI - Test statistics package functionality

Usage:
  statistics-test -cmd <command> [options]

Commands:
  gpu-allocation     Test GPU allocation calculation
  label-aggregation  Test label/annotation aggregation
  list-workloads     List active workloads for a time range
  help               Show this help message

Database Options:
  -db-host       Database host (default: localhost)
  -db-port       Database port (default: 5432)
  -db-name       Database name (default: primus_lens)
  -db-user       Database user (default: postgres)
  -db-password   Database password
  -db-ssl-mode   Database SSL mode (default: disable)

Cluster Options:
  -cluster       Cluster name for ClusterManager (default: default)

Time Range Options:
  -hour          Hour to process (format: 2006-01-02T15:04)
  -start         Start time (format: 2006-01-02T15:04)
  -end           End time (format: 2006-01-02T15:04)

Label Aggregation Options:
  -label-keys      Comma-separated list of label keys
  -annotation-keys Comma-separated list of annotation keys
  -default-value   Default value for missing labels (default: unknown)

Filter Options:
  -namespace     Namespace filter

Output Options:
  -json          Output in JSON format
  -verbose       Verbose output

Examples:
  # Test GPU allocation for the last hour
  statistics-test -cmd gpu-allocation -db-password mypass -hour "$(date -u +%Y-%m-%dT%H:00)"

  # Test label aggregation with specific keys
  statistics-test -cmd label-aggregation -db-password mypass \
    -annotation-keys "primus-safe.user.name" \
    -hour "2025-01-01T10:00"

  # List active workloads
  statistics-test -cmd list-workloads -db-password mypass \
    -start "2025-01-01T00:00" -end "2025-01-01T12:00"
`)
}

func initDatabase(config *Config) error {
	dbConfig := sql.DatabaseConfig{
		Host:        config.DBHost,
		Port:        config.DBPort,
		DBName:      config.DBName,
		UserName:    config.DBUser,
		Password:    config.DBPassword,
		SSLMode:     config.DBSSLMode,
		Driver:      "postgres",
		MaxIdleConn: 5,
		MaxOpenConn: 10,
	}

	db, err := sql.InitDefault(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	fmt.Printf("✓ Connected to database %s@%s:%d/%s\n", config.DBUser, config.DBHost, config.DBPort, config.DBName)

	// Initialize ClusterManager with the database connection
	clusterName := config.ClusterName
	if clusterName == "" {
		clusterName = "default"
	}

	clientSet := &clientsets.ClusterClientSet{
		ClusterName: clusterName,
		StorageClientSet: &clientsets.StorageClientSet{
			DB: db,
		},
	}
	clientsets.InitClusterManagerWithClientSet(clientSet)
	fmt.Printf("✓ Initialized ClusterManager with cluster: %s\n", clusterName)

	return nil
}

func parseTimeRange(config *Config) (time.Time, time.Time, error) {
	timeFormat := "2006-01-02T15:04"

	if config.Hour != "" {
		hour, err := time.Parse(timeFormat, config.Hour)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid hour format: %w", err)
		}
		return hour.Truncate(time.Hour), hour.Truncate(time.Hour).Add(time.Hour), nil
	}

	if config.StartTime != "" && config.EndTime != "" {
		start, err := time.Parse(timeFormat, config.StartTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time format: %w", err)
		}
		end, err := time.Parse(timeFormat, config.EndTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end time format: %w", err)
		}
		return start, end, nil
	}

	// Default: last completed hour
	now := time.Now()
	endTime := now.Truncate(time.Hour)
	startTime := endTime.Add(-time.Hour)
	return startTime, endTime, nil
}

func runGpuAllocationTest(ctx context.Context, config *Config) {
	fmt.Println("\n=== GPU Allocation Test ===")

	startTime, endTime, err := parseTimeRange(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing time range: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Time range: %s - %s\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	fmt.Printf("Cluster: %s\n", getClusterDisplayName(config.ClusterName))
	if config.Namespace != "" {
		fmt.Printf("Namespace: %s\n", config.Namespace)
	}

	calculator := statistics.NewGpuAllocationCalculator(config.ClusterName)

	var result *statistics.GpuAllocationResult
	if config.Namespace != "" {
		result, err = calculator.CalculateNamespaceGpuAllocation(ctx, config.Namespace, startTime, endTime)
	} else {
		result, err = calculator.CalculateClusterGpuAllocation(ctx, startTime, endTime)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating GPU allocation: %v\n", err)
		os.Exit(1)
	}

	if config.OutputJSON {
		outputJSON(result)
		return
	}

	fmt.Println("\n--- Results ---")
	fmt.Printf("Total Allocated GPU: %.2f\n", result.TotalAllocatedGpu)
	fmt.Printf("Workload Count: %d\n", result.WorkloadCount)
	fmt.Printf("Pod Count: %d\n", result.PodCount)

	if config.Verbose && len(result.Details) > 0 {
		fmt.Println("\n--- Workload Details ---")
		for i, detail := range result.Details {
			fmt.Printf("\n[%d] %s/%s (%s)\n", i+1, detail.Namespace, detail.WorkloadName, detail.WorkloadKind)
			fmt.Printf("    UID: %s\n", detail.WorkloadUID)
			fmt.Printf("    Allocated GPU: %.2f\n", detail.AllocatedGpu)
			fmt.Printf("    Active Duration: %.0f seconds\n", detail.ActiveDuration)
			fmt.Printf("    Pod Count: %d\n", detail.PodCount)
		}
	}
}

func runLabelAggregationTest(ctx context.Context, config *Config) {
	fmt.Println("\n=== Label Aggregation Test ===")

	// Parse label and annotation keys
	labelKeys := parseCommaSeparated(config.LabelKeys)
	annotationKeys := parseCommaSeparated(config.AnnotationKeys)

	if len(labelKeys) == 0 && len(annotationKeys) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one of -label-keys or -annotation-keys must be specified\n")
		os.Exit(1)
	}

	startTime, endTime, err := parseTimeRange(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing time range: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Time range: %s - %s\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	fmt.Printf("Cluster: %s\n", getClusterDisplayName(config.ClusterName))
	fmt.Printf("Label keys: %v\n", labelKeys)
	fmt.Printf("Annotation keys: %v\n", annotationKeys)
	fmt.Printf("Default value: %s\n", config.DefaultValue)

	aggConfig := &statistics.LabelAggregationConfig{
		LabelKeys:      labelKeys,
		AnnotationKeys: annotationKeys,
		DefaultValue:   config.DefaultValue,
	}

	calculator := statistics.NewLabelAggregationCalculator(config.ClusterName, aggConfig)
	summary, err := calculator.CalculateLabelAggregation(ctx, startTime, endTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating label aggregation: %v\n", err)
		os.Exit(1)
	}

	if config.OutputJSON {
		outputJSON(summary)
		return
	}

	fmt.Println("\n--- Results ---")
	fmt.Printf("Total Workloads: %d\n", summary.TotalWorkloads)
	fmt.Printf("Aggregation Groups: %d\n", len(summary.Results))

	if len(summary.Results) > 0 {
		fmt.Println("\n--- Aggregation Details ---")
		for key, result := range summary.Results {
			fmt.Printf("\n[%s]\n", key)
			fmt.Printf("    Type: %s\n", result.DimensionType)
			fmt.Printf("    Key: %s\n", result.DimensionKey)
			fmt.Printf("    Value: %s\n", result.DimensionValue)
			fmt.Printf("    Allocated GPU: %.2f\n", result.TotalAllocatedGpu)
			fmt.Printf("    Active Workloads: %d\n", result.ActiveWorkloadCount)

			if config.Verbose && len(result.WorkloadUIDs) > 0 {
				fmt.Printf("    Workload UIDs:\n")
				for _, uid := range result.WorkloadUIDs {
					fmt.Printf("      - %s\n", uid)
				}
			}
		}
	}
}

func runListWorkloadsTest(ctx context.Context, config *Config) {
	fmt.Println("\n=== List Workloads Test ===")

	startTime, endTime, err := parseTimeRange(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing time range: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Time range: %s - %s\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	fmt.Printf("Cluster: %s\n", getClusterDisplayName(config.ClusterName))
	if config.Namespace != "" {
		fmt.Printf("Namespace: %s\n", config.Namespace)
	}

	facade := database.GetFacadeForCluster(config.ClusterName).GetWorkload()
	workloads, err := facade.ListActiveTopLevelWorkloads(ctx, startTime, endTime, config.Namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing workloads: %v\n", err)
		os.Exit(1)
	}

	if config.OutputJSON {
		outputJSON(workloads)
		return
	}

	fmt.Println("\n--- Results ---")
	fmt.Printf("Total Workloads: %d\n", len(workloads))

	if len(workloads) > 0 {
		fmt.Println("\n--- Workload List ---")
		for i, w := range workloads {
			fmt.Printf("\n[%d] %s/%s (%s)\n", i+1, w.Namespace, w.Name, w.Kind)
			fmt.Printf("    UID: %s\n", w.UID)
			fmt.Printf("    Status: %s\n", w.Status)
			fmt.Printf("    GPU Request: %d\n", w.GpuRequest)
			fmt.Printf("    Created: %s\n", w.CreatedAt.Format(time.RFC3339))
			if !w.EndAt.IsZero() {
				fmt.Printf("    Ended: %s\n", w.EndAt.Format(time.RFC3339))
			}

			if config.Verbose {
				if len(w.Labels) > 0 {
					fmt.Printf("    Labels:\n")
					for k, v := range w.Labels {
						fmt.Printf("      %s: %v\n", k, v)
					}
				}
				if len(w.Annotations) > 0 {
					fmt.Printf("    Annotations:\n")
					for k, v := range w.Annotations {
						fmt.Printf("      %s: %v\n", k, v)
					}
				}
			}
		}
	}
}

func getClusterDisplayName(clusterName string) string {
	if clusterName == "" {
		return "default"
	}
	return clusterName
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
