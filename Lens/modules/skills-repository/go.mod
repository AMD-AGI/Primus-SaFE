module github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository

go 1.24.7

require (
	github.com/AMD-AGI/Primus-SaFE/Lens/core v0.0.0
	github.com/gin-gonic/gin v1.10.1
	github.com/go-resty/resty/v2 v2.16.5
	github.com/pgvector/pgvector-go v0.2.2
	github.com/sashabaranov/go-openai v1.36.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/gorm v1.30.0
)

replace github.com/AMD-AGI/Primus-SaFE/Lens/core => ../core
