module github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter

go 1.24.7

replace github.com/AMD-AGI/Primus-SaFE/Lens/core => ../../core

require (
	github.com/AMD-AGI/Primus-SaFE/Lens/core v0.0.0-00010101000000-000000000000
	k8s.io/apimachinery v0.34.0
	k8s.io/client-go v0.34.0
	sigs.k8s.io/controller-runtime v0.21.0
)

