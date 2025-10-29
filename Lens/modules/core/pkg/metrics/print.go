package metrics

import (
	"bytes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// 返回promethues指标
func GetPromethuesAsFmtText() (string, error) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return "", err
	}

	bb := bytes.NewBuffer([]byte{})
	enc := expfmt.NewEncoder(bb, expfmt.FmtText)
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			return "", err
		}
	}

	return bb.String(), nil
}
