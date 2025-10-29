package filter

type WorkloadFilter struct {
	Kind      *string `json:"kind,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
	Name      *string `json:"name,omitempty"`
	Uid       *string `json:"uid,omitempty"`
	ParentUid *string `json:"parent_uid,omitempty"`
	Status    *string `json:"status,omitempty"`
	Limit     int
	Offset    int
	OrderBy   string
	Order     string
}
