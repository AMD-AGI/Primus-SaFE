package filter

type NodeFilter struct {
	Name          *string
	Address       *string
	GPUName       *string
	GPUAllocation *int
	GPUCount      *int
	GPUUtilMin    *float64
	GPUUtilMax    *float64
	Status        []string
	CPU           *string
	CPUCount      *int
	Memory        *string
	K8sVersion    *string
	K8sStatus     *string
	Limit         int
	Offset        int
	OrderBy       string
	Order         string
}
