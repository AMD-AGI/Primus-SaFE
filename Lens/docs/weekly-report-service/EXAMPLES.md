# 代码示例和使用场景

本文档提供周报服务的常见使用场景和代码示例。

## 1. 调用 Conductor API 生成报告

### 1.1 Go 客户端调用示例

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-resty/resty/v2"
)

// ConductorClient Conductor API 客户端
type ConductorClient struct {
	baseURL string
	client  *resty.Client
}

// ClusterReportRequest 报告请求
type ClusterReportRequest struct {
	Cluster              string     `json:"cluster"`
	TimeRangeDays        int        `json:"time_range_days"`
	StartTime            *time.Time `json:"start_time,omitempty"`
	EndTime              *time.Time `json:"end_time,omitempty"`
	UtilizationThreshold int        `json:"utilization_threshold"`
	MinGPUCount          int        `json:"min_gpu_count"`
	TopN                 int        `json:"top_n"`
}

// ClusterReportResponse 报告响应
type ClusterReportResponse struct {
	Status    string                 `json:"status"`
	Report    string                 `json:"report"`
	ChartData map[string]interface{} `json:"chart_data"`
	Metadata  map[string]interface{} `json:"metadata"`
	Error     *string                `json:"error,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

func NewConductorClient(baseURL string) *ConductorClient {
	return &ConductorClient{
		baseURL: baseURL,
		client: resty.New().
			SetBaseURL(baseURL).
			SetTimeout(5 * time.Minute).
			SetRetryCount(3),
	}
}

func (c *ConductorClient) GenerateClusterReport(ctx context.Context, req *ClusterReportRequest) (*ClusterReportResponse, error) {
	var resp ClusterReportResponse

	httpResp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&resp).
		Post("/api/v1/cluster-report")

	if err != nil {
		return nil, fmt.Errorf("failed to call API: %w", err)
	}

	if httpResp.IsError() {
		return nil, fmt.Errorf("API error: status=%d, body=%s",
			httpResp.StatusCode(), httpResp.String())
	}

	if resp.Status != "success" {
		return nil, fmt.Errorf("report generation failed: %s", *resp.Error)
	}

	return &resp, nil
}

func main() {
	// 创建客户端
	client := NewConductorClient("http://primus-conductor-api:8000")

	// 构建请求
	now := time.Now()
	endTime := now.AddDate(0, 0, -1).Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
	startTime := endTime.AddDate(0, 0, -6).Truncate(24 * time.Hour)

	req := &ClusterReportRequest{
		Cluster:              "x-flannel",
		TimeRangeDays:        7,
		StartTime:            &startTime,
		EndTime:              &endTime,
		UtilizationThreshold: 30,
		MinGPUCount:          1,
		TopN:                 20,
	}

	// 调用 API
	resp, err := client.GenerateClusterReport(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	// 打印结果
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Report Length: %d chars\n", len(resp.Report))
	fmt.Printf("Chart Data Keys: %v\n", getKeys(resp.ChartData))

	// 保存报告
	saveReport("cluster-report.md", resp.Report)
	saveChartData("chart-data.json", resp.ChartData)

	fmt.Println("Report generated successfully!")
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func saveReport(filename, content string) {
	// 实现保存 Markdown 报告
}

func saveChartData(filename string, data map[string]interface{}) {
	// 实现保存 JSON 数据
}
```

---

## 2. 周报服务 API 调用示例

### 2.1 触发周报生成

**curl 示例**:

```bash
# 基本调用
curl -X POST http://localhost:8080/api/v1/weekly-reports \
  -H "Content-Type: application/json" \
  -d '{
    "cluster": "x-flannel",
    "time_range_days": 7,
    "utilization_threshold": 30,
    "min_gpu_count": 1,
    "top_n": 20,
    "send_email": true
  }'

# 响应
{
  "report_id": "rpt_20251123_x_flannel_abc123",
  "status": "generating",
  "message": "Report generation started",
  "created_at": "2025-11-23T19:53:39Z",
  "estimated_completion_time": "2025-11-23T19:55:00Z"
}
```

**Python 示例**:

```python
import requests
from datetime import datetime, timedelta

# API 配置
BASE_URL = "http://localhost:8080/api/v1"

def generate_weekly_report(cluster_name):
    """生成周报"""
    url = f"{BASE_URL}/weekly-reports"
    
    payload = {
        "cluster": cluster_name,
        "time_range_days": 7,
        "utilization_threshold": 30,
        "min_gpu_count": 1,
        "top_n": 20,
        "send_email": True
    }
    
    response = requests.post(url, json=payload)
    response.raise_for_status()
    
    data = response.json()
    print(f"Report ID: {data['report_id']}")
    print(f"Status: {data['status']}")
    
    return data['report_id']

if __name__ == "__main__":
    report_id = generate_weekly_report("x-flannel")
    print(f"Report generated: {report_id}")
```

**Go 示例**:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type GenerateReportRequest struct {
	Cluster              string `json:"cluster"`
	TimeRangeDays        int    `json:"time_range_days"`
	UtilizationThreshold int    `json:"utilization_threshold"`
	MinGPUCount          int    `json:"min_gpu_count"`
	TopN                 int    `json:"top_n"`
	SendEmail            bool   `json:"send_email"`
}

type GenerateReportResponse struct {
	ReportID                string    `json:"report_id"`
	Status                  string    `json:"status"`
	Message                 string    `json:"message"`
	CreatedAt               time.Time `json:"created_at"`
	EstimatedCompletionTime time.Time `json:"estimated_completion_time"`
}

func main() {
	client := resty.New()

	req := GenerateReportRequest{
		Cluster:              "x-flannel",
		TimeRangeDays:        7,
		UtilizationThreshold: 30,
		MinGPUCount:          1,
		TopN:                 20,
		SendEmail:            true,
	}

	var resp GenerateReportResponse

	_, err := client.R().
		SetBody(req).
		SetResult(&resp).
		Post("http://localhost:8080/api/v1/weekly-reports")

	if err != nil {
		panic(err)
	}

	fmt.Printf("Report ID: %s\n", resp.ReportID)
	fmt.Printf("Status: %s\n", resp.Status)
}
```

---

### 2.2 查询报告状态

```bash
# 轮询报告状态
REPORT_ID="rpt_20251123_x_flannel_abc123"

while true; do
  STATUS=$(curl -s "http://localhost:8080/api/v1/weekly-reports/$REPORT_ID/status" | jq -r '.status')
  echo "Current status: $STATUS"
  
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    break
  fi
  
  sleep 5
done

echo "Report generation finished with status: $STATUS"
```

---

### 2.3 下载报告

```bash
REPORT_ID="rpt_20251123_x_flannel_abc123"

# 下载 PDF
curl -O "http://localhost:8080/api/v1/weekly-reports/$REPORT_ID/download?format=pdf"

# 下载 HTML
curl -O "http://localhost:8080/api/v1/weekly-reports/$REPORT_ID/download?format=html"

# 下载 JSON
curl -O "http://localhost:8080/api/v1/weekly-reports/$REPORT_ID/download?format=json"
```

**Python 下载示例**:

```python
import requests

def download_report(report_id, format="pdf"):
    """下载报告"""
    url = f"http://localhost:8080/api/v1/weekly-reports/{report_id}/download"
    params = {"format": format}
    
    response = requests.get(url, params=params, stream=True)
    response.raise_for_status()
    
    filename = f"report_{report_id}.{format}"
    with open(filename, 'wb') as f:
        for chunk in response.iter_content(chunk_size=8192):
            f.write(chunk)
    
    print(f"Downloaded: {filename}")
    return filename

# 使用
download_report("rpt_20251123_x_flannel_abc123", "pdf")
download_report("rpt_20251123_x_flannel_abc123", "html")
```

---

### 2.4 查询报告列表

```bash
# 查询所有报告
curl "http://localhost:8080/api/v1/weekly-reports?page=1&page_size=10"

# 按集群过滤
curl "http://localhost:8080/api/v1/weekly-reports?cluster=x-flannel"

# 按状态过滤
curl "http://localhost:8080/api/v1/weekly-reports?status=completed"

# 按日期范围过滤
curl "http://localhost:8080/api/v1/weekly-reports?start_date=2025-11-01&end_date=2025-11-30"
```

---

## 3. 批量生成多集群报告

### 3.1 Shell 脚本

```bash
#!/bin/bash
# generate-weekly-reports.sh

API_URL="http://localhost:8080/api/v1"
CLUSTERS=("x-flannel" "y-cluster" "z-cluster")

for cluster in "${CLUSTERS[@]}"; do
  echo "Generating report for cluster: $cluster"
  
  RESPONSE=$(curl -s -X POST "$API_URL/weekly-reports" \
    -H "Content-Type: application/json" \
    -d "{
      \"cluster\": \"$cluster\",
      \"time_range_days\": 7,
      \"utilization_threshold\": 30,
      \"min_gpu_count\": 1,
      \"top_n\": 20,
      \"send_email\": true
    }")
  
  REPORT_ID=$(echo $RESPONSE | jq -r '.report_id')
  echo "  Report ID: $REPORT_ID"
done
```

### 3.2 Python 脚本

```python
import requests
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

API_URL = "http://localhost:8080/api/v1"
CLUSTERS = ["x-flannel", "y-cluster", "z-cluster"]

def generate_report_for_cluster(cluster_name):
    """为单个集群生成报告"""
    url = f"{API_URL}/weekly-reports"
    payload = {
        "cluster": cluster_name,
        "time_range_days": 7,
        "utilization_threshold": 30,
        "min_gpu_count": 1,
        "top_n": 20,
        "send_email": True
    }
    
    try:
        response = requests.post(url, json=payload, timeout=30)
        response.raise_for_status()
        data = response.json()
        return {
            "cluster": cluster_name,
            "report_id": data["report_id"],
            "status": "success"
        }
    except Exception as e:
        return {
            "cluster": cluster_name,
            "error": str(e),
            "status": "failed"
        }

def generate_weekly_reports_batch():
    """批量生成周报"""
    print(f"Generating reports for {len(CLUSTERS)} clusters...")
    
    results = []
    with ThreadPoolExecutor(max_workers=3) as executor:
        futures = {
            executor.submit(generate_report_for_cluster, cluster): cluster 
            for cluster in CLUSTERS
        }
        
        for future in as_completed(futures):
            result = future.result()
            results.append(result)
            
            if result["status"] == "success":
                print(f"✓ {result['cluster']}: {result['report_id']}")
            else:
                print(f"✗ {result['cluster']}: {result.get('error', 'Unknown error')}")
    
    return results

if __name__ == "__main__":
    results = generate_weekly_reports_batch()
    
    success_count = sum(1 for r in results if r["status"] == "success")
    print(f"\nCompleted: {success_count}/{len(CLUSTERS)} reports generated successfully")
```

---

## 4. 定时任务配置

### 4.1 Crontab 配置

```bash
# 编辑 crontab
crontab -e

# 每周一早上 9:00 生成报告
0 9 * * 1 /path/to/generate-weekly-reports.sh

# 或使用 Python 脚本
0 9 * * 1 /usr/bin/python3 /path/to/generate_reports.py

# 每月第一天生成月报（如果支持）
0 9 1 * * /path/to/generate-monthly-reports.sh
```

### 4.2 Systemd Timer

```ini
# /etc/systemd/system/weekly-report.service
[Unit]
Description=Generate Weekly GPU Cluster Report
After=network.target

[Service]
Type=oneshot
User=weekly-report
ExecStart=/usr/local/bin/generate-weekly-reports.sh
```

```ini
# /etc/systemd/system/weekly-report.timer
[Unit]
Description=Weekly GPU Cluster Report Timer

[Timer]
OnCalendar=Mon *-*-* 09:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
# 启用和启动 timer
sudo systemctl enable weekly-report.timer
sudo systemctl start weekly-report.timer

# 查看状态
sudo systemctl status weekly-report.timer
sudo systemctl list-timers
```

---

## 5. 邮件集成示例

### 5.1 重新发送邮件

```bash
# 重新发送邮件到新收件人
REPORT_ID="rpt_20251123_x_flannel_abc123"

curl -X POST "http://localhost:8080/api/v1/weekly-reports/$REPORT_ID/resend" \
  -H "Content-Type: application/json" \
  -d '{
    "recipients": [
      "newuser@example.com"
    ],
    "cc": [
      "manager@example.com"
    ]
  }'
```

### 5.2 自定义邮件模板

如果需要自定义邮件内容，可以在配置中修改：

```yaml
# config/config.yaml
email:
  subject_template: "【GPU 集群周报】{{.ClusterName}} - {{.WeekRange}}"
  body_template_file: "./templates/email/custom-notification.html"
```

---

## 6. 前端集成示例

### 6.1 React 组件

```typescript
import React, { useState, useEffect } from 'react';
import axios from 'axios';

interface Report {
  id: string;
  cluster: string;
  status: string;
  created_at: string;
  summary: {
    avg_utilization: number;
    avg_allocation: number;
    low_util_users_count: number;
  };
}

const WeeklyReportList: React.FC = () => {
  const [reports, setReports] = useState<Report[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchReports();
  }, []);

  const fetchReports = async () => {
    try {
      const response = await axios.get('http://localhost:8080/api/v1/weekly-reports');
      setReports(response.data.reports);
    } catch (error) {
      console.error('Failed to fetch reports:', error);
    } finally {
      setLoading(false);
    }
  };

  const downloadReport = (reportId: string, format: 'html' | 'pdf' | 'json') => {
    window.open(
      `http://localhost:8080/api/v1/weekly-reports/${reportId}/download?format=${format}`,
      '_blank'
    );
  };

  if (loading) return <div>Loading...</div>;

  return (
    <div className="report-list">
      <h2>周报列表</h2>
      {reports.map(report => (
        <div key={report.id} className="report-card">
          <h3>{report.cluster}</h3>
          <p>状态: {report.status}</p>
          <p>平均利用率: {report.summary.avg_utilization.toFixed(2)}%</p>
          <p>低利用率用户: {report.summary.low_util_users_count}</p>
          <button onClick={() => downloadReport(report.id, 'pdf')}>
            下载 PDF
          </button>
          <button onClick={() => downloadReport(report.id, 'html')}>
            查看 HTML
          </button>
        </div>
      ))}
    </div>
  );
};

export default WeeklyReportList;
```

---

## 7. 监控和告警

### 7.1 Prometheus 查询示例

```promql
# 报告生成成功率
rate(weekly_reports_generated_total{status="completed"}[5m]) / 
rate(weekly_reports_generated_total[5m])

# 报告生成耗时（P99）
histogram_quantile(0.99, 
  rate(weekly_report_generation_duration_seconds_bucket[5m])
)

# 邮件发送失败率
rate(weekly_reports_emails_sent_total{status="failed"}[5m])
```

### 7.2 告警规则

```yaml
# prometheus-alerts.yaml
groups:
- name: weekly_reports
  interval: 1m
  rules:
  - alert: ReportGenerationFailed
    expr: |
      rate(weekly_reports_generated_total{status="failed"}[5m]) > 0
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "周报生成失败"
      description: "集群 {{ $labels.cluster }} 的周报生成失败"
  
  - alert: ReportGenerationSlow
    expr: |
      histogram_quantile(0.99, 
        rate(weekly_report_generation_duration_seconds_bucket[5m])
      ) > 300
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "周报生成耗时过长"
      description: "P99 生成时间超过 5 分钟"
```

---

## 8. 故障恢复脚本

### 8.1 重试失败的报告

```python
import requests

API_URL = "http://localhost:8080/api/v1"

def retry_failed_reports():
    """重试所有失败的报告"""
    # 1. 查询失败的报告
    response = requests.get(f"{API_URL}/weekly-reports?status=failed")
    reports = response.json()["reports"]
    
    print(f"Found {len(reports)} failed reports")
    
    # 2. 重新生成
    for report in reports:
        print(f"Retrying report {report['id']} for cluster {report['cluster']}")
        
        payload = {
            "cluster": report["cluster"],
            "time_range_days": report["time_range"]["days"],
            "utilization_threshold": report["parameters"]["utilization_threshold"],
            "min_gpu_count": report["parameters"]["min_gpu_count"],
            "top_n": report["parameters"]["top_n"],
            "send_email": False  # 先不发邮件
        }
        
        try:
            response = requests.post(f"{API_URL}/weekly-reports", json=payload)
            new_report_id = response.json()["report_id"]
            print(f"  → New report ID: {new_report_id}")
        except Exception as e:
            print(f"  → Failed: {e}")

if __name__ == "__main__":
    retry_failed_reports()
```

---

## 9. 数据备份脚本

```bash
#!/bin/bash
# backup-reports.sh

BACKUP_DIR="/backups/weekly-reports"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 备份数据库
pg_dump -U weekly_reports_user weekly_reports > "$BACKUP_DIR/db_backup_$TIMESTAMP.sql"

# 备份报告文件
tar -czf "$BACKUP_DIR/files_backup_$TIMESTAMP.tar.gz" /data/reports/

# 清理 30 天前的备份
find $BACKUP_DIR -name "*.sql" -mtime +30 -delete
find $BACKUP_DIR -name "*.tar.gz" -mtime +30 -delete

echo "Backup completed: $TIMESTAMP"
```

---

## 10. 开发环境快速启动

```bash
#!/bin/bash
# dev-setup.sh

# 1. 启动依赖服务
docker-compose -f docker-compose.dev.yaml up -d postgres

# 2. 等待数据库就绪
until pg_isready -h localhost -p 5432 -U weekly_reports_user; do
  echo "Waiting for database..."
  sleep 2
done

# 3. 运行迁移
psql $DATABASE_URL < migrations/001_init.sql

# 4. 启动应用
go run cmd/server/main.go
```

---

这些示例涵盖了周报服务的主要使用场景，可以作为开发和集成的参考。

