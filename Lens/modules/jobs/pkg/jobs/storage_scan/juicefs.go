package storage_scan

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type JuiceFSDriver struct{}

func (d *JuiceFSDriver) Name() string { return "juicefs" }

func (d *JuiceFSDriver) Detect(ctx context.Context, dctx DriverContext) (int, error) {
	// 简化：按 StorageClass 的 provisioner 包含 "juicedata.com" 或 "csi.juicefs.com" 检测
	scs, err := dctx.Kube.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, err
	}
	cnt := 0
	for _, sc := range scs.Items {
		if sc.Provisioner == "csi.juicefs.com" || sc.Provisioner == "juicedata.com" {
			cnt++
		}
	}
	return cnt, nil
}

func (d *JuiceFSDriver) ListBackends(ctx context.Context, dctx DriverContext) ([]string, error) {
	scs, err := dctx.Kube.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var backends []string
	for _, sc := range scs.Items {
		if sc.Provisioner == "csi.juicefs.com" || sc.Provisioner == "juicedata.com" {
			backends = append(backends, sc.Name)
		}
	}
	return backends, nil
}

func (d *JuiceFSDriver) Collect(ctx context.Context, dctx DriverContext, backend string) (BackendReport, error) {
	br := BackendReport{BackendKind: BackendJuiceFS, BackendName: backend, Health: HealthUnknown}
	sc, err := dctx.Kube.StorageV1().StorageClasses().Get(ctx, backend, metav1.GetOptions{})
	if err != nil {
		return br, err
	}
	br.MetaSecret = types.NamespacedName{
		Namespace: sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"],
		Name:      sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"],
	}
	secret, err := dctx.Kube.CoreV1().Secrets(br.MetaSecret.Namespace).Get(ctx, br.MetaSecret.Name, metav1.GetOptions{})
	if err != nil {
		return br, err
	}
	br.BackendName = string(secret.Data["name"])
	br.MetaSecret = types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}
	return br, nil
}
