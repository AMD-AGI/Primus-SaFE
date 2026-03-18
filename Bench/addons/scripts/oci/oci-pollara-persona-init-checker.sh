#!/bin/bash

# ################
#
# If you change this script then Matt Field's team
# must be notified so they can update their oca-plugin
# code.
#
# ################

#
#
# Adding GA7 OCI persona config checker script.

# OCA-plugin or any tool configuring RDMA NIC configuration,
# must first run this checker and check for the return code.
# Any RDMA configuration must be applied only if the return
# code is zero.

# return-code: non-zero implies, persona configuration checker
#              failed. Check the error message reason.
#              Try re-running again after 15 secs and no
#              more than 20 attempts: total of 300 secs.
#              If the checker still returns non-zero return
#              code after 20 attempts then could indicate a
#              persistent software(Pollara)/hardware issue.
# return-code: zero implies, persona config is applied across all
#              the Pollara NICs.


version="1.0"
prog=$(realpath $0)

exp_nic_cnt=8
nic_model='salina'
rc=0
verbose=0

[[ ! "$(command -v jq)" ]] && { printf "ERROR: jq util doesn't exist. Please install and re-run this checker.\n"; exit 1; }

[[ $# -ge 1 ]] && { verbose=1; }

out=$(nicctl show card)
rc=$?

if [ ${rc} -ne 0 ];
then
    printf "ERROR: nicctl show card returned error:${rc}. Exiting.\n"
    exit 1
fi

curr_nics=$(echo "${out}" | grep -c ${nic_model})
if [[ ${curr_nics} -ne ${exp_nic_cnt} ]];
then
    printf "ERROR: Expected-nics:${exp_nic_cnt} discovered-nics:${curr_nics} for nic_model:${nic_model}. Exiting.\n"
    printf "output of nicctl show card:\n${out}\n"
    exit 1
fi

jout=$(nicctl show card -j)
rc=$?

if [ ${rc} -ne 0 ];
then
    printf "ERROR: nicctl show card -j returned error:${rc}. Exiting.\n"
    exit 1
fi

out=$(echo "${jout}" | jq -r '.nic[] | "\(.pcie_bdf) \(.generation_id)"')
rc=$?

if [ ${rc} -ne 0 ];
then
    printf "ERROR: jq parsing returned error:${rc}. Exiting.\n"
    exit 1
fi

echo "${out}" | while read -r bdf gen_id;
do
    file_id=$(cat /etc/amd/ainic/$bdf/nicctl_card_state.txt)
    if [[ "${gen_id}" = "${file_id}" ]];
    then
        [[ ${verbose} -eq 1 ]] && { printf "On $bdf persona based config is applied.\n"; }
    else
        printf "WARN: for $bdf: persona based config is not applied yet.\n"
        printf "  generation_id: $gen_id\n"
        printf "  file_id:       $file_id\n"
        rc=1
    fi
done

[[ ${rc} -eq 0 ]] && { printf "OCI persona config is applied across all ${exp_nic_cnt} nics.\n"; }
exit ${rc}
