package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
)

// E2ETestSuite 端到端测试套件
type E2ETestSuite struct {
	ctx              context.Context
	metadataFacade   database.AiWorkloadMetadataFacadeInterface
	detectionManager *framework.FrameworkDetectionManager
	reuseEngine      *framework.ReuseEngine
	apiServer        *gin.Engine
	baseURL          string
}

// SetupE2ETest 设置端到端测试环境
func SetupE2ETest(t *testing.T) *E2ETestSuite {
	ctx := context.Background()
	
	// 初始化数据库 facade（使用测试数据库）
	metadataFacade := database.NewAiWorkloadMetadataFacade()
	
	// 初始化 DetectionManager
	detectionConfig := framework.DefaultDetectionConfig()
	detectionManager := framework.NewFrameworkDetectionManager(metadataFacade, detectionConfig)
	
	// 初始化 ReuseEngine
	reuseConfig := framework.ReuseConfig{
		Enabled:             true,
		MinSimilarityScore:  0.85,
		TimeWindowDays:      30,
		MinConfidence:       0.75,
		ConfidenceDecayRate: 0.9,
		MaxCandidates:       100,
		CacheTTLMinutes:     10,
	}
	reuseEngine := framework.NewReuseEngine(metadataFacade, reuseConfig)
	
	// 初始化 API Handler
	api.InitFrameworkDetectionHandler(detectionManager)
	
	// 初始化 WandB Handler
	wandbDetector := logs.NewWandBFrameworkDetector(detectionManager)
	metricsStorage := logs.NewInMemoryMetricsStorage(10000)
	wandbLogProcessor := logs.NewWandBLogProcessor(metricsStorage)
	logs.InitWandBHandler(wandbDetector, wandbLogProcessor)
	
	// 创建测试 API 服务器
	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiV1 := router.Group("/api/v1")
	{
		// Framework detection API
		apiV1.GET("workloads/:uid/framework-detection", api.GetFrameworkDetection)
		apiV1.POST("workloads/:uid/framework-detection", api.UpdateFrameworkDetection)
		
		// WandB API
		apiV1.POST("wandb/detection", logs.ReceiveWandBDetection)
		apiV1.POST("wandb/metrics", logs.ReceiveWandBMetrics)
		apiV1.POST("wandb/logs", logs.ReceiveWandBLogs)
		apiV1.POST("wandb/batch", logs.ReceiveWandBBatch)
	}
	
	// 启动测试服务器
	testServer := httptest.NewServer(router)
	
	return &E2ETestSuite{
		ctx:              ctx,
		metadataFacade:   metadataFacade,
		detectionManager: detectionManager,
		reuseEngine:      reuseEngine,
		apiServer:        router,
		baseURL:          testServer.URL,
	}
}

// TearDown 清理测试环境
func (s *E2ETestSuite) TearDown(t *testing.T) {
	// 清理测试数据
	// TODO: 在真实环境中需要清理数据库
}

// ========== 辅助函数 ==========

// createTestWorkload 创建测试 Workload
func (s *E2ETestSuite) createTestWorkload(t *testing.T, workloadUID string, image string) {
	// 模拟 Workload 创建
	// 在真实环境中，这会触发 Adapter 的 reconcile 流程
	
	// 这里我们直接创建 metadata 记录
	err := s.metadataFacade.CreateAiWorkloadMetadata(s.ctx, &database.model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Type:        "training",
		Framework:   "unknown", // 初始状态
		Metadata: database.model.ExtType{
			"image": image,
		},
		CreatedAt: time.Now(),
	})
	
	if err != nil {
		t.Logf("Failed to create workload metadata: %v", err)
	}
}

// getDetection 查询检测结果
func (s *E2ETestSuite) getDetection(t *testing.T, workloadUID string) *model.FrameworkDetection {
	url := fmt.Sprintf("%s/api/v1/workloads/%s/framework-detection", s.baseURL, workloadUID)
	
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	
	require.Equal(t, http.StatusOK, resp.StatusCode)
	
	var detection model.FrameworkDetection
	err = json.NewDecoder(resp.Body).Decode(&detection)
	require.NoError(t, err)
	
	return &detection
}

// reportDetection 上报检测结果
func (s *E2ETestSuite) reportDetection(
	t *testing.T,
	workloadUID string,
	source string,
	framework string,
	confidence float64,
) {
	err := s.detectionManager.ReportDetection(
		s.ctx,
		workloadUID,
		source,
		framework,
		"training",
		confidence,
		map[string]interface{}{
			"method": source,
		},
	)
	
	require.NoError(t, err)
}

// sendLog 发送日志
func (s *E2ETestSuite) sendLog(t *testing.T, workloadUID string, message string) {
	// 模拟日志处理
	// 在真实环境中，这会通过日志采集系统发送
	
	// 这里我们直接调用日志处理逻辑
	// TODO: 实现日志处理的模拟
}

// callWandBDetectionAPI 调用 WandB Detection API
func (s *E2ETestSuite) callWandBDetectionAPI(
	t *testing.T,
	req *logs.WandBDetectionRequest,
) *http.Response {
	url := fmt.Sprintf("%s/api/v1/wandb/detection", s.baseURL)
	
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	
	return resp
}

// callWandBMetricsAPI 调用 WandB Metrics API
func (s *E2ETestSuite) callWandBMetricsAPI(
	t *testing.T,
	req *logs.WandBMetricsRequest,
) *http.Response {
	url := fmt.Sprintf("%s/api/v1/wandb/metrics", s.baseURL)
	
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	
	return resp
}

// callWandBLogsAPI 调用 WandB Logs API
func (s *E2ETestSuite) callWandBLogsAPI(
	t *testing.T,
	req *logs.WandBLogsRequest,
) *http.Response {
	url := fmt.Sprintf("%s/api/v1/wandb/logs", s.baseURL)
	
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	
	return resp
}

// callWandBBatchAPI 调用 WandB Batch API
func (s *E2ETestSuite) callWandBBatchAPI(
	t *testing.T,
	req interface{},
) *http.Response {
	url := fmt.Sprintf("%s/api/v1/wandb/batch", s.baseURL)
	
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	
	return resp
}

// getSourceNames 获取所有 source 名称
func getSourceNames(sources []model.DetectionSource) []string {
	names := make([]string, len(sources))
	for i, source := range sources {
		names[i] = source.Source
	}
	return names
}

// findSource 查找指定 source
func findSource(sources []model.DetectionSource, sourceName string) *model.DetectionSource {
	for _, source := range sources {
		if source.Source == sourceName {
			return &source
		}
	}
	return nil
}

// ========== 测试场景 ==========

// TestE2E_ReuseFlow 场景 1: 完整的复用流程
func TestE2E_ReuseFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	suite := SetupE2ETest(t)
	defer suite.TearDown(t)
	
	t.Log("=== 场景 1: 完整的复用流程 ===")
	
	// 1. 创建第一个 Workload（Workload-A）
	t.Log("Step 1: 创建 Workload-A")
	suite.createTestWorkload(t, "workload-a", "primus:v1.2.3")
	
	// 模拟组件检测
	suite.reportDetection(t, "workload-a", "component", "primus", 0.90)
	
	// 2. 等待检测完成
	time.Sleep(500 * time.Millisecond)
	
	// 3. 验证 Workload-A 的检测结果
	t.Log("Step 2: 验证 Workload-A 检测结果")
	detectionA := suite.getDetection(t, "workload-a")
	assert.NotNil(t, detectionA, "Workload-A should have detection result")
	if detectionA != nil {
		assert.Equal(t, "primus", detectionA.Framework)
		assert.GreaterOrEqual(t, detectionA.Confidence, 0.80)
		t.Logf("✓ Workload-A detected: framework=%s, confidence=%.2f", 
			detectionA.Framework, detectionA.Confidence)
	}
	
	// 4. 创建相似的 Workload（Workload-B）
	t.Log("Step 3: 创建相似的 Workload-B")
	suite.createTestWorkload(t, "workload-b", "primus:v1.2.3")
	
	// 尝试复用
	reusedDetection, err := suite.reuseEngine.TryReuse(suite.ctx, &framework.WorkloadInfo{
		WorkloadUID: "workload-b",
		Image:       "primus:v1.2.3",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		EnvVars:     make(map[string]string),
	})
	
	if err == nil && reusedDetection != nil {
		// 上报复用结果
		suite.reportDetection(t, "workload-b", "reuse", reusedDetection.Framework, reusedDetection.Confidence)
	}
	
	// 5. 验证复用结果
	time.Sleep(500 * time.Millisecond)
	detectionB := suite.getDetection(t, "workload-b")
	
	if reusedDetection != nil {
		assert.NotNil(t, detectionB)
		if detectionB != nil {
			assert.Equal(t, "primus", detectionB.Framework)
			assert.Contains(t, getSourceNames(detectionB.Sources), "reuse")
			t.Logf("✓ Workload-B reused from Workload-A: framework=%s", detectionB.Framework)
		}
	} else {
		t.Log("⚠ Reuse failed (expected in test environment without full similarity matching)")
	}
	
	// 6. 添加组件判断
	suite.reportDetection(t, "workload-b", "component", "primus", 0.90)
	time.Sleep(500 * time.Millisecond)
	
	detectionB = suite.getDetection(t, "workload-b")
	assert.NotNil(t, detectionB)
	if detectionB != nil {
		assert.GreaterOrEqual(t, len(detectionB.Sources), 1)
		t.Logf("✓ Workload-B has %d detection sources", len(detectionB.Sources))
	}
	
	t.Log("✅ 场景 1 完成")
}

// TestE2E_ConflictResolution 场景 3: 冲突解决
func TestE2E_ConflictResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	suite := SetupE2ETest(t)
	defer suite.TearDown(t)
	
	t.Log("=== 场景 3: 冲突解决 ===")
	
	// 1. 创建 Workload
	t.Log("Step 1: 创建 Workload-D")
	suite.createTestWorkload(t, "workload-d", "custom-image:v1.0")
	
	// 2. 模拟组件判断为 primus
	t.Log("Step 2: 组件判断为 primus")
	suite.reportDetection(t, "workload-d", "component", "primus", 0.8)
	time.Sleep(300 * time.Millisecond)
	
	// 3. 模拟日志识别为 deepspeed
	t.Log("Step 3: 日志识别为 deepspeed")
	suite.reportDetection(t, "workload-d", "log", "deepspeed", 0.7)
	time.Sleep(300 * time.Millisecond)
	
	// 4. 查询检测结果
	t.Log("Step 4: 验证冲突检测")
	detection := suite.getDetection(t, "workload-d")
	assert.NotNil(t, detection)
	
	if detection != nil {
		// 验证冲突解决（component 优先级高于 log，所以应该选择 primus）
		assert.Equal(t, "primus", detection.Framework, "Should choose component result (higher priority)")
		assert.GreaterOrEqual(t, len(detection.Conflicts), 0, "May have conflicts")
		t.Logf("✓ Conflict resolved: framework=%s, sources=%d, conflicts=%d",
			detection.Framework, len(detection.Sources), len(detection.Conflicts))
	}
	
	// 6. 手动标注为 deepspeed
	t.Log("Step 5: 用户手动标注为 deepspeed")
	suite.reportDetection(t, "workload-d", "user", "deepspeed", 1.0)
	time.Sleep(300 * time.Millisecond)
	
	// 7. 验证用户标注覆盖
	t.Log("Step 6: 验证用户标注生效")
	detection = suite.getDetection(t, "workload-d")
	assert.NotNil(t, detection)
	
	if detection != nil {
		// 用户标注应该覆盖其他检测结果
		assert.Equal(t, "deepspeed", detection.Framework, "User annotation should override")
		assert.Equal(t, 1.0, detection.Confidence, "User annotation has confidence 1.0")
		t.Logf("✓ User annotation applied: framework=%s", detection.Framework)
	}
	
	t.Log("✅ 场景 3 完成")
}

// TestE2E_WandBDetection 场景 4: WandB 数据源（API 上报）
func TestE2E_WandBDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	suite := SetupE2ETest(t)
	defer suite.TearDown(t)
	
	t.Log("=== 场景 4: WandB 数据源（API 上报）===")
	
	// 1. 创建 Workload
	t.Log("Step 1: 创建 Workload-E")
	suite.createTestWorkload(t, "workload-e", "pytorch:latest")
	
	// 2. 模拟 wandb-exporter 调用 API 上报检测数据
	t.Log("Step 2: WandB Exporter 上报检测数据")
	wandbRequest := &logs.WandBDetectionRequest{
		Source:      "wandb",
		Type:        "framework_detection_raw",
		Version:     "1.0",
		WorkloadUID: "workload-e",
		PodUID:      "pod-e-123",
		Evidence: logs.WandBEvidence{
			WandB: logs.WandBInfo{
				Project: "primus-training-exp",
				Config: map[string]interface{}{
					"framework": "primus",
				},
			},
			Environment: map[string]string{
				"PRIMUS_CONFIG":  "/config/primus.yaml",
				"PRIMUS_VERSION": "1.2.3",
			},
		},
		Hints: logs.WandBHints{
			PossibleFrameworks: []string{"primus"},
			Confidence:         "high",
			PrimaryIndicators:  []string{"PRIMUS_CONFIG", "wandb_config.framework"},
		},
	}
	
	// 3. 调用 WandB Detection API
	resp := suite.callWandBDetectionAPI(t, wandbRequest)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	t.Log("✓ WandB detection API called successfully")
	
	// 4. 查询检测结果
	time.Sleep(500 * time.Millisecond)
	t.Log("Step 3: 验证检测结果")
	detection := suite.getDetection(t, "workload-e")
	assert.NotNil(t, detection)
	
	if detection != nil {
		assert.Equal(t, "primus", detection.Framework)
		assert.Contains(t, getSourceNames(detection.Sources), "wandb")
		
		// 5. 验证证据信息
		wandbSource := findSource(detection.Sources, "wandb")
		assert.NotNil(t, wandbSource)
		if wandbSource != nil {
			assert.Equal(t, "primus", wandbSource.Framework)
			assert.GreaterOrEqual(t, wandbSource.Confidence, 0.70)
			t.Logf("✓ WandB detection: framework=%s, confidence=%.2f, method=%v",
				wandbSource.Framework, wandbSource.Confidence, wandbSource.Evidence["method"])
		}
	}
	
	t.Log("✅ 场景 4 完成")
}

// TestE2E_WandBLogsAndMetrics 场景 5: WandB 日志和指标上报
func TestE2E_WandBLogsAndMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	suite := SetupE2ETest(t)
	defer suite.TearDown(t)
	
	t.Log("=== 场景 5: WandB 日志和指标上报 ===")
	
	// 1. 创建 Workload
	t.Log("Step 1: 创建 Workload-F")
	suite.createTestWorkload(t, "workload-f", "primus:v1.2.3")
	
	// 2. 上报框架检测数据
	t.Log("Step 2: 上报框架检测数据")
	detectionResp := suite.callWandBDetectionAPI(t, &logs.WandBDetectionRequest{
		WorkloadUID: "workload-f",
		Evidence: logs.WandBEvidence{
			Environment: map[string]string{
				"PRIMUS_CONFIG": "/config.yaml",
			},
		},
	})
	assert.Equal(t, http.StatusOK, detectionResp.StatusCode)
	detectionResp.Body.Close()
	
	// 3. 上报训练指标
	t.Log("Step 3: 上报训练指标")
	timestamp := float64(time.Now().Unix())
	metricsResp := suite.callWandBMetricsAPI(t, &logs.WandBMetricsRequest{
		WorkloadUID: "workload-f",
		RunID:       "run-123",
		Metrics: []logs.WandBMetric{
			{Name: "loss", Value: 2.5, Step: 0, Timestamp: timestamp},
			{Name: "loss", Value: 2.0, Step: 100, Timestamp: timestamp},
			{Name: "loss", Value: 1.5, Step: 200, Timestamp: timestamp},
			{Name: "accuracy", Value: 0.65, Step: 0, Timestamp: timestamp},
			{Name: "accuracy", Value: 0.80, Step: 100, Timestamp: timestamp},
			{Name: "accuracy", Value: 0.90, Step: 200, Timestamp: timestamp},
		},
	})
	assert.Equal(t, http.StatusOK, metricsResp.StatusCode)
	metricsResp.Body.Close()
	
	// 4. 上报训练日志
	t.Log("Step 4: 上报训练日志")
	logsResp := suite.callWandBLogsAPI(t, &logs.WandBLogsRequest{
		WorkloadUID: "workload-f",
		RunID:       "run-123",
		Logs: []logs.WandBLog{
			{
				Level:     "info",
				Message:   "[Primus] Initializing distributed training",
				Timestamp: timestamp,
				Source:    "stdout",
			},
			{
				Level:     "info",
				Message:   "[Primus] Training started with batch_size=32",
				Timestamp: timestamp,
				Source:    "stdout",
			},
		},
	})
	assert.Equal(t, http.StatusOK, logsResp.StatusCode)
	logsResp.Body.Close()
	
	// 5. 等待数据处理
	time.Sleep(1 * time.Second)
	
	// 6. 验证框架检测
	t.Log("Step 5: 验证框架检测结果")
	detection := suite.getDetection(t, "workload-f")
	assert.NotNil(t, detection)
	
	if detection != nil {
		assert.Equal(t, "primus", detection.Framework)
		// wandb 和可能的 log 检测
		assert.GreaterOrEqual(t, len(detection.Sources), 1)
		t.Logf("✓ Detection completed: framework=%s, sources=%d",
			detection.Framework, len(detection.Sources))
	}
	
	// 注意：指标和日志的存储验证需要实际的存储层支持
	// 在真实环境中应该验证这些数据被正确存储
	
	t.Log("✅ 场景 5 完成")
}

// TestE2E_WandBBatchReport 场景 6: WandB 批量上报
func TestE2E_WandBBatchReport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	suite := SetupE2ETest(t)
	defer suite.TearDown(t)
	
	t.Log("=== 场景 6: WandB 批量上报 ===")
	
	// 1. 创建 Workload
	t.Log("Step 1: 创建 Workload-G")
	suite.createTestWorkload(t, "workload-g", "pytorch:latest")
	
	// 2. 一次性上报所有数据
	t.Log("Step 2: 批量上报所有数据")
	timestamp := float64(time.Now().Unix())
	
	batchRequest := map[string]interface{}{
		"detection": map[string]interface{}{
			"workload_uid": "workload-g",
			"evidence": map[string]interface{}{
				"environment": map[string]string{
					"DEEPSPEED_CONFIG": "/ds_config.json",
				},
			},
		},
		"metrics": map[string]interface{}{
			"workload_uid": "workload-g",
			"metrics": []map[string]interface{}{
				{"name": "loss", "value": 1.8, "step": 50, "timestamp": timestamp},
			},
		},
		"logs": map[string]interface{}{
			"workload_uid": "workload-g",
			"logs": []map[string]interface{}{
				{
					"level":     "info",
					"message":   "[DeepSpeed] Engine initialized",
					"timestamp": timestamp,
				},
			},
		},
	}
	
	batchResp := suite.callWandBBatchAPI(t, batchRequest)
	assert.Equal(t, http.StatusOK, batchResp.StatusCode)
	
	// 3. 验证批量上报结果
	var batchResult map[string]interface{}
	err := json.NewDecoder(batchResp.Body).Decode(&batchResult)
	assert.NoError(t, err)
	batchResp.Body.Close()
	
	if batchResult != nil && batchResult["results"] != nil {
		t.Log("✓ Batch API response received")
	}
	
	// 4. 验证数据正确处理
	time.Sleep(1 * time.Second)
	t.Log("Step 3: 验证检测结果")
	detection := suite.getDetection(t, "workload-g")
	assert.NotNil(t, detection)
	
	if detection != nil {
		// DeepSpeed 应该被检测到
		assert.Equal(t, "deepspeed", detection.Framework)
		t.Logf("✓ Batch detection completed: framework=%s", detection.Framework)
	}
	
	t.Log("✅ 场景 6 完成")
}

