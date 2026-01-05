//go:build integration

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func mockWorkloadHandler(ctx context.Context, obj *unstructured.Unstructured) error {
	Expect(obj.GetKind()).To(Equal(v1.WorkloadKind))
	Expect(obj.GroupVersionKind().Version).To(Equal(common.DefaultVersion))
	Expect(obj.GroupVersionKind().Group).To(Equal(v1.SchemeGroupVersion.Group))
	Expect(obj.GetName()).To(Equal(TestWorkloadData.Name))

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["key"] = "value"
	obj.SetLabels(labels)
	mockClient.Update(ctx, obj)
	return nil
}

func mockWorkloadFilter(_, objectNew *unstructured.Unstructured) bool {
	if objectNew.GetName() == "test" {
		return true
	}
	return false
}

func testWorkloadExporter() {
	var err error

	workload1 := TestWorkloadData.DeepCopy()
	createObject(workload1)
	time.Sleep(time.Millisecond * 200)
	Expect(err).Should(BeNil())
	Expect(ctrlutil.ContainsFinalizer(workload1, v1.ExporterFinalizer)).To(BeTrue())
	Expect(workload1.Labels["key"]).To(Equal("value"))
	deleteObject(workload1)

	workload2 := TestWorkloadData.DeepCopy()
	workload2.Name = "test"
	createObject(workload2)
	time.Sleep(time.Millisecond * 200)
	Expect(err).Should(BeNil())
	Expect(ctrlutil.ContainsFinalizer(workload2, v1.ExporterFinalizer)).To(BeFalse())
	Expect(workload2.Labels["key"]).NotTo(Equal("value"))
	deleteObject(workload2)
}

var _ = Describe("Resource Exporter Test", func() {
	Context("", func() {
		It("Test workload exporter", func() {
			testWorkloadExporter()
		})
	})
})
