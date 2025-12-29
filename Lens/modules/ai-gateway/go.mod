module github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway

go 1.24.7

replace github.com/AMD-AGI/Primus-SaFE/Lens/core => ../core

require (
	github.com/AMD-AGI/Primus-SaFE/Lens/core v0.0.0-00010101000000-000000000000
	github.com/gin-gonic/gin v1.10.1
	github.com/google/uuid v1.6.0
	github.com/robfig/cron/v3 v3.0.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/gorm v1.30.0
)

