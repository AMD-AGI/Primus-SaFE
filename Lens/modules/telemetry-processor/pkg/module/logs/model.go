package logs

import (
	"time"
)

type PodLog struct {
	Date       float64    `json:"date"`
	Time       time.Time  `json:"time"`
	Stream     string     `json:"stream"`
	Logtag     string     `json:"logtag"`
	Message    string     `json:"message"`
	Log        string     `json:"log"`
	Kubernetes *K8SFields `json:"kubernetes"`
	Type       string     `json:"type"`
	*JournalFields
}

type K8SFields struct {
	PodName        string          `json:"pod_name"`
	NamespaceName  string          `json:"namespace_name"`
	PodId          string          `json:"pod_id"`
	Labels         *K8SFieldLabels `json:"labels"`
	Host           string          `json:"host"`
	ContainerName  string          `json:"container_name"`
	DockerId       string          `json:"docker_id"`
	ContainerHash  string          `json:"container_hash"`
	ContainerImage string          `json:"container_image"`
}

type K8SFieldLabels struct {
	App                             string `json:"app"`
	Component                       string `json:"component"`
	ControllerRevisionHash          string `json:"controller-revision-hash"`
	PodTemplateGeneration           string `json:"pod-template-generation"`
	JuicefsUniqueid                 string `json:"juicefs-uniqueid"`
	TrainingKubeflowOrgJobName      string `json:"training.kubeflow.org/job-name"`
	TrainingKubeflowOrgOperatorName string `json:"training.kubeflow.org/operator-name"`
	TrainingKubeflowOrgReplicaIndex string `json:"training.kubeflow.org/replica-index"`
	TrainingKubeflowOrgReplicaType  string `json:"training.kubeflow.org/replica-type"`
}

type JournalFields struct {
	TRANSPORT               string `json:"_TRANSPORT"`
	UID                     string `json:"_UID"`
	GID                     string `json:"_GID"`
	SELINUXCONTEXT          string `json:"_SELINUX_CONTEXT"`
	BOOTID                  string `json:"_BOOT_ID"`
	MACHINEID               string `json:"_MACHINE_ID"`
	Hostname                string `json:"hostname"`
	PRIORITY                string `json:"PRIORITY"`
	SYSTEMDSLICE            string `json:"_SYSTEMD_SLICE"`
	SYSLOGFACILITY          string `json:"SYSLOG_FACILITY"`
	TID                     string `json:"TID"`
	CODEFILE                string `json:"CODE_FILE"`
	SYSLOGIDENTIFIER        string `json:"SYSLOG_IDENTIFIER"`
	USERID                  string `json:"USER_ID"`
	PID                     string `json:"_PID"`
	COMM                    string `json:"_COMM"`
	EXE                     string `json:"_EXE"`
	CMDLINE                 string `json:"_CMDLINE"`
	CAPEFFECTIVE            string `json:"_CAP_EFFECTIVE"`
	SYSTEMDCGROUP           string `json:"_SYSTEMD_CGROUP"`
	SYSTEMDUNIT             string `json:"_SYSTEMD_UNIT"`
	SYSTEMDINVOCATIONID     string `json:"_SYSTEMD_INVOCATION_ID"`
	CODELINE                string `json:"CODE_LINE"`
	CODEFUNC                string `json:"CODE_FUNC"`
	MESSAGEID               string `json:"MESSAGE_ID"`
	SESSIONID               string `json:"SESSION_ID"`
	LEADER                  string `json:"LEADER"`
	MESSAGE                 string `json:"MESSAGE"`
	SOURCEREALTIMETIMESTAMP string `json:"_SOURCE_REALTIME_TIMESTAMP"`
}
