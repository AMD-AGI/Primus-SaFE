package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	dbName  = flag.String("dbName", "primus_lens", "The name of the database")
	dbUser  = flag.String("dbUser", "postgres", "The user of the database")
	dbPass  = flag.String("dbPass", "", "The password of the database")
	dbHost  = flag.String("dbHost", "localhost", "The host of the database")
	dbPort  = flag.String("dbPort", "5432", "The port of the database")
	sslMode = flag.String("sslMode", "disable", "The ssl mode of the database")
)

func main() {
	flag.Parse()

	fmt.Println("📊 GPU Usage Weekly Report - 保存到数据库")
	fmt.Println("==========================================")
	fmt.Println()

	// 文件路径相对于 jobs 目录
	baseDir := ""

	// 1. 读取 report_data.json
	inputPath := filepath.Join(baseDir, "report_data.json")
	fmt.Printf("📖 读取 %s...\n", inputPath)
	jsonData, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("❌ 读取 report_data.json 失败: %v\n", err)
		os.Exit(1)
	}

	var reportData gpu_usage_weekly_report.ReportData
	err = json.Unmarshal(jsonData, &reportData)
	if err != nil {
		fmt.Printf("❌ 解析 JSON 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ JSON 解析成功 - 集群: %s\n", reportData.ClusterName)

	// 2. 读取 report_output.html
	htmlPath := filepath.Join(baseDir, "report_output.html")
	fmt.Printf("📄 读取 %s...\n", htmlPath)
	htmlContent, err := os.ReadFile(htmlPath)
	if err != nil {
		fmt.Printf("❌ 读取 HTML 文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML 读取成功 - 大小: %d bytes\n", len(htmlContent))

	// 3. 读取 report_output.pdf (可选)
	var pdfContent []byte
	pdfPath := filepath.Join(baseDir, "report_output.pdf")
	fmt.Printf("📄 读取 %s...\n", pdfPath)
	pdfContent, err = os.ReadFile(pdfPath)
	if err != nil {
		fmt.Printf("⚠️  PDF 文件不存在或读取失败: %v\n", err)
		fmt.Println("   将继续保存，但不包含 PDF 内容")
		pdfContent = nil
	} else {
		fmt.Printf("✅ PDF 读取成功 - 大小: %d bytes\n", len(pdfContent))
	}

	// 4. 初始化数据库连接
	fmt.Println("\n💾 连接数据库...")
	fmt.Printf("   - Host: %s:%s\n", *dbHost, *dbPort)
	fmt.Printf("   - Database: %s\n", *dbName)
	fmt.Printf("   - User: %s\n", *dbUser)

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		*dbHost, *dbPort, *dbUser, *dbName, *dbPass, *sslMode)

	db, err := gorm.Open(postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})

	if err != nil {
		fmt.Printf("❌ 数据库连接失败: %v\n", err)
		fmt.Println("\n💡 提示: 请检查数据库参数是否正确")
		fmt.Println("   使用方法: go run main.go -dbHost=localhost -dbPort=5432 -dbUser=postgres -dbPass=yourpass -dbName=primus_lens")
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功")

	// 5. 准备数据库记录
	fmt.Println("\n📝 准备数据库记录...")

	// 生成唯一 ID
	reportID := generateReportID(reportData.ClusterName)

	// 解析时间范围
	var periodStart, periodEnd time.Time
	if reportData.Metadata != nil {
		if params, ok := reportData.Metadata["parameters"].(map[string]interface{}); ok {
			if startTimeStr, ok := params["start_time"].(string); ok {
				periodStart, _ = time.Parse(time.RFC3339, startTimeStr)
			}
			if endTimeStr, ok := params["end_time"].(string); ok {
				periodEnd, _ = time.Parse(time.RFC3339, endTimeStr)
			}
		}
	}

	// 如果从 metadata 中无法获取，使用 Period 字段
	if periodStart.IsZero() {
		periodStart = reportData.Period.StartTime
	}
	if periodEnd.IsZero() {
		periodEnd = reportData.Period.EndTime
	}

	// 如果仍然是零值，使用当前时间的前7天到现在
	if periodStart.IsZero() {
		periodEnd = time.Now()
		periodStart = periodEnd.AddDate(0, 0, -7)
	}

	// 准备 json_content
	jsonContent := reportData.ToExtType()

	// 准备 metadata
	metadata := reportData.GenerateMetadata()

	// 创建数据库记录
	record := &dbmodel.GpuUsageWeeklyReports{
		ID:           reportID,
		ClusterName:  reportData.ClusterName,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
		GeneratedAt:  time.Now(),
		Status:       "generated",
		HTMLContent:  htmlContent,
		PdfContent:   pdfContent,
		JSONContent:  jsonContent,
		Metadata:     metadata,
		ErrorMessage: "",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	fmt.Printf("   - 报告 ID: %s\n", reportID)
	fmt.Printf("   - 集群名称: %s\n", record.ClusterName)
	fmt.Printf("   - 周期: %s 到 %s\n",
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))
	fmt.Printf("   - 状态: %s\n", record.Status)

	// 6. 保存到数据库
	fmt.Println("\n💾 保存到数据库...")

	// 检查是否已存在相同 ID 的记录
	var existingRecord dbmodel.GpuUsageWeeklyReports
	result := db.Where("id = ?", reportID).First(&existingRecord)

	if result.Error == nil {
		// 记录已存在，询问是否覆盖
		fmt.Printf("⚠️  警告: ID 为 %s 的报告已存在\n", reportID)
		fmt.Println("   是否覆盖现有记录? (y/n)")

		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Println("❌ 操作已取消")
			os.Exit(0)
		}

		// 更新现有记录
		result = db.Model(&existingRecord).Updates(record)
		if result.Error != nil {
			fmt.Printf("❌ 更新记录失败: %v\n", result.Error)
			os.Exit(1)
		}
		fmt.Println("✅ 记录更新成功")
	} else if result.Error == gorm.ErrRecordNotFound {
		// 记录不存在，创建新记录
		result = db.Create(record)
		if result.Error != nil {
			fmt.Printf("❌ 创建记录失败: %v\n", result.Error)
			os.Exit(1)
		}
		fmt.Println("✅ 记录创建成功")
	} else {
		fmt.Printf("❌ 数据库查询失败: %v\n", result.Error)
		os.Exit(1)
	}

	// 7. 显示摘要
	fmt.Println("\n✨ 保存完成！")
	fmt.Println("\n📊 报告摘要:")
	fmt.Printf("   - 报告 ID: %s\n", reportID)
	fmt.Printf("   - 集群名称: %s\n", record.ClusterName)
	fmt.Printf("   - HTML 大小: %d bytes\n", len(htmlContent))
	if len(pdfContent) > 0 {
		fmt.Printf("   - PDF 大小: %d bytes\n", len(pdfContent))
	}

	if reportData.Summary != nil {
		fmt.Println("\n📈 统计数据:")
		fmt.Printf("   - Total GPUs: %d\n", reportData.Summary.TotalGPUs)
		fmt.Printf("   - Avg Utilization: %.2f%%\n", reportData.Summary.AvgUtilization)
		fmt.Printf("   - Avg Allocation: %.2f%%\n", reportData.Summary.AvgAllocation)
		fmt.Printf("   - Low Util Users: %d\n", reportData.Summary.LowUtilCount)
		fmt.Printf("   - Wasted GPU Days: %.1f\n", reportData.Summary.WastedGpuDays)
	}

	fmt.Println("\n💡 可以通过以下方式查询报告:")
	fmt.Printf("   SELECT * FROM gpu_usage_weekly_reports WHERE id = '%s';\n", reportID)
}

// generateReportID 生成报告的唯一标识符
// 格式: rpt_YYYYMMDD_clustername_uuid
func generateReportID(clusterName string) string {
	now := time.Now()
	dateStr := now.Format("20060102")
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("rpt_%s_%s_%s", dateStr, clusterName, shortUUID)
}
