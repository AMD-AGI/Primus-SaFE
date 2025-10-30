#!/usr/bin/env bash

set -Eeuo pipefail

example_line="$(basename "$0") --mount /shared_nfs --hosts node01,node02,node03 --engine psync|libaio|io_uring --runtime [seconds] --job_id [optional job id]"

usage() {
  cat <<EOF
Usage:
  $(basename "$0") $example_line

Example:
  $example_line

Options:
  --mount    Path to a directory (ideally a mount point)
  --hosts    Comma-separated list of hostnames (letters, digits, dot, dash, underscore)
  -h, --help Show this help and exit
EOF
}

err() {
  echo "Error: $1" >&2
  echo "Example: $example_line" >&2
  exit 2
}

echo "All args: $@"

mount_path=""
hosts_csv=""
engine=psync
runtime=10
job_id=""
run_mdtest=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage; exit 0
      ;;
    --mount)
      [[ $# -ge 2 ]] || err "--mount requires an argument"
      mount_path="$2"; shift 2
      ;;
    --mount=*)
      mount_path="${1#*=}"; shift
      ;;
    --hosts)
      [[ $# -ge 2 ]] || err "--hosts requires an argument"
      hosts_csv="$2"; shift 2
      ;;
    --hosts=*)
      hosts_csv="${1#*=}"; shift
      ;;
    --engine)
      [[ $# -ge 2 ]] || err "--engine requires an argument"
      engine="$2"; shift 2
      ;;
    --engine=*)
      engine="${1#*=}"; shift
      ;;
    --runtime)
      [[ $# -ge 2 ]] || err "--runtime requires an argument"
      runtime="$2"; shift 2
      ;;
    --runtime=*)
      runtime="${1#*=}"; shift
      ;;
    --job_id)
      [[ $# -ge 2 ]] || err "--job_id requires an argument"
      job_id="$2"; shift 2
      ;;
    --job_id=*)
      job_id="${1#*=}"; shift
                ;;
    --run_mdtest)
      [[ $# -ge 2 ]] || err "--run_mdtest requires an argument"
      run_mdtest="$2"; shift 2
      ;;
    --run_mdtest=*)
      run_mdtest="${1#*=}"; shift
      ;;
         --) shift; break ;;
    -*)
      err "Unknown option: $1"
      ;;
    *)
      err "Unexpected positional argument: $1"
      ;;
  esac
done


[[ -n "$mount_path" ]] || err "Missing --mount"
[[ -n "$hosts_csv"  ]] || err "Missing --hosts"

# Validate mount path exists
if [[ ! -d "$mount_path" ]]; then
  err "--mount path does not exist or is not a directory: $mount_path"
fi


# Validate hosts CSV (no spaces; allowed chars only)
if [[ "$hosts_csv" =~ [[:space:]] ]]; then
  err "--hosts must be comma-separated with no spaces"
fi
if [[ ! "$hosts_csv" =~ ^[A-Za-z0-9._-]+(,[A-Za-z0-9._-]+)*$ ]]; then
  err "--hosts must be a comma-separated list of hostnames (letters, digits, dot, dash, underscore)"
fi


if [[ ! "$runtime" =~ ^[0-9]+$ ]]; then
  err "--runtime must be an integer"
fi


# Parse hosts into an array if you need them later
IFS=',' read -r -a HOSTS <<< "$hosts_csv"

current_dir="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$current_dir"

if [[ $job_id == "" ]]; then
        JOB_ID="$(date +"%Y%m%d-%H%M%S")"
else
    JOB_ID=$job_id
fi

#prepare output directory
FIO_DATA_DIR="${mount_path}/fio_data"
FIO_OUTPUT_DIR="${mount_path}/fio/${JOB_ID}/$(hostname)"
MDTEST_DATA_DIR="${mount_path}/mdtest_data"
MDTEST_OUTPUT_DIR="${mount_path}/mdtest/${JOB_ID}"

rm -rf $FIO_DATA_DIR $MDTEST_DATA_DIR
mkdir -p $FIO_DATA_DIR $FIO_OUTPUT_DIR $MDTEST_DATA_DIR $MDTEST_OUTPUT_DIR

IFS=$'\n'; echo "${HOSTS[*]}" > hosts.list

case "$engine" in
   psync)
      iodepth=1
      ;;
   io_uring|libaio)
      iodepth=8
      ;;
esac

PATH="/root/bin:$PATH"

for s in `cat fio_write_sections.txt`; do
   if [[ "$s" != \#* ]]; then
      printf "\n========      Start FIO test: %20s %s     ========\n" $s "with "${#HOSTS[@]}" nodes"
      RESULT_FILE="$FIO_OUTPUT_DIR"/"$s".json
      
      DATA_DIR="$FIO_DATA_DIR" OUTPUT_DIR="$FIO_OUTPUT_DIR" JOB_NAME="$s" ENGINE="$engine" IODEPTH="$iodepth" RUNTIME="$runtime" \
	      fio --client=hosts.list --section="$s"  --output-format=json --eta=never --output=$RESULT_FILE fio_write.fio


      n=2
      write_bw=$(jq -cer '.client_stats[-1].write.bw / 1000' $RESULT_FILE) #MBps
      write_iops=$(jq -cer '.client_stats[-1].write.iops' $RESULT_FILE)
      write_lat=$(jq -cer '.client_stats[-1].write.lat_ns.mean / 1000000' $RESULT_FILE) #ms
      printf "Write throughput\t %.${n}f MBps\tIOPS: %.${n}f\tLatency: %.${n}f ms  \n" $write_bw $write_iops $write_lat

      read_bw=$(jq -cer '.client_stats[-1].read.bw / 1000' $RESULT_FILE) #MBps
      read_iops=$(jq -cer '.client_stats[-1].read.iops' $RESULT_FILE)
      read_lat=$(jq -cer '.client_stats[-1].read.lat_ns.mean / 1000000' $RESULT_FILE) #ms
      printf "Read throughput\t\t %.${n}f MBps\tIOPS: %.${n}f\tLatency: %.${n}f ms  \n" $read_bw $read_iops $read_lat
   fi
done

rm -rf $FIO_DATA_DIR/*

for s in `cat fio_read_sections.txt`; do
   if [[ "$s" != \#* ]]; then
      printf "\n========      Start FIO test: %30s %s     ========\n" $s  "with "${#HOSTS[@]}" nodes"

      RESULT_FILE="$FIO_OUTPUT_DIR"/"$s".json
      DATA_DIR="$FIO_DATA_DIR" OUTPUT_DIR="$FIO_OUTPUT_DIR" JOB_NAME="$s" ENGINE="$engine" IODEPTH="$iodepth" RUNTIME="$runtime" \
              fio --client=hosts.list --section="$s" --output-format=json --eta=never  --output=$RESULT_FILE fio_read.fio

      n=2
      write_bw=$(jq -cer '.client_stats[-1].write.bw / 1000' $RESULT_FILE) #MBps
      write_iops=$(jq -cer '.client_stats[-1].write.iops' $RESULT_FILE)
      write_lat=$(jq -cer '.client_stats[-1].write.lat_ns.mean / 1000000' $RESULT_FILE) #ms
      printf "Write throughput\t %.${n}f MBps\tIOPS: %.${n}f\tLatency: %.${n}f ms  \n" $write_bw $write_iops $write_lat

      read_bw=$(jq -cer '.client_stats[-1].read.bw / 1000' $RESULT_FILE) #MBps
      read_iops=$(jq -cer '.client_stats[-1].read.iops' $RESULT_FILE)
      read_lat=$(jq -cer '.client_stats[-1].read.lat_ns.mean / 1000000' $RESULT_FILE) #ms
      printf "Read throughput\t\t %.${n}f MBps\tIOPS: %.${n}f\tLatency: %.${n}f ms  \n" $read_bw $read_iops $read_lat

      rm -rf $FIO_DATA_DIR/*
   fi
done

echo && echo "========       FIO tests completed at" "$FIO_OUTPUT_DIR" "      ========"

if [[ $run_mdtest == "0" ]]; then
        exit 0
fi

#--------------- MDTEST   ------------
number_of_files=100
number_of_iterations=10
mdtest_output_file="$MDTEST_OUTPUT_DIR"/mdtest.txt

printf "========       Start mdtest with %s nodes %s files and %s runs      ========\n" ${#HOSTS[@]} $number_of_files $number_of_iterations
mpirun --allow-run-as-root  -n ${#HOSTS[@]} -host  "${hosts_csv}"  \
        /bin/bash -c "mdtest -d $MDTEST_DATA_DIR -n $number_of_files -i $number_of_iterations" > $mdtest_output_file

#convert txt output into json
mdtest_output_json="$MDTEST_OUTPUT_DIR"/mdtest.json
echo "{ \"results\":" > $mdtest_output_json
tail -12 $mdtest_output_file | head -10 | \
        awk '{print  "{\"" $1 "_" $2 "\": {\"stats\": {\"max\": " $3 ", \"min\": " $4  ", \"mean\":" $5 ", \"std_dev\":" $6 "}}}"}'|jq . -s \
        >> $mdtest_output_json
echo "}" >> $mdtest_output_json

echo && tail -14 $mdtest_output_file | head -12 && echo

printf "========      mdtest completed at %s      ========\n" $MDTEST_OUTPUT_DIR
