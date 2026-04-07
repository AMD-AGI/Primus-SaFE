#!/bin/sh
echo "Starting Monarch Worker ..."
python -u -c "
import os
import logging

from monarch.actor import run_worker_loop_forever
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger('monarch-worker')
port = os.environ.get('MONARCH_PORT', '26600')
pod_ip = os.environ.get('POD_IP', '0.0.0.0')
address = 'tcp://' + pod_ip + ':' + port
logger.info('--- Starting Monarch Worker (replica0) ---')
logger.info('POD_IP: ' + pod_ip)
logger.info('PYTHONPATH: ' + os.environ.get('PYTHONPATH', 'not set'))
logger.info('Listening on: ' + address)
run_worker_loop_forever(address=address, ca='trust_all_connections')
"