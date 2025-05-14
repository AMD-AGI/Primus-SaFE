module github.com/AMD-AIG-AIMA/SAFE/webhooks

go 1.24.2

require (
	github.com/AMD-AIG-AIMA/SAFE/apis v0.0.0
	github.com/AMD-AIG-AIMA/SAFE/utils v0.0.0
	github.com/AMD-AIG-AIMA/SAFE/common v0.0.0
	k8s.io/apimachinery v0.33.0
	k8s.io/client-go v0.33.0
	k8s.io/klog/v2 v2.130.1
	sigs.k8s.io/controller-runtime v0.20.4
)

replace (
	github.com/AMD-AIG-AIMA/SAFE/apis => ../apis
	github.com/AMD-AIG-AIMA/SAFE/utils => ../utils
	github.com/AMD-AIG-AIMA/SAFE/common => ../common
)