
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.32/deploy/local-path-storage.yaml

if [ -z "$DIR" ]
then
  echo "Please enter nfs directory or local directory."
  read DIR
fi

file="$DIR/test"
if ! echo "test" > "$file" 2>/dev/null; then
    echo "❌ Cannot write to $DIR (maybe read-only)"
    exit 1
fi

# Clean up
rm -f "$file"

cat <<EOYAML | kubectl apply -f -
kind: ConfigMap
apiVersion: v1
metadata:
  name: local-path-config
  namespace: local-path-storage
data:
  config.json: |-
        {
                "nodePathMap":[
                {
                        "node":"DEFAULT_PATH_FOR_NON_LISTED_NODES",
                        "paths":["$DIR"]
                }
                ]
        }
  setup: |-
        #!/bin/sh
        set -eu
        mkdir -m 0777 -p "\$VOL_DIR"
  teardown: |-
        #!/bin/sh
        set -eu
        rm -rf "\$VOL_DIR"
  helperPod.yaml: |-
        apiVersion: v1
        kind: Pod
        metadata:
          name: helper-pod
        spec:
          priorityClassName: system-node-critical
          tolerations:
            - key: node.kubernetes.io/disk-pressure
              operator: Exists
              effect: NoSchedule
          containers:
          - name: helper-pod
            image: busybox
EOYAML
