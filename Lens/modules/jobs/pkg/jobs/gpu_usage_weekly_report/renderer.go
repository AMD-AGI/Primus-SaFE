package gpu_usage_weekly_report

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
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

	// Generate full HTML page with template
	fullHTML, err := r.generateHTMLPage(data, htmlBody)
	if err != nil {
		log.Errorf("ReportRenderer: failed to generate HTML page: %v", err)
		return nil, err
	}

	log.Info("ReportRenderer: HTML rendering completed")
	return []byte(fullHTML), nil
}

// RenderPDF renders the report as PDF (stub implementation for now)
func (r *ReportRenderer) RenderPDF(ctx context.Context, htmlContent []byte) ([]byte, error) {
	log.Info("ReportRenderer: PDF rendering not yet implemented, skipping")
	// TODO: Implement PDF rendering using chromedp or wkhtmltopdf
	// For now, return nil to indicate PDF is not available
	return nil, nil
}

// markdownToHTML converts markdown text to HTML
func (r *ReportRenderer) markdownToHTML(mdText string) string {
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
        // Chart initialization would go here
        // This is a placeholder for future chart rendering implementation
    </script>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare template data
	templateData := map[string]interface{}{
		"ClusterName":  data.ClusterName,
		"PeriodStart":  data.Period.StartTime.Format("2006-01-02"),
		"PeriodEnd":    data.Period.EndTime.Format("2006-01-02"),
		"GeneratedAt":  data.Period.EndTime.Format("2006-01-02 15:04:05"),
		"BodyHTML":     template.HTML(bodyHTML),
		"PrimaryColor": r.getPrimaryColor(),
		"CompanyName":  r.getCompanyName(),
		"Summary":      data.Summary,
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
