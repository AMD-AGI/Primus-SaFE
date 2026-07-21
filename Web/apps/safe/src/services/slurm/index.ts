import request from '@/services/request'
import type {
  SlurmClusterItem,
  SlurmClusterListResp,
  CreateSlurmClusterData,
  EditSlurmClusterData,
  SlurmLoginInfo,
} from './type'

// Slurm clusters are per-workspace Helm releases of the Slinky `slurm` chart
// (v1.2.0), deployed into the workspace namespace via the SaFE Addon mechanism.
// All operations are scoped to a workspace (workspaceId) on a target cluster.

export const getSlurmClusterList = (
  clusterId: string,
  workspaceId: string,
): Promise<SlurmClusterListResp> =>
  request.get(`/clusters/${clusterId}/slurmclusters`, { params: { workspaceId } })

export const getSlurmClusterDetail = (
  clusterId: string,
  name: string,
  workspaceId: string,
): Promise<SlurmClusterItem> =>
  request.get(`/clusters/${clusterId}/slurmclusters/${name}`, { params: { workspaceId } })

export const createSlurmCluster = (clusterId: string, data: CreateSlurmClusterData) =>
  request.post(`/clusters/${clusterId}/slurmclusters`, data)

export const editSlurmCluster = (
  clusterId: string,
  name: string,
  workspaceId: string,
  data: EditSlurmClusterData,
) =>
  request.patch(`/clusters/${clusterId}/slurmclusters/${name}`, data, {
    params: { workspaceId },
  })

export const deleteSlurmCluster = (clusterId: string, name: string, workspaceId: string) =>
  request.delete(`/clusters/${clusterId}/slurmclusters/${name}`, { params: { workspaceId } })

// Fetch a ready-to-copy SSH command for the cluster's login node.
export const getSlurmClusterLogin = (
  clusterId: string,
  name: string,
  workspaceId: string,
): Promise<SlurmLoginInfo> =>
  request.get(`/clusters/${clusterId}/slurmclusters/${name}/login`, { params: { workspaceId } })

// Stop a cluster: scales its components to zero while keeping it in the list as
// history (status becomes "Stopped"). Resume restores the saved node counts.
export const stopSlurmCluster = (clusterId: string, name: string, workspaceId: string) =>
  request.post(`/clusters/${clusterId}/slurmclusters/${name}/stop`, null, {
    params: { workspaceId },
  })

export const resumeSlurmCluster = (clusterId: string, name: string, workspaceId: string) =>
  request.post(`/clusters/${clusterId}/slurmclusters/${name}/resume`, null, {
    params: { workspaceId },
  })
