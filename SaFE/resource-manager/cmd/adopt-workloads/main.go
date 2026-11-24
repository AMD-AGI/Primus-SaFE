/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Command adopt-workloads imports the unfinished Workloads of the SaFE running on an adopted
// cluster into the admin plane. Owners that match an admin plane user by id and name are kept;
// other owners are taken from a user map file keyed by data plane user name, whose values are
// admin plane user ids. Workloads the data plane SaFE has not scheduled are imported only with
// -include-unscheduled. Without -apply it only prints the plan.
//
//	adopt-workloads -cluster ccs300 -data-kubeconfig ccs300.conf [-kubeconfig admin.conf] [-user-map users.yaml] [-include-unscheduled] [-apply]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/adopt"
)

func main() {
	kubeconfig := flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "kubeconfig of the admin plane")
	dataKubeconfig := flag.String("data-kubeconfig", "", "kubeconfig of the adopted cluster")
	cluster := flag.String("cluster", "", "name of the adopted cluster on the admin plane")
	userMapFile := flag.String("user-map", "", "YAML or JSON map from data plane user name to admin plane user id")
	apply := flag.Bool("apply", false, "import the workloads; without it only the plan is printed")
	includeUnscheduled := flag.Bool("include-unscheduled", false,
		"also import workloads the data plane SaFE has not scheduled; use once it no longer runs")
	flag.Parse()
	if *cluster == "" || *dataKubeconfig == "" {
		fail(fmt.Errorf("-cluster and -data-kubeconfig are required"))
	}
	fail(run(context.Background(), *kubeconfig, *dataKubeconfig, *cluster, *userMapFile, *apply, *includeUnscheduled))
}

// run prints the import plan for the cluster and imports the planned workloads when apply is set.
func run(ctx context.Context, kubeconfig, dataKubeconfig, cluster, userMapFile string, apply, includeUnscheduled bool) error {
	userMap, err := readUserMap(userMapFile)
	if err != nil {
		return err
	}
	adminClient, err := newAdminClient(kubeconfig)
	if err != nil {
		return err
	}
	dataConfig, err := clientcmd.BuildConfigFromFlags("", dataKubeconfig)
	if err != nil {
		return err
	}
	dataClient, err := dynamic.NewForConfig(dataConfig)
	if err != nil {
		return err
	}
	locals, err := adopt.ListUnfinishedWorkloads(ctx, dataClient)
	if err != nil {
		return err
	}
	plans, err := adopt.PlanImports(ctx, adminClient, locals, userMap, includeUnscheduled)
	if err != nil {
		return err
	}
	out := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(out, "WORKLOAD\tWORKSPACE\tPHASE\tLOCAL USER\tADMIN USER\tACTION\tRESULT")
	failed := 0
	for _, plan := range plans {
		admin, result := "", ""
		if plan.User != nil {
			admin = fmt.Sprintf("%s(%s)", v1.GetUserName(plan.User), plan.User.Name)
		}
		if apply && (plan.Action == adopt.ActionMatched || plan.Action == adopt.ActionMapped) {
			created, err := adopt.ImportWorkload(ctx, adminClient, cluster, plan.Workload, plan.User)
			switch {
			case err != nil:
				result, failed = "error: "+err.Error(), failed+1
			case created:
				result = "imported"
			default:
				result = "exists"
			}
		}
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", plan.Workload.Name, plan.Workload.Spec.Workspace,
			plan.Workload.Status.Phase, v1.GetUserName(plan.Workload), admin, plan.Action, result)
	}
	if err = out.Flush(); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d workloads failed to import", failed)
	}
	return nil
}

// readUserMap reads the data plane user name to admin plane user id map, or returns none without a file.
func readUserMap(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	userMap := map[string]string{}
	if err = yaml.Unmarshal(data, &userMap); err != nil {
		return nil, fmt.Errorf("parse user map %s: %w", path, err)
	}
	return userMap, nil
}

// newAdminClient returns a client of the admin plane that knows the SaFE types.
func newAdminClient(kubeconfig string) (client.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	if err = v1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return client.New(config, client.Options{Scheme: scheme})
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adopt-workloads:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
