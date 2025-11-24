package main

import (
	"flag"
	"fmt"
	"os"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
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
	
	fmt.Println("📊 GPU Usage Weekly Reports - 查询数据库")
	fmt.Println("==========================================\n")

	// 初始化数据库连接
	fmt.Println("💾 连接数据库...")
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
		fmt.Println("\n💡 提示: 使用方法: go run main.go -dbHost=localhost -dbPort=5432 -dbUser=postgres -dbPass=yourpass -dbName=primus_lens")
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功\n")

	// 查询所有报告（不包含大字段）
	var reports []dbmodel.GpuUsageWeeklyReports
	result := db.Select("id, cluster_name, period_start, period_end, generated_at, status, error_message, created_at, updated_at").
		Order("generated_at DESC").
		Limit(20).
		Find(&reports)

	if result.Error != nil {
		fmt.Printf("❌ 查询失败: %v\n", result.Error)
		os.Exit(1)
	}

	if len(reports) == 0 {
		fmt.Println("📭 没有找到任何报告")
		return
	}

	fmt.Printf("📋 找到 %d 条报告记录 (显示最近 20 条):\n\n", len(reports))

	// 显示报告列表
	for i, report := range reports {
		fmt.Printf("═══════════════════════════════════════════════════════════\n")
		fmt.Printf("报告 #%d\n", i+1)
		fmt.Printf("───────────────────────────────────────────────────────────\n")
		fmt.Printf("  ID:           %s\n", report.ID)
		fmt.Printf("  集群名称:     %s\n", report.ClusterName)
		fmt.Printf("  周期开始:     %s\n", report.PeriodStart.Format("2006-01-02 15:04:05"))
		fmt.Printf("  周期结束:     %s\n", report.PeriodEnd.Format("2006-01-02 15:04:05"))
		fmt.Printf("  生成时间:     %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  状态:         %s\n", report.Status)
		
		// 显示元数据中的统计信息
		if report.Metadata != nil {
			if totalGPUs, ok := report.Metadata["total_gpus"].(float64); ok {
				fmt.Printf("  Total GPUs:   %.0f\n", totalGPUs)
			}
			if avgUtil, ok := report.Metadata["avg_utilization"].(float64); ok {
				fmt.Printf("  平均利用率:   %.2f%%\n", avgUtil)
			}
			if avgAlloc, ok := report.Metadata["avg_allocation"].(float64); ok {
				fmt.Printf("  平均分配率:   %.2f%%\n", avgAlloc)
			}
			if lowUtilCount, ok := report.Metadata["low_util_count"].(float64); ok {
				fmt.Printf("  低利用率用户: %.0f\n", lowUtilCount)
			}
		}
		
		if report.ErrorMessage != "" {
			fmt.Printf("  错误信息:     %s\n", report.ErrorMessage)
		}
		fmt.Println()
	}

	fmt.Println("═══════════════════════════════════════════════════════════\n")
	
	// 提供导出选项
	if len(reports) > 0 {
		fmt.Println("💡 提示: 要导出某个报告，请运行:")
		fmt.Printf("   cd cmd/export-report && go run main.go %s\n", reports[0].ID)
	}
}

