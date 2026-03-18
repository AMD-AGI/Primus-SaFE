// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_usage_weekly_report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// ReportRenderer handles rendering reports in various formats
type ReportRenderer struct {
	config *config.WeeklyReportConfig
}

// NewReportRenderer creates a new ReportRenderer instance
func NewReportRenderer(cfg *config.WeeklyReportConfig) *ReportRenderer {
	return &ReportRenderer{
		config: cfg,
	}
}

// RenderHTML renders the report as HTML
func (r *ReportRenderer) RenderHTML(ctx context.Context, data *ReportData) ([]byte, error) {
	log.Info("ReportRenderer: rendering HTML report")

	// Convert Markdown to HTML
	htmlBody := r.markdownToHTML(data.MarkdownReport)

	// Replace chart placeholders with actual chart divs
	htmlBody = r.replaceChartPlaceholders(htmlBody, data.ChartData)

	// Generate full HTML page with template
	fullHTML, err := r.generateHTMLPage(data, htmlBody)
	if err != nil {
		log.Errorf("ReportRenderer: failed to generate HTML page: %v", err)
		return nil, err
	}

	log.Info("ReportRenderer: HTML rendering completed")
	return []byte(fullHTML), nil
}

// RenderPDF renders the report as PDF using chromedp (headless Chrome)
func (r *ReportRenderer) RenderPDF(ctx context.Context, htmlContent []byte) ([]byte, error) {
	// Check if PDF rendering is enabled
	if !r.isPDFRenderingEnabled() {
		log.Info("ReportRenderer: PDF rendering is not enabled in configuration")
		return nil, nil
	}

	log.Info("ReportRenderer: starting PDF rendering using chromedp")

	// Create a temporary file to store the HTML content
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("report_%d.html", time.Now().UnixNano()))

	err := os.WriteFile(tmpFile, htmlContent, 0644)
	if err != nil {
		log.Errorf("ReportRenderer: failed to write temporary HTML file: %v", err)
		return nil, fmt.Errorf("failed to write temporary HTML file: %w", err)
	}
	defer os.Remove(tmpFile) // Clean up temporary file

	log.Debugf("ReportRenderer: temporary HTML file created at %s", tmpFile)

	// Create chromedp context with custom allocator options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	// Create browser context
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Set a reasonable timeout for PDF generation
	pdfCtx, pdfCancel := context.WithTimeout(browserCtx, 60*time.Second)
	defer pdfCancel()

	// Generate PDF
	var pdfBuf []byte

	// Build file URL
	fileURL := "file:///" + filepath.ToSlash(tmpFile)

	log.Debugf("ReportRenderer: loading HTML from %s", fileURL)

	err = chromedp.Run(pdfCtx,
		chromedp.Navigate(fileURL),
		// Wait for the page to fully load
		chromedp.WaitReady("body"),
		// Wait for ECharts to initialize (check if echarts object exists)
		chromedp.Sleep(5*time.Second), // Give ECharts time to render all charts
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Info("ReportRenderer: generating PDF from rendered HTML")
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).   // A4 width in inches
				WithPaperHeight(11.69). // A4 height in inches
				WithMarginTop(0.4).
				WithMarginBottom(0.4).
				WithMarginLeft(0.4).
				WithMarginRight(0.4).
				WithScale(0.9).
				WithDisplayHeaderFooter(false).
				WithPreferCSSPageSize(false).
				Do(ctx)
			return err
		}),
	)

	if err != nil {
		log.Errorf("ReportRenderer: chromedp failed to generate PDF: %v", err)
		log.Warn("ReportRenderer: Make sure Chrome/Chromium is installed on the system")
		return nil, fmt.Errorf("chromedp failed to generate PDF (Chrome may not be installed): %w", err)
	}

	log.Infof("ReportRenderer: PDF generation completed successfully, size: %d bytes", len(pdfBuf))
	return pdfBuf, nil
}

// markdownToHTML converts markdown text to HTML
func (r *ReportRenderer) markdownToHTML(mdText string) string {
	// Preprocess: remove code block wrappers if present
	mdText = r.unwrapMarkdownCodeBlock(mdText)

	// Create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(mdText))

	// Create HTML renderer with options
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return string(markdown.Render(doc, renderer))
}

// unwrapMarkdownCodeBlock removes code block wrapper (```markdown or ```) from the content
func (r *ReportRenderer) unwrapMarkdownCodeBlock(content string) string {
	// Trim leading and trailing whitespace
	content = strings.TrimSpace(content)

	// Check if content is wrapped in code blocks
	// Pattern 1: ```markdown\n...\n```
	// Pattern 2: ```\n...\n```
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) < 2 {
			return content
		}

		// Find the first line - should be ``` or ```markdown or ```md
		firstLine := strings.TrimSpace(lines[0])
		if firstLine == "```" || firstLine == "```markdown" || firstLine == "```md" {
			// Remove first line
			lines = lines[1:]

			// Find and remove the last ``` line
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.TrimSpace(lines[i]) == "```" {
					lines = lines[:i]
					break
				}
			}

			// Rejoin the content
			unwrapped := strings.Join(lines, "\n")
			log.Debugf("ReportRenderer: unwrapped markdown code block, original length: %d, unwrapped length: %d",
				len(content), len(unwrapped))
			return unwrapped
		}
	}

	return content
}

// replaceChartPlaceholders replaces chart placeholders in HTML with actual chart divs
func (r *ReportRenderer) replaceChartPlaceholders(htmlContent string, chartData *ChartData) string {
	if chartData == nil {
		return htmlContent
	}

	// Replace cluster_usage_trend placeholder
	if chartData.ClusterUsageTrend != nil {
		chartDiv := `<div id="chart-cluster-usage-trend" class="chart" style="width: 100%; height: 400px;"></div>`

		// Support multiple placeholder formats:
		// 1. HTML comment format: <!-- CHART:cluster_usage_trend -->
		// 2. Double brace format (may be wrapped in <p> tags): {{CHART:cluster_usage_trend}}
		// 3. Double brace in paragraph: <p>{{CHART:cluster_usage_trend}}</p>

		htmlContent = strings.Replace(htmlContent, "<!-- CHART:cluster_usage_trend -->", chartDiv, -1)
		htmlContent = strings.Replace(htmlContent, "{{CHART:cluster_usage_trend}}", chartDiv, -1)
		htmlContent = strings.Replace(htmlContent, "<p>{{CHART:cluster_usage_trend}}</p>", chartDiv, -1)

		log.Debugf("ReportRenderer: replaced cluster_usage_trend chart placeholder")
	}

	return htmlContent
}

// generateHTMLPage generates a complete HTML page with styling and charts
func (r *ReportRenderer) generateHTMLPage(data *ReportData, bodyHTML string) (string, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GPU Usage Weekly Report - {{.ClusterName}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .header {
            background: {{.PrimaryColor}};
            color: white;
            padding: 30px;
            border-radius: 8px;
            margin-bottom: 20px;
        }
        .header h1 {
            margin: 0 0 10px 0;
        }
        .header .meta {
            opacity: 0.9;
            font-size: 14px;
        }
        .content {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .summary-cards {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin: 20px 0;
        }
        .card {
            background: #f8f9fa;
            padding: 15px;
            border-radius: 6px;
            border-left: 4px solid {{.PrimaryColor}};
        }
        .card-title {
            font-size: 12px;
            color: #666;
            text-transform: uppercase;
        }
        .card-value {
            font-size: 24px;
            font-weight: bold;
            color: #333;
            margin: 5px 0;
        }
        .chart {
            margin: 30px 0;
            min-height: 400px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 20px 0;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f8f9fa;
            font-weight: 600;
        }
        tr:hover {
            background-color: #f8f9fa;
        }
        .footer {
            text-align: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #ddd;
            color: #666;
            font-size: 14px;
        }
        h1, h2, h3 {
            color: #333;
        }
        h2 {
            border-bottom: 2px solid {{.PrimaryColor}};
            padding-bottom: 10px;
            margin-top: 30px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>GPU Usage Weekly Report</h1>
        <div class="meta">
            <strong>Cluster:</strong> {{.ClusterName}}<br>
            <strong>Period:</strong> {{.PeriodStart}} to {{.PeriodEnd}}<br>
            <strong>Generated:</strong> {{.GeneratedAt}}
        </div>
    </div>

    {{if .Summary}}
    <div class="content">
        <h2>Summary Statistics</h2>
        <div class="summary-cards">
            <div class="card">
                <div class="card-title">Total GPUs</div>
                <div class="card-value">{{.Summary.TotalGPUs}}</div>
            </div>
            <div class="card">
                <div class="card-title">Avg Utilization</div>
                <div class="card-value">{{printf "%.1f" .Summary.AvgUtilization}}%</div>
            </div>
            <div class="card">
                <div class="card-title">Avg Allocation</div>
                <div class="card-value">{{printf "%.1f" .Summary.AvgAllocation}}%</div>
            </div>
            <div class="card">
                <div class="card-title">Low Util Users</div>
                <div class="card-value">{{.Summary.LowUtilCount}}</div>
            </div>
        </div>
    </div>
    {{end}}

    <div class="content">
        {{.BodyHTML}}
    </div>

    <div class="footer">
        <p>Generated by Primus Lens - {{.CompanyName}}</p>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/echarts@5.4.3/dist/echarts.min.js"></script>
    <script>
        // Chart data from backend
        const chartData = {{.ChartDataJSON}};

        // Initialize charts when DOM is ready
        document.addEventListener('DOMContentLoaded', function() {
            // Render cluster usage trend chart
            if (chartData.cluster_usage_trend) {
                renderClusterUsageTrendChart(chartData.cluster_usage_trend);
            }
        });

        function renderClusterUsageTrendChart(data) {
            const chartDom = document.getElementById('chart-cluster-usage-trend');
            if (!chartDom) {
                console.warn('Chart container not found: chart-cluster-usage-trend');
                return;
            }

            const myChart = echarts.init(chartDom);
            
            // Prepare series data
            const series = (data.series || []).map(s => ({
                name: s.name,
                type: s.type || 'line',
                data: s.data || [],
                smooth: s.smooth !== undefined ? s.smooth : true,
                symbol: 'circle',
                symbolSize: 6,
                lineStyle: {
                    width: 2
                }
            }));

            const option = {
                title: {
                    text: data.title || 'GPU Utilization and Allocation Rate',
                    left: 'center',
                    top: 10,
                    textStyle: {
                        fontSize: 18,
                        fontWeight: 'bold'
                    }
                },
                tooltip: {
                    trigger: 'axis',
                    axisPointer: {
                        type: 'cross'
                    },
                    formatter: function(params) {
                        let result = params[0].axisValue + '<br/>';
                        params.forEach(param => {
                            result += param.marker + param.seriesName + ': ' + 
                                     (param.value != null ? param.value.toFixed(2) + '%' : 'N/A') + '<br/>';
                        });
                        return result;
                    }
                },
                legend: {
                    data: series.map(s => s.name),
                    top: 40,
                    left: 'center'
                },
                grid: {
                    left: '3%',
                    right: '4%',
                    bottom: '10%',
                    top: '80px',
                    containLabel: true
                },
                xAxis: {
                    type: 'category',
                    boundaryGap: false,
                    data: data.xAxis || [],
                    axisLabel: {
                        rotate: 45,
                        interval: Math.floor((data.xAxis || []).length / 10) || 0
                    }
                },
                yAxis: {
                    type: 'value',
                    name: 'Percentage (%)',
                    axisLabel: {
                        formatter: '{value}%'
                    },
                    min: 0,
                    max: function(value) {
                        return Math.ceil(value.max / 10) * 10;
                    }
                },
                series: series,
                dataZoom: [
                    {
                        type: 'inside',
                        start: 0,
                        end: 100
                    },
                    {
                        start: 0,
                        end: 100,
                        height: 20,
                        bottom: 20
                    }
                ]
            };

            myChart.setOption(option);

            // Responsive resize
            window.addEventListener('resize', function() {
                myChart.resize();
            });
        }
    </script>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare chart data JSON
	chartDataJSON := r.prepareChartDataJSON(data.ChartData)

	// Extract start_time and end_time from metadata.parameters or fallback to Period
	periodStart, periodEnd, generatedAt := r.extractTimeFromMetadata(data)

	// Prepare template data
	templateData := map[string]interface{}{
		"ClusterName":   data.ClusterName,
		"PeriodStart":   periodStart,
		"PeriodEnd":     periodEnd,
		"GeneratedAt":   generatedAt,
		"BodyHTML":      template.HTML(bodyHTML),
		"PrimaryColor":  r.getPrimaryColor(),
		"CompanyName":   r.getCompanyName(),
		"Summary":       data.Summary,
		"ChartDataJSON": template.JS(chartDataJSON),
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getPrimaryColor gets the primary color from configuration or returns default
func (r *ReportRenderer) getPrimaryColor() string {
	if r.config != nil && r.config.Brand.PrimaryColor != "" {
		return r.config.Brand.PrimaryColor
	}
	return "#ED1C24" // AMD red as default
}

// getCompanyName gets the company name from configuration or returns default
func (r *ReportRenderer) getCompanyName() string {
	if r.config != nil && r.config.Brand.CompanyName != "" {
		return r.config.Brand.CompanyName
	}
	return "AMD AGI"
}

// prepareChartDataJSON converts chart data to JSON string for JavaScript
func (r *ReportRenderer) prepareChartDataJSON(chartData *ChartData) string {
	if chartData == nil {
		return "{}"
	}

	data := make(map[string]interface{})
	if chartData.ClusterUsageTrend != nil {
		data["cluster_usage_trend"] = chartData.ClusterUsageTrend
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		log.Errorf("ReportRenderer: failed to marshal chart data: %v", err)
		return "{}"
	}

	return string(jsonBytes)
}

// isPDFRenderingEnabled checks if PDF rendering is enabled in configuration
func (r *ReportRenderer) isPDFRenderingEnabled() bool {
	if r.config == nil {
		return false
	}

	for _, format := range r.config.OutputFormats {
		if format == "pdf" {
			return true
		}
	}

	return false
}

// extractTimeFromMetadata extracts start_time and end_time from metadata.parameters
// Falls back to Period if metadata is not available
func (r *ReportRenderer) extractTimeFromMetadata(data *ReportData) (periodStart, periodEnd, generatedAt string) {
	// Try to extract from metadata.parameters
	if data.Metadata != nil {
		if params, ok := data.Metadata["parameters"].(map[string]interface{}); ok {
			// Extract start_time
			if startTimeStr, ok := params["start_time"].(string); ok && startTimeStr != "" {
				// Try to parse RFC3339 format
				if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
					periodStart = t.Format("2006-01-02")
				} else {
					// If parsing fails, use the string directly
					periodStart = startTimeStr
				}
			}

			// Extract end_time
			if endTimeStr, ok := params["end_time"].(string); ok && endTimeStr != "" {
				// Try to parse RFC3339 format
				if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
					periodEnd = t.Format("2006-01-02")
					generatedAt = t.Format("2006-01-02 15:04:05")
				} else {
					// If parsing fails, use the string directly
					periodEnd = endTimeStr
					generatedAt = endTimeStr
				}
			}
		}
	}

	// Fallback to Period if metadata extraction failed
	if periodStart == "" {
		periodStart = data.Period.StartTime.Format("2006-01-02")
	}
	if periodEnd == "" {
		periodEnd = data.Period.EndTime.Format("2006-01-02")
	}
	if generatedAt == "" {
		generatedAt = data.Period.EndTime.Format("2006-01-02 15:04:05")
	}

	log.Debugf("ReportRenderer: using time range from metadata - start: %s, end: %s, generated: %s",
		periodStart, periodEnd, generatedAt)

	return periodStart, periodEnd, generatedAt
}
