#! /usr/bin/env bash

#SBATCH -J fio-cs
#SBATCH -N 4                   # <— number of nodes you want
#SBATCH --ntasks-per-node=1
#SBATCH -t 00:30:00
#SBATCH -o slurm.%x.%j.out
#SBATCH -e slurm.%x.%j.err

set -euo pipefail

### === USER CONFIG ===
: "${IMAGE:?IMAGE environment variable is required}"

#export IMAGE="percyzhao/benchmark:0.51"
export IMAGE="${IMAGE}"
export FIO_PORT=8765
export MOUNT="${MOUNT:-/mnt/weka}"
export ENGINE="${ENGINE:-psync}"
export RUNTIME="${RUNTIME:-10}"
export IODEPTH="${IODEPTH:-1}"
export IP_CSV=""


if [[ $ENGINE == "psync" && $IODEPTH != 1 ]]; then
   IODEPTH=1
fi

mapfile -t NODES < <(scontrol show hostnames "${SLURM_NODELIST}")
HEAD_NODE="${NODES[0]}"
NODES_CSV=$(echo ${NODES[@]} | tr ' ' ',')
for n in "${NODES[@]}"; do
        ip=$(srun --nodes=1 --ntasks=1 -w "$n" hostname -i);
        IP_CSV+=$ip,
done
IP_CSV="${IP_CSV%,}"


#printf "Start FIO benchmark testing with the following configs:\
#        \nMount: %s \nEngine: %s \nIO Depth: %s \nRuntime: %s \nHead node: %s \nNodes: %s\nIPs: %s\n" \
#        $MOUNT $ENGINE $IODEPTH $RUNTIME $HEAD_NODE $NODES_CSV $IP_CSV


start_server_cmd() {
  cat <<'EOS'
set -euo pipefail
NAME="fio_srv_${SLURM_JOB_ID}"
# Stop any previous containers
#docker ps -a | grep "fio_srv_" | awk '{print $1}' | xargs --no-run-if-empty docker rm -f
# Start server in detached mode with host networking
docker run -d --name "${NAME}" --rm  --privileged --network host \
  -v "$PWD":/work -v "$MOUNT":"$MOUNT" -w /work \
  "${IMAGE}" \
  /root/bin/fio --server=:"${FIO_PORT} &"
EOS
}

echo "[+] Starting fio servers on ${NODES_CSV} nodes (port ${FIO_PORT})..."
srun --ntasks=${#NODES[@]} --ntasks-per-node=1 bash -lc "$(start_server_cmd)"

#wait fio server to start
sleep 5



run_bench_cmd(){
        cat <<'EOS'
set -euo pipefail
NAME="fio_srv_${SLURM_JOB_ID}"
docker exec -i "${NAME}" /root/bench.sh --mount="${MOUNT}" --hosts="${IP_CSV}" \
--engine="${ENGINE}" --runtime="${RUNTIME}"  --job_id="${SLURM_JOB_ID}"
EOS
}

run_bench_mdtest_cmd(){
        cat <<'EOS'
set -euo pipefail
NAME="fio_srv_${SLURM_JOB_ID}"
NUMBER_OF_FILES=100
NUMBER_OF_ITERATIONS=10
MDTEST_DATA_ROOT=$MOUNT/mdtest_data
MDTEST_DATA_DIR=$MDTEST_DATA_ROOT/$(hostname)
MDTEST_OUTPUT_DIR=$MOUNT/mdtest/${SLURM_JOB_ID}
MDTEST_OUTPUT_FILE=$MDTEST_OUTPUT_DIR/mdtest_$(hostname).txt

docker exec -i ${NAME} bash -lc "mkdir -p $MDTEST_OUTPUT_DIR $MDTEST_DATA_DIR && /root/bin/mdtest -u -d $MDTEST_DATA_DIR -n $NUMBER_OF_FILES -i $NUMBER_OF_ITERATIONS > $MDTEST_OUTPUT_FILE && rm -rf ${MDTEST_DATA_ROOT}/*"
EOS
}

parse_mdtest_result_cmd(){
        cat <<'EOS'
set -euo pipefail
MDTEST_OUTPUT_DIR=$MOUNT/mdtest/${SLURM_JOB_ID}
printf "["
for f in `ls $MDTEST_OUTPUT_DIR`; do
   host=$(echo "$f" | sed -E 's/mdtest_(.*)\.txt/\1/')
   if grep -q "finished" $MDTEST_OUTPUT_DIR/$f; then
      err=0
   else
      err=1
   fi

   printf "{\"host\": \"%s\", \"err\":%s, " $host $err
   if [[ $err == 1 ]]; then
      printf "},"
      continue
   fi

   tail -12 $MDTEST_OUTPUT_DIR/$f | head -10 | while IFS= read -r line; do
      echo $line | awk '{
      key_name = $1 " " $2
      min = $3
      max = $4
      mean = $5
      std_dev = $6

      printf "\"%s\":{\"min\":%s,\"max\":%s,\"mean\":%s,\"std_dev\":%s},\n", key_name, min, max, mean, std_dev
   }'
   done
   printf "},"
done
printf "]"

EOS
}

#run fio on the head node
echo "[+] Running fio client on head node: ${HEAD_NODE}"
srun -N1 -n1 -w "${HEAD_NODE}" bash -lc "$(run_bench_cmd)"

#echo "[+] Running mdtest "
#srun --mpi=pmi2 -N ${#NODES[@]} -n ${#NODES[@]} bash -lc "$(run_bench_mdtest_cmd)"

#parse the mdtest result
#srun -N1 -n1 -w "${HEAD_NODE}" bash -lc "$(parse_mdtest_result_cmd) > $MOUNT/mdtest/${SLURM_JOB_ID}/mdtest.json"

stop_server_cmd() {
  cat <<'EOS'
set -euo pipefail
NAME="fio_srv_${SLURM_JOB_ID}"
docker rm -f "${NAME}" >/dev/null 2>&1 || true
EOS
}

cleanup() {
  echo "[*] Cleaning up fio servers on all nodes..."
  srun --ntasks=${#NODES[@]} --ntasks-per-node=1 bash -lc "$(stop_server_cmd)" || true
}

trap cleanup EXIT