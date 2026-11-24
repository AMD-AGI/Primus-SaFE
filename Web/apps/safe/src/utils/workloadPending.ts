import { WorkloadKind } from '@/services/workload/type'

// Kinds that run as long-lived applications; their NotReady phase is treated as pending.
const APPLICATION_KINDS: string[] = [
  WorkloadKind.Deployment,
  WorkloadKind.StatefulSet,
  WorkloadKind.DynamoDeployment,
  WorkloadKind.InferaDeployment,
  WorkloadKind.Atom,
  WorkloadKind.SGLangRouter,
  WorkloadKind.GithubRunner,
]

interface PendingCandidate {
  phase?: string
  kind?: string
  groupVersionKind?: { kind?: string }
}

// Returns true when the pending cause analysis applies: Pending, or NotReady for an application.
export const canAnalyzePending = (workload?: PendingCandidate): boolean => {
  if (!workload) return false
  if (workload.phase === 'Pending') return true
  const kind = workload.groupVersionKind?.kind || workload.kind || ''
  return workload.phase === 'NotReady' && APPLICATION_KINDS.includes(kind)
}
