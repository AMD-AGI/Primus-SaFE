#!/bin/bash

# AMD provided the recipe and Chet has scripted it.
# For producion, this will be incorporated in the oca-plugin.

version="1.7"
prog=$(realpath $0)
#rdma_mtu=4096
eth_mtu=9000
vf_mtu=9000


echo "Script ${prog} version:${version}"

#port_list=$(nicctl show port | grep 'Port :' | awk '{print $3}')

#for port_id in ${port_list}; do
#echo "Configuring port:${port_id}"
#nicctl update port --port ${port_id} --admin-state up --fec-type rs --mtu 9216 --pause-type pfc --rx-pause enable --tx-pause enable --speed 400g
#done

# required for a-27 fw - Chet Loke
#nicctl update port --all --mtu 9000

nicctl update qos dscp-to-purpose --dscp 46 --purpose rdma-ack

#no longer required since jumbo-mtu is phase2.
#nicctl update lif --rdma-mtu ${rdma_mtu}


#Following configs are configured via AINIC_PERSONA=1 during host-sw install.
#nicctl update qos --classification-type dscp
#nicctl update qos dscp-to-priority --dscp 10 --priority 0
#nicctl update qos dscp-to-priority --dscp 46 --priority 6
#nicctl update qos dscp-to-priority --dscp 0-9,11-45,47-63 --priority 1
#nicctl update qos pfc --priority 0 --no-drop enable
#port-mtu is set to 9000B

# qos scheduling config will always be volatile.
nicctl update qos scheduling --priority 0,1,6 --dwrr 99,1,0 --rate-limit 0,0,10

vf_ndevs=$(rdma link | grep '_vf' | awk '{print $NF}')

for vf_ndev in ${vf_ndevs};
do
    ip link set mtu ${vf_mtu} ${vf_ndev}
done

ndevs=$(rdma link | grep 'enp' | awk '{print $NF}')

for ndev in ${ndevs};
do
    ip link set mtu ${eth_mtu} ${ndev}
done

nicctl debug update pipeline internal rdma --skip-data-copy disable

#configure dcqcn
bash oci-pollara-cfg-dcqcn.sh

#Disable ACS
bash oci-pollara-acs-dis.sh

# Run OCI lab specific settings.
bash oci-nic-cfg-for-oci-env.sh

echo "Script $0 version:${version} done"
