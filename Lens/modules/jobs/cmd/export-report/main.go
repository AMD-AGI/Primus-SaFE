package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("📊 GPU Usage Weekly Report - 从数据库导出")
		fmt.Println("==========================================\n")
		fmt.Println("用法: go run main.go [选项] <report_id>")
		fmt.Println("\n示例: go run main.go -dbHost=localhost -dbPass=yourpass rpt_20251125_x-flannel_abc12345")
		fmt.Println("\n选项:")
		flag.PrintDefaults()
		fmt.Println("\n💡 提示: 运行 'cd ../list-reports && go run main.go' 查看所有报告 ID")
		os.Exit(1)
	}

	reportID := args[0]

	fmt.Println("📊 GPU Usage Weekly Report - 从数据库导出")
	fmt.Println("==========================================\n")
	fmt.Printf("📋 报告 ID: %s\n\n", reportID)

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
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功")

	// 查询报告
	fmt.Println("🔍 查询报告...")
	var report dbmodel.GpuUsageWeeklyReports
	result := db.Where("id = ?", reportID).First(&report)
	if result.Error != nil {
		fmt.Printf("❌ 查询失败: %v\n", result.Error)
		fmt.Println("\n💡 提示: 使用 'cd ../list-reports && go run main.go' 查看所有可用的报告")
		os.Exit(1)
	}
	fmt.Println("✅ 报告找到")

	// 显示报告信息
	fmt.Println("\n📊 报告信息:")
	fmt.Printf("   - ID: %s\n", report.ID)
	fmt.Printf("   - 集群: %s\n", report.ClusterName)
	fmt.Printf("   - 周期: %s 到 %s\n", 
		report.PeriodStart.Format("2006-01-02"),
		report.PeriodEnd.Format("2006-01-02"))
	fmt.Printf("   - 生成时间: %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - 状态: %s\n", report.Status)

	// 创建输出目录
	outputDir := fmt.Sprintf("exported_report_%s", reportID)
	fmt.Printf("\n📁 创建输出目录: %s\n", outputDir)
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		fmt.Printf("❌ 创建目录失败: %v\n", err)
		os.Exit(1)
	}

	filesExported := 0

	// 导出 JSON 数据
	if report.JSONContent != nil {
		fmt.Println("📄 导出 JSON 数据...")
		jsonPath := filepath.Join(outputDir, "report_data.json")
		jsonBytes, err := json.MarshalIndent(report.JSONContent, "", "  ")
		if err != nil {
			fmt.Printf("⚠️  JSON 序列化失败: %v\n", err)
		} else {
			err = os.WriteFile(jsonPath, jsonBytes, 0644)
			if err != nil {
				fmt.Printf("⚠️  保存 JSON 失败: %v\n", err)
			} else {
				fmt.Printf("   ✅ %s (%d bytes)\n", jsonPath, len(jsonBytes))
				filesExported++
			}
		}
	}

	// 导出 HTML
	if len(report.HTMLContent) > 0 {
		fmt.Println("📄 导出 HTML 报告...")
		htmlPath := filepath.Join(outputDir, "report.html")
		err = os.WriteFile(htmlPath, report.HTMLContent, 0644)
		if err != nil {
			fmt.Printf("⚠️  保存 HTML 失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ %s (%d bytes)\n", htmlPath, len(report.HTMLContent))
			filesExported++
		}
	}

	// 导出 PDF
	if len(report.PdfContent) > 0 {
		fmt.Println("📄 导出 PDF 报告...")
		pdfPath := filepath.Join(outputDir, "report.pdf")
		err = os.WriteFile(pdfPath, report.PdfContent, 0644)
		if err != nil {
			fmt.Printf("⚠️  保存 PDF 失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ %s (%d bytes)\n", pdfPath, len(report.PdfContent))
			filesExported++
		}
	} else {
		fmt.Println("ℹ️  此报告没有 PDF 内容")
	}

	// 导出元数据
	if report.Metadata != nil {
		fmt.Println("📄 导出元数据...")
		metadataPath := filepath.Join(outputDir, "metadata.json")
		metadataBytes, err := json.MarshalIndent(report.Metadata, "", "  ")
		if err != nil {
			fmt.Printf("⚠️  元数据序列化失败: %v\n", err)
		} else {
			err = os.WriteFile(metadataPath, metadataBytes, 0644)
			if err != nil {
				fmt.Printf("⚠️  保存元数据失败: %v\n", err)
			} else {
				fmt.Printf("   ✅ %s (%d bytes)\n", metadataPath, len(metadataBytes))
				filesExported++
			}
		}
	}

	// 创建摘要文件
	fmt.Println("📄 创建摘要文件...")
	summaryPath := filepath.Join(outputDir, "README.txt")
	summary := fmt.Sprintf(`GPU Usage Weekly Report Export
================================

Report ID:       %s
Cluster:         %s
Period:          %s to %s
Generated:       %s
Status:          %s
Exported:        %s

Files:
------
`, 
		report.ID,
		report.ClusterName,
		report.PeriodStart.Format("2006-01-02"),
		report.PeriodEnd.Format("2006-01-02"),
		report.GeneratedAt.Format("2006-01-02 15:04:05"),
		report.Status,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if len(report.HTMLContent) > 0 {
		summary += "- report.html: HTML report\n"
	}
	if len(report.PdfContent) > 0 {
		summary += "- report.pdf: PDF report\n"
	}
	if report.JSONContent != nil {
		summary += "- report_data.json: Structured report data\n"
	}
	if report.Metadata != nil {
		summary += "- metadata.json: Report metadata and statistics\n"
	}

	err = os.WriteFile(summaryPath, []byte(summary), 0644)
	if err != nil {
		fmt.Printf("⚠️  保存摘要失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ %s\n", summaryPath)
		filesExported++
	}

	fmt.Println("\n✨ 导出完成！")
	fmt.Printf("   共导出 %d 个文件到: %s/\n", filesExported, outputDir)
	fmt.Println("\n💡 提示: 在浏览器中打开 report.html 查看报告")
}

