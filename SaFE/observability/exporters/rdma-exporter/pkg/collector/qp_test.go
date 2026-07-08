// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"encoding/json"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
)

func TestQPJSONParsing(t *testing.T) {
	const sample = `[{"ifindex":0,"ifname":"bnxt_re0","port":1,"lqpn":123,"rqpn":456,"type":"RC","state":"RTS","sq-psn":0,"comm":"python3","pid":1234},{"ifindex":0,"ifname":"bnxt_re0","port":1,"lqpn":1,"type":"GSI","state":"RTS","sq-psn":0,"comm":"ib_core"}]`

	var qps []model.RDMAQP
	if err := json.Unmarshal([]byte(sample), &qps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(qps) != 2 {
		t.Fatalf("len: got %d, want 2", len(qps))
	}
	q0 := qps[0]
	if q0.LQPN != 123 {
		t.Errorf("LQPN: got %d, want 123", q0.LQPN)
	}
	if q0.RQPN != 456 {
		t.Errorf("RQPN: got %d, want 456", q0.RQPN)
	}
	if q0.Type != "RC" {
		t.Errorf("Type: got %q, want RC", q0.Type)
	}
	if q0.Comm != "python3" {
		t.Errorf("Comm: got %q, want python3", q0.Comm)
	}
	if q0.PID != 1234 {
		t.Errorf("PID: got %d, want 1234", q0.PID)
	}
}
