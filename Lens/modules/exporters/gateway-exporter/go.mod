module github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter

go 1.23

require (
	github.com/AMD-AGI/Primus-SaFE/Lens/core v0.0.0
	github.com/gin-gonic/gin v1.10.0
	github.com/prometheus/client_golang v1.20.5
	github.com/prometheus/client_model v0.6.1
	github.com/prometheus/common v0.60.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.31.2
	k8s.io/apimachinery v0.31.2
	sigs.k8s.io/controller-runtime v0.19.1
)

replace github.com/AMD-AGI/Primus-SaFE/Lens/core => ../core

