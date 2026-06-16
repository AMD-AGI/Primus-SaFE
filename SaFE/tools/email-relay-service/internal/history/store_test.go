package history

import "testing"

// TestNewStore verifies the default and explicit max size handling.
func TestNewStore(t *testing.T) {
	if s := NewStore(0); s.maxSize != 500 {
		t.Errorf("default maxSize = %d, want 500", s.maxSize)
	}
	if s := NewStore(10); s.maxSize != 10 {
		t.Errorf("maxSize = %d, want 10", s.maxSize)
	}
}

// TestRegisterCluster verifies clusters are registered once.
func TestRegisterCluster(t *testing.T) {
	s := NewStore(10)
	s.RegisterCluster("c1", "http://x")
	s.RegisterCluster("c1", "http://other")
	statuses := s.GetClusterStatuses()
	if len(statuses) != 1 {
		t.Fatalf("cluster count = %d, want 1", len(statuses))
	}
	if statuses[0].BaseURL != "http://x" {
		t.Errorf("base_url overwritten: %q", statuses[0].BaseURL)
	}
}

// TestSetConnected verifies connection state updates for known clusters only.
func TestSetConnected(t *testing.T) {
	s := NewStore(10)
	s.RegisterCluster("c1", "http://x")
	s.SetConnected("c1", true)
	s.SetConnected("unknown", true) // no-op, must not panic
	if !s.GetClusterStatuses()[0].Connected {
		t.Errorf("cluster c1 should be connected")
	}
}

// TestAddRecord verifies records are appended, counters updated and size capped.
func TestAddRecord(t *testing.T) {
	s := NewStore(2)
	s.RegisterCluster("c1", "http://x")
	s.AddRecord("c1", 1, "src", []string{"a@b.com"}, "sub", "sent", "")
	s.AddRecord("c1", 2, "src", []string{"a@b.com"}, "sub", "failed", "boom")
	s.AddRecord("c1", 3, "src", []string{"a@b.com"}, "sub", "sent", "")

	records := s.GetRecords("", 0)
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2 (capped)", len(records))
	}
	status := s.GetClusterStatuses()[0]
	if status.SentCount != 2 || status.FailCount != 1 {
		t.Errorf("counters sent=%d fail=%d, want 2/1", status.SentCount, status.FailCount)
	}
}

// TestGetRecords verifies filtering, ordering and limiting.
func TestGetRecords(t *testing.T) {
	s := NewStore(10)
	s.AddRecord("c1", 1, "src", nil, "s1", "sent", "")
	s.AddRecord("c2", 2, "src", nil, "s2", "sent", "")
	s.AddRecord("c1", 3, "src", nil, "s3", "sent", "")

	// newest first
	all := s.GetRecords("", 0)
	if len(all) != 3 || all[0].Subject != "s3" {
		t.Fatalf("unexpected order/len: %+v", all)
	}

	// filter by cluster
	c1 := s.GetRecords("c1", 0)
	if len(c1) != 2 {
		t.Errorf("c1 records = %d, want 2", len(c1))
	}

	// limit applies
	limited := s.GetRecords("", 1)
	if len(limited) != 1 {
		t.Errorf("limited records = %d, want 1", len(limited))
	}
}

// TestGetClusterStatuses verifies a snapshot of all clusters is returned.
func TestGetClusterStatuses(t *testing.T) {
	s := NewStore(10)
	s.RegisterCluster("c1", "http://x")
	s.RegisterCluster("c2", "http://y")
	if len(s.GetClusterStatuses()) != 2 {
		t.Errorf("expected 2 cluster statuses")
	}
}
