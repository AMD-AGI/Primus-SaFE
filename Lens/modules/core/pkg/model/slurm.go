package model

type SlurmJob struct {
	ID         uint32
	Partition  string
	Name       string
	User       string
	State      string
	Elapsed    string
	Nodes      uint32
	Reason     string
	SubmitTime string
	Account    string
	QOS        string
	GPU        string
	GPUCount   uint32
}

type SlurmNode struct {
	Name      string
	State     string
	NodeCount uint32
	CPU       string
	Memory    uint32
	Features  string
	GRES      string
	Partition string
	Arch      string
	Extend    string
	Load      string
	MemUsed   string
}
