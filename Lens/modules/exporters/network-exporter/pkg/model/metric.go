package model

type TcpEgressMetricValue struct {
	Raddr     string  `json:"raddr"`
	Rport     int     `json:"rport"`
	Direction string  `json:"direction"`
	Type      string  `json:"type"`
	Value     float64 `json:"value"`
}

func (v TcpEgressMetricValue) String() string {
	return v.Raddr + "_" + string(rune(v.Rport)) + "_" + v.Direction + "_" + v.Type
}

type TcpIngressMetricValue struct {
	Lport     int     `json:"lport"`
	Raddr     string  `json:"raddr"`
	Direction string  `json:"direction"`
	Type      string  `json:"type"`
	Value     float64 `json:"value"`
}

func (v TcpIngressMetricValue) String() string {
	return string(rune(v.Lport)) + "_" + v.Raddr + "_" + v.Direction + "_" + v.Type
}
