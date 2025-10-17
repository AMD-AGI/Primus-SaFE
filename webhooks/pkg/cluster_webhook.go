/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func AddClusterWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.ClusterKind), &webhook.Admission{Handler: &ClusterMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.ClusterKind), &webhook.Admission{Handler: &ClusterValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

type ClusterMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *ClusterMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	cluster := &v1.Cluster{}
	if err := m.decoder.Decode(req, cluster); err != nil {
		return handleError(v1.ClusterKind, err)
	}
	m.mutateOnCreation(ctx, cluster)
	data, err := json.Marshal(cluster)
	if err != nil {
		return handleError(v1.ClusterKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

func (m *ClusterMutator) mutateOnCreation(_ context.Context, cluster *v1.Cluster) {
	cluster.Name = stringutil.NormalizeName(cluster.Name)
	controllerutil.AddFinalizer(cluster, v1.ClusterFinalizer)
	if cluster.Spec.ControlPlane.KubeNetworkPlugin == nil || *cluster.Spec.ControlPlane.KubeNetworkPlugin == "" {
		cluster.Spec.ControlPlane.KubeNetworkPlugin = pointer.String(v1.CiliumNetworkPlugin)
	}
}

type ClusterValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *ClusterValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	cluster := &v1.Cluster{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, cluster); err != nil {
			break
		}
		if !cluster.GetDeletionTimestamp().IsZero() {
			return admission.Allowed("")
		}
		err = v.validateOnCreation(ctx, cluster)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, cluster); err != nil {
			break
		}
		if !cluster.GetDeletionTimestamp().IsZero() {
			return admission.Allowed("")
		}
		oldCluster := &v1.Cluster{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldCluster); err == nil {
			err = v.validateOnUpdate(cluster, oldCluster)
		}
	default:
	}
	if err != nil {
		return handleError(v1.ClusterKind, err)
	}
	return admission.Allowed("")
}

func (v *ClusterValidator) validateOnCreation(ctx context.Context, cluster *v1.Cluster) error {
	if err := validateDisplayName(v1.GetDisplayName(cluster)); err != nil {
		return err
	}
	if err := v.validateControlPlane(ctx, cluster); err != nil {
		return err
	}
	return nil
}

func (v *ClusterValidator) validateControlPlane(ctx context.Context, cluster *v1.Cluster) error {
	if len(cluster.Spec.ControlPlane.Nodes) == 0 {
		return fmt.Errorf("the KubeControlPlane nodes of spec are empty")
	}
	if err := v.validateNodesInUse(ctx, cluster); err != nil {
		return err
	}
	if cluster.Spec.ControlPlane.KubePodsSubnet == nil || *cluster.Spec.ControlPlane.KubePodsSubnet == "" {
		return fmt.Errorf("the KubePodsSubnet of spec is empty")
	}
	if cluster.Spec.ControlPlane.KubeServiceAddress == nil || *cluster.Spec.ControlPlane.KubeServiceAddress == "" {
		return fmt.Errorf("the KubeServiceAddress of spec is empty")
	}
	if cluster.Spec.ControlPlane.NodeLocalDNSIP == nil || *cluster.Spec.ControlPlane.NodeLocalDNSIP == "" {
		return fmt.Errorf("the NodeLocalDNSIP of spec is empty")
	}
	if cluster.Spec.ControlPlane.KubeSprayImage == nil || *cluster.Spec.ControlPlane.KubeSprayImage == "" {
		return fmt.Errorf("the KubeSprayImage of spec is empty")
	}
	return nil
}

func (v *ClusterValidator) validateNodesInUse(ctx context.Context, cluster *v1.Cluster) error {
	clusterList := &v1.ClusterList{}
	if err := v.List(ctx, clusterList); err != nil {
		return err
	}
	currentNodesSet := sets.NewSet()
	for _, cl := range clusterList.Items {
		for _, n := range cl.Spec.ControlPlane.Nodes {
			currentNodesSet.Insert(n)
		}
	}
	for _, n := range cluster.Spec.ControlPlane.Nodes {
		if currentNodesSet.Has(n) {
			return commonerrors.NewAlreadyExist(fmt.Sprintf("the node(%s) is already in use", n))
		}
	}
	return nil
}

func (v *ClusterValidator) validateOnUpdate(newCluster, oldCluster *v1.Cluster) error {
	if err := v.validateImmutableFields(newCluster, oldCluster); err != nil {
		return err
	}
	return nil
}

func (v *ClusterValidator) validateImmutableFields(newCluster, oldCluster *v1.Cluster) error {
	if !slice.EqualIgnoreOrder(newCluster.Spec.ControlPlane.Nodes, oldCluster.Spec.ControlPlane.Nodes) {
		return field.Forbidden(field.NewPath("spec").Key("controlPlane").
			Key("nodes"), "immutable")
	}
	return nil
}

func getCluster(ctx context.Context, cli client.Client, clusterId string) (*v1.Cluster, error) {
	if clusterId == "" {
		return nil, nil
	}
	cluster := &v1.Cluster{}
	if err := cli.Get(ctx, client.ObjectKey{Name: clusterId}, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
