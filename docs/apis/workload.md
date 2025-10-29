# Workload API Documentation

## Overview

The Workload API is a core set of interfaces for managing workloads, enabling users to create, manage, monitor, and operate various types of computing tasks. These APIs support multiple workload types, including machine learning training jobs (PyTorchJob), deployed services (Deployment), stateful applications (StatefulSet), and development machines (Authoring).

## API Endpoints
### Create Workload

#### Request
```http
POST /api/v1/workloads
Content-Type: application/json
```


**Request Body (CreateWorkloadRequest):**
```json
{
  // The Workload name(display only). Used to generate the workload id,
  // which will do normalization processing, such as lowercase and random suffix
  "displayName": "string",
  // The workload description
  "description": "string",
  // Workspace ID to which the workload is delivered
  "workspaceId": "string",
  // When specifying a workload run on nodes, the replica count will be overwritten with the node count.
  "specifiedNodes": ["string"],
  // Workload resource requirements
  "resource": {
    // Number of requested nodes
    "replica": 0,
    // Requested CPU core count (e.g., 128)
    "cpu": "string",
    // Requested GPU card count (e.g., 8)
    "gpu": "string",
    // Requested Memory size (e.g., 128Gi)
    "memory": "string",
    // Ephemeral-storage for pod. Default is 50Gi
    "ephemeralStorage": "string"
  },
  // The address of the image used by the workload. e.g., docker.io/your-group/xxx
  "image": "string",
  // Workload startup command, required in base64 encoding
  "entryPoint": "string",
  // Supervision flag for the workload. When enabled, it performs operations like hang detection
  "isSupervised": false,
  // Workload scheduling priority. Defaults is 0, valid range: 0–2
  "priority": 0,
  // Workload timeout in seconds. Default is 0 (no timeout).
  "timeout": 0,
  // Failure retry limit. default: 0
  "maxRetry": 0,
  // The lifecycle of the workload after completion, in seconds. Default is 60.
  "ttlSecondsAfterFinished": 60,
  // The workload will run on nodes with the user-specified labels.
  // If multiple labels are specified, all of them must be satisfied.
  "customerLabels": {
    "key": "string"
  },
  // Environment variable for workload
  "env": {
    "key": "string"
  },
  // K8s liveness check. used for deployment/statefulSet
  "liveness": {
    // The path for health check
    "path": "string",
    // Service port for health detect
    "port": 0,
    // Initial delay seconds. default is 600s
    "initialDelaySeconds":600,
    // Period check interval. default is 3s
    "periodSeconds": 3,
    // Failure retry limit. default is 3
    "failureThreshold": 3
  },
  // K8s readiness check. used for deployment/statefulSet, like liveness
  "readiness": {
  },
  "service": {
    // TCP(default) or UDP
    "protocol": "TCP",
    // Service port for external access
    "port": 0,
    // K8s node port
    "nodePort": 0,
    // Pod service listening port
    "targetPort": 0,
    // The type of service, such as ClusterIP, NodePort
    "serviceType": "string",
    // Extended environment variable
    "extends": {}
  },
  // Dependencies defines a list of other Workloads that must complete successfully
  // before this Workload can start execution. If any dependency fails, this Workload
  // will not be scheduled and is considered failed.
  "dependencies": ["string"],
  // Cron Job configuration
  "cronJobs": [
    {
      // Scheduled execution time, such as "2025-09-30T16:04:00.000Z" or "0 3 * * *"
      // Note: Only minute-level input is supported; seconds are not supported.
      "schedule": "string",
      // The action to take when the schedule is triggered. e.g. start
      "action": "start"
    }
  ],
  // Version: version of workload, default value is v1
  // Kind: kind of workload, Valid values includes: PyTorchJob/Deployment/StatefulSet/Authoring, default is PyTorchJob
  "groupVersionKind": {
    "version": "v1",
    "kind": "PyTorchJob"
  },
  // Indicates whether the workload tolerates node taints
  "isTolerateAll": false
}
```


#### Response (CreateWorkloadResponse)
```json
{
  // The workload id
  "workloadId": "string"
}
```


### List Workloads

#### Request
```http
GET /api/v1/workloads?workspaceId=string&phase=string&clusterId=string&userId=string&userName=string&kind=string&description=string&offset=0&limit=100&sortBy=string&order=desc&since=string&until=string&workloadId=string
```


**Query Parameters:**
- `workspaceId`: Filter results by workspace id
- `phase`: Filter results by phase (Succeeded,Failed,Pending,Running,Stopped). Multiple values separated by commas
- `clusterId`: Filter results by cluster id
- `userId`: Filter results by user id
- `userName`: Filter results by username, supports fuzzy matching
- `kind`: Filter results by workload kind (Deployment/PyTorchJob/StatefulSet/Authoring). Multiple values separated by commas
- `description`: Filter results by workload description, supports fuzzy matching
- `offset`: Starting offset for the results. Default is 0
- `limit`: Limit the number of returned results. Default is 100
- `sortBy`: Sort results by the specified field. Default is create_time
- `order`: The sorting order. Valid values are "desc" (default) or "asc"
- `since`: Query the start time of the workload, based on the workload creation time (e.g. '2006-01-02T15:04:05.000Z')
- `until`: Query the end time of the workload, similar to since
- `workloadId`: The workload id, Supports fuzzy matching

#### Response (ListWorkloadResponse)
```json
{
  // The total number of node templates, not limited by pagination
  "totalCount": 0,
  "items": [
    {
      // The workload id
      "workloadId": "string",
      // The workspace which workload belongs to
      "workspaceId": "string",
      // The workload resource requirements
      "resource": {
        "replica": 0,
        "cpu": "string",
        "gpu": "string",
        "memory": "string",
        "ephemeralStorage": "string",
        "sharedMemory": "string"
      },
      // The workload name (display only)
      "displayName": "string",
      // The workload description
      "description": "string",
      // The user id of workload submitter
      "userId": "string",
      // The username of workload submitter
      "userName": "string",
      // The cluster which the workload belongs to
      "clusterId": "string",
      // The status of workload, such as Succeeded, Failed, Pending, Running, Stopped, Updating
      "phase": "string",
      // Shows the reason if the workload is in pending status.
      "message": "string",
      // Workload scheduling priority. Defaults is 0, valid range: 0–2
      "priority": 0,
      // The workload creation time
      "creationTime": "string",
      // The workload start time
      "startTime": "string",
      // The workload end time
      "endTime": "string",
      // The workload deletion time
      "deletionTime": "string",
      // The workload run time, Calculated from the start time. such as 1h2m3s or 1h15s
      "runtime": "string",
      // Seconds remaining before workload timeout. Only applicable if a timeout is set.
      // This is calculated from when the workload starts running. If it has not yet started, return -1.
      "secondsUntilTimeout": 0,
      // Show the queue position of the workload if it is pending.
      "schedulerOrder": 0,
      // Total dispatch count of workload
      "dispatchCount": 0,
      // Indicates whether the workload tolerates node taints
      "isTolerateAll": false,
      // Defines the group, version, and kind of the workload. Currently, the group is not used
      "groupVersionKind": {
        "version": "string",
        "kind": "string"
      },
      // Workload timeout in seconds. Default is 0 (no timeout).
      "timeout": 0,
      // Workload uid
      "workloadUid": "string",
      // K8s object uid corresponding to the workload
      "k8sObjectUid": "string"
    }
  ]
}
```


### Get Workload Details

#### Request
```http
GET /api/v1/workloads/{workloadId}
```


#### Response (GetWorkloadResponse)

```json
{
  // The workload id
  "workloadId": "string",
  // The workspace which workload belongs to
  "workspaceId": "string",
  // The workload resource requirements
  "resource": {
    "replica": 0,
    "cpu": "string",
    "gpu": "string",
    "memory": "string",
    "ephemeralStorage": "string",
    "sharedMemory": "string"
  },
  // The workload name (display only)
  "displayName": "string",
  // The workload description
  "description": "string",
  // The user id of workload submitter
  "userId": "string",
  // The username of workload submitter
  "userName": "string",
  // The cluster which the workload belongs to
  "clusterId": "string",
  // The status of workload, such as Succeeded, Failed, Pending, Running, Stopped, Updating
  "phase": "string",
  // Shows the reason if the workload is in pending status.
  "message": "string",
  // Workload scheduling priority. Defaults is 0, valid range: 0–2
  "priority": 0,
  // The workload creation time, such as "2006-01-02T15:04:05"
  "creationTime": "string",
  // The workload start time, such as "2006-01-02T15:04:05"
  "startTime": "string",
  // The workload end time, such as "2006-01-02T15:04:05"
  "endTime": "string",
  // The workload deletion time, such as "2006-01-02T15:04:05"
  "deletionTime": "string",
  // The workload run time, Calculated from the start time. such as 1h2m3s or 1h15s
  "runtime": "string",
  // Seconds remaining before workload timeout. Only applicable if a timeout is set.
  "secondsUntilTimeout": 0,
  // Show the queue position of the workload if it is pending.
  "schedulerOrder": 0,
  // Total dispatch count of workload
  "dispatchCount": 0,
  // Indicates whether the workload tolerates node taints
  "isTolerateAll": false,
  // Defines the group, version, and kind of the workload. Currently, the group is not used
  "groupVersionKind": {
    "group": "string",
    "version": "string",
    "kind": "string"
  },
  // Workload timeout in seconds. Default is 0 (no timeout).
  "timeout": 0,
  // Workload uid
  "workloadUid": "string",
  // K8s object uid corresponding to the workload
  "k8sObjectUid": "string",
  // The node specified by the user when creating the workload
  "specifiedNodes": [
    "string"
  ],
  // The address of the image used by the workload
  "image": "string",
  // Workload startup command, required in base64 encoding
  "entryPoint": "string",
  // Supervision flag for the workload. When enabled, it performs operations like hang detection
  "isSupervised": false,
  // Failure retry limit. default: 0
  "maxRetry": 0,
  // The lifecycle of the workload after completion, in seconds. Default to 60.
  "ttlSecondsAfterFinished": 60,
  // Detailed processing workflow of the workload
  "conditions": [
    {
      "type": "string",
      "status": "string",
      "reason": "string",
      "message": "string",
      "lastTransitionTime": "string"
    }
  ],
  // Pod info related to the workload
  "pods": [
    {
      // The podId
      "podId": "string",
      // The Kubernetes node that the Pod is scheduled on
      "k8sNodeName": "string",
      // The admin node that the Pod is scheduled on
      "adminNodeName": "string",
      // Pod status: Pending, Running, Succeeded, Failed, Unknown
      "phase": "string",
      // Pod start time
      "startTime": "string",
      // Pod end time
      "endTime": "string",
      // The node IP address where the Pod is running
      "hostIP": "string",
      // The pod IP address where the Pod is running
      "podIP": "string",
      // The rank of pod, only for pytorch-job
      "rank": "string",
      // SSH address to log in
      "sshAddr": "string",
      // The Container info of pod
      "containers": [
        {
          // Container name
          "name": "string",
          // (brief) reason from the last termination of the container
          "reason": "string",
          // Message regarding the last termination of the container
          "message": "string",
          // Exit status from the last termination of the container
          "exitCode": 0
        }
      ]
    }
  ],
  // The node used for each workload execution. If the workload is retried multiple times, there will be multiple entries.
  "nodes": [
    [
      "string"
    ]
  ],
  // The rank is only valid for the PyTorch job and corresponds one-to-one with the nodes listed above.
  "ranks": [
    [
      "string"
    ]
  ],
  // The workload will run on nodes with the user-specified labels.
  // If multiple labels are specified, all of them must be satisfied.
  "customerLabels": {
    "key": "string"
  },
  // Environment variables
  "env": {
    "key": "string"
  },
  // K8s liveness check. used for deployment/statefulSet
  "liveness": {
    // The path for health check
    "path": "string",
    // Service port for health detect
    "port": 0,
    // Initial delay seconds. default is 600s
    "initialDelaySeconds":600,
    // Period check interval. default is 3s
    "periodSeconds": 3,
    // Failure retry limit. default is 3
    "failureThreshold": 3
  },
  // K8s readiness check. used for deployment/statefulSet, like liveness
  "readiness": {
  },
  "service": {
    // TCP(default) or UDP
    "protocol": "TCP",
    // Service port for external access
    "port": 0,
    // K8s node port
    "nodePort": 0,
    // Pod service listening port
    "targetPort": 0,
    // The type of service, such as ClusterIP, NodePort
    "serviceType": "string",
    // Extended environment variable
    "extends": {}
  },
  // Dependencies defines a list of other Workloads that must complete successfully
  // before this Workload can start execution. If any dependency fails, this Workload
  // will not be scheduled and is considered failed.
  "dependencies": [
    "string"
  ],
  // Cron Job configuration
  "cronJobs": [
    {
      // Scheduled execution time, such as "2025-09-30T16:04:00.000Z" or "0 3 * * *"
      // Note: Only minute-level input is supported; seconds are not supported.
      "schedule": "string",
      // The action to take when the schedule is triggered. e.g. start
      "action": "start"
    }
  ]
}
```


### Update Workload

#### Request
```http
PATCH /api/v1/workloads/{workloadId}
Content-Type: application/json
```


**Request Body (PatchWorkloadRequest):**
```json
{
  // Workload scheduling priority. Defaults is 0, valid range: 0–2
  "priority": 0,
  // Requested replica count for the workload
  "replica": 0,
  // Cpu cores, e.g. 128
  "cpu": "string",
  // Gpu card, e.g. 8
  "gpu": "string",
  // Memory size, e.g. 128Gi
  "memory": "string",
  // Pod storage size, e.g. 50Gi
  "ephemeralStorage": "string",
  // Shared memory, e.g. 20Gi
  "sharedMemory": "string",
  // The image address used by workload
  "image": "string",
  // Workload startup command, required in base64 encoding
  "entryPoint": "string",
  // Environment variable for workload
  "env": {
    "key": "string"
  },
  // Workload description
  "description": "string",
  // Workload timeout in seconds. Default is 0 (no timeout).
  "timeout": 0,
  // Failure retry limit
  "maxRetry": 0,
  // Cron Job configuration
  "cronJobs": [
    {
      // Scheduled execution time, such as "2025-09-30T16:04:00.000Z" or "0 3 * * *"
      // Note: Only minute-level input is supported; seconds are not supported.
      "schedule": "string",
      // The action to take when the schedule is triggered. e.g. start
      "action": "start"
    }
  ]
}
```


#### Response
Returns 200 OK status code with no response body on success.

### Delete Workload

#### Request
```http
DELETE /api/v1/workloads/{workloadId}
```


#### Response
Returns 200 OK status code with no response body on success.

### Batch Delete Workloads

#### Request
```http
POST /api/v1/workloads/delete
Content-Type: application/json
```


**Request Body (BatchWorkloadsRequest):**
```json
{
  // List of workload IDs to be processed
  "workloadIds": ["string"]
}
```


#### Response
Returns 200 OK status code with no response body on success.

### Stop Workload

#### Request
```http
POST /api/v1/workloads/{workloadId}/stop
```


#### Response
Returns 200 OK status code with no response body on success.

### Batch Stop Workloads

#### Request
```http
POST /api/v1/workloads/stop
Content-Type: application/json
```


**Request Body (BatchWorkloadsRequest):**
```json
{
  // List of workload IDs to be processed
  "workloadIds": ["string"]
}
```


#### Response
Returns 200 OK status code with no response body on success.

### Clone Workloads

#### Request
```http
POST /api/v1/workloads/clone
Content-Type: application/json
```


**Request Body (BatchWorkloadsRequest):**
```json
{
  // List of workload IDs to be processed
  "workloadIds": ["string"]
}
```


#### Response
Returns 200 OK status code with no response body on success.

### Get Workload Pod Log

#### Request
```http
GET /api/v1/workloads/{workloadId}/pods/{podId}/logs?tailLines=1000&container=string&sinceSeconds=0
```


**Query Parameters:**
- `tailLines`: Retrieve the last n lines of logs. Default is 1000
- `container`: Return logs for the corresponding container
- `sinceSeconds`: Start time for retrieving logs, in seconds

#### Response (GetWorkloadPodLogResponse)
```json
{
  // The workload id
  "workloadId": "string",
  // The pod id
  "podId": "string",
  // The namespace which the workload belongs to
  "namespace": "string",
  // An array of log lines, returned in the same order as they appear in the original logs
  "logs": ["string"]
}
```


### Get Workload Pod Containers

#### Request
```http
GET /api/v1/workloads/{workloadId}/pods/{podId}/containers
```


#### Response (GetWorkloadPodContainersResponse)
```json
{
  // List of containers in the workload pod.
  "containers": [
    {
      // Name of the container.
      "name": "string"
    }
  ],
  // Supported shells, should allow user customization. e.g. "bash", "sh", "zsh"
  "shells": ["string"]
}
```
