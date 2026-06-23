module github.com/AMD-AIG-AIMA/SAFE/kube-scheduler-plugins

go 1.25.5

require (
	k8s.io/api v0.34.1
	k8s.io/apimachinery v0.34.1
	k8s.io/client-go v0.34.1
	k8s.io/component-base v0.33.1
	k8s.io/component-helpers v0.32.5
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubernetes v1.32.5
	k8s.io/utils v0.0.0-20250820121507-0af2bda4dd1d
	sigs.k8s.io/controller-runtime v0.21.0
	sigs.k8s.io/scheduler-plugins v0.32.5-devel
)

require (
	cel.dev/expr v0.25.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containers/common v0.61.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/crossplane/crossplane-runtime v0.14.1-0.20210713194031-85b19c28ea88 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/diktyo-io/appgroup-api v1.0.1-alpha // indirect
	github.com/diktyo-io/networktopology-api v1.0.1-alpha // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/cel-go v0.25.0 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20250820193118-f64d9cf942d6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus v1.1.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.1-0.20210315223345-82c243799c99 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k8stopologyawareschedwg/noderesourcetopology-api v0.1.2 // indirect
	github.com/k8stopologyawareschedwg/podfingerprint v0.2.2 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/onsi/ginkgo/v2 v2.25.1 // indirect
	github.com/onsi/gomega v1.38.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/runtime-spec v1.3.0 // indirect
	github.com/opencontainers/runtime-tools v0.9.1-0.20251114084447-edf4cb3d2116 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/paypal/load-watcher v0.2.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/seccomp/libseccomp-golang v0.10.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.etcd.io/etcd/api/v3 v3.6.6 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.6 // indirect
	go.etcd.io/etcd/client/v3 v3.6.6 // indirect
	go.etcd.io/etcd/server/v3 v3.6.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/net v0.54.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/term v0.43.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260406210006-6f92a3bedf2d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260406210006-6f92a3bedf2d // indirect
	google.golang.org/grpc v1.81.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	k8s.io/apiextensions-apiserver v0.33.0 // indirect
	k8s.io/apiserver v0.33.1 // indirect
	k8s.io/cloud-provider v0.32.5 // indirect
	k8s.io/controller-manager v0.32.5 // indirect
	k8s.io/csi-translation-lib v0.32.5 // indirect
	k8s.io/dynamic-resource-allocation v0.32.5 // indirect
	k8s.io/kms v0.32.5 // indirect
	k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b // indirect
	k8s.io/kube-scheduler v0.32.5 // indirect
	k8s.io/kubelet v0.32.5 // indirect
	k8s.io/metrics v0.32.5 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.31.2 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/release-utils v0.12.2 // indirect
	sigs.k8s.io/security-profiles-operator v0.4.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.32.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.32.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.32.5
	k8s.io/apiserver => k8s.io/apiserver v0.32.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.32.5
	k8s.io/client-go => k8s.io/client-go v0.32.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.32.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.32.5
	k8s.io/code-generator => k8s.io/code-generator v0.32.5
	k8s.io/component-base => k8s.io/component-base v0.32.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.32.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.32.5
	k8s.io/cri-api => k8s.io/cri-api v0.32.5
	k8s.io/cri-client => k8s.io/cri-client v0.32.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.32.5
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.32.5
	k8s.io/endpointslice => k8s.io/endpointslice v0.32.5
	k8s.io/kms => k8s.io/kms v0.32.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.32.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.32.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.32.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.32.5
	k8s.io/kubectl => k8s.io/kubectl v0.32.5
	k8s.io/kubelet => k8s.io/kubelet v0.32.5
	k8s.io/kubernetes => k8s.io/kubernetes v1.32.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.31.8
	k8s.io/metrics => k8s.io/metrics v0.32.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.32.5
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.32.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.32.5
)

replace k8s.io/externaljwt => k8s.io/externaljwt v0.32.5

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.32.5

replace k8s.io/sample-controller => k8s.io/sample-controller v0.32.5
