#!/bin/bash
nicctl update port --all --pause-type pfc --rx-pause enable --tx-pause enable
nicctl update qos --classification-type pcp
nicctl update qos --classification-type dscp
nicctl update qos dscp-to-priority --dscp 10 --priority 0
nicctl update qos dscp-to-priority --dscp 46 --priority 6
nicctl update qos dscp-to-priority --dscp 0-9,11-45,47-63 --priority 1
nicctl update qos dscp-to-purpose --dscp 46 --purpose rdma-ack
nicctl update qos pfc --priority 0 --no-drop enable
nicctl update qos scheduling --priority 0,1,6 --dwrr 99,1,0 --rate-limit 0,0,10
nicctl update port --all --mtu 9000