#! /usr/bin/env bash
set -Eeuo pipefail

DS_NAME="${DS_NAME:-storage-benchmark}"
#IMAGE_NAME="${IMAGE_NAME:-percyzhao/benchmark:0.47}"
#PVC_NAME="${PVC_NAME:-juicefs-pvc-bench}"
#PVC_SIZE="${PVC_SIZE:-200Gi}"
FORCE="${FORCE:-false}"
ENGINE="${ENGINE:-psync}"
RUNTIME="${RUNTIME:-30}"
IPs="${IPs:-""}"
NAMESPACE="${NAMESAPCE:="default"}"

: "${IMAGE_NAME:?IMAGE_NAME environment variable is required}"
: "${MOUNT:?MOUNT environment variable is required}"

usage() {
  cat <<EOF
Usage:
  $(basename "$0")

Options:
   -h, --help Show this help and exit
   -f, --force Run benchmark on the available nodes when the daemonset is not ready yet
EOF
}

while [[ $# -gt 0 ]]; do
   case "$1" in
      -h|--help)
         usage; exit 0
         ;;
      -f|--force)
         FORCE=true; shift
         ;;
      --) shift; break ;;
      -*)
         usage; exit 2
         ;;
      *)
         usage; exit 2
         ;;
   esac
done



echo "Creating Benchmark Daemonset"
kubectl -n ${NAMESPACE} apply -f - <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: ${DS_NAME}
   namespace: ${NAMESPACE}
spec:
  selector:
    matchLabels:
      app: benchmark-worker
  template:
    metadata:
      labels:
        app: benchmark-worker
    spec:
      serviceAccountName: default
      volumes:
      - name: target
        hostPath: 
            path: /${MOUNT}
            type: Directory
      containers:
      - name: benchmark-container
        image: ${IMAGE_NAME}
        command: ["/bin/sh", "-c", "tail -f /dev/null"]
        ports:
        - containerPort: 22
          name: ssh
        volumeMounts:
        - mountPath: /${MOUNT}
          name: target
          

EOF

run_bench() {
   echo "Start benchmarking"

   pod="$(kubectl -n ${NAMESPACE} get pod -l app=benchmark-worker -o json | jq -r '.items[] | select(any(.status.conditions[]; .type=="Ready" and .status=="True")) | "\(.metadata.name)"' | head -1)"

   if [[ $IPs == "" ]]; then
      IPs="$(kubectl  -n ${NAMESPACE} get pod -l app=benchmark-worker -o json | jq -r '[.items[] | select(any(.status.conditions[]; .type=="Ready" and .status=="True")) | .status.podIP] | join(",")')"
   fi

   echo "using pod:" $pod "and IPs:" $IPs

   cmd="/root/bench.sh"

   kubectl -n ${NAMESPACE} exec $pod -- $cmd --mount ${MOUNT} --hosts $IPs --engine $ENGINE --runtime $RUNTIME --run_mdtest=1
}


#Checking DS status
read -r scheduled available <<<"$(kubectl -n ${NAMESPACE} get ds ${DS_NAME} -o jsonpath='{.status.currentNumberScheduled} {.status.numberAvailable}')"


if [[ "$scheduled" == "$available" ]]; then
   run_bench
else
   if [[ $FORCE == "true" ]]; then
      if [[ $available == 0 ]]; then
         echo "No nodes available yet"
         exit 1
      fi
      run_bench
   else
      printf "%s/%s nodes in the daemonset are available, please use -f flag to start benchmarking with the available nodes\n" $available $scheduled
   fi
fi