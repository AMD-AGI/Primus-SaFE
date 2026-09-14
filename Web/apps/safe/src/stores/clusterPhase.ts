const USABLE_CLUSTER_PHASES = new Set(['Ready', 'Upgrading', 'UpgradeFailed'])

// Reports whether the cluster data plane remains available.
export const isUsableClusterPhase = (phase?: string) => !!phase && USABLE_CLUSTER_PHASES.has(phase)

// Returns the visual status type for a cluster phase.
export const clusterPhaseTagType = (phase?: string) => {
  if (phase === 'Ready') return 'success'
  if (phase === 'Upgrading') return 'warning'
  return 'danger'
}
