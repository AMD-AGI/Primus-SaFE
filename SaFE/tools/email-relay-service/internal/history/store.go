package history

import (
	"sync"
	"time"
)

type Record struct {
	ID         int       `json:"id"`
	Cluster    string    `json:"cluster"`
	OutboxID   int32     `json:"outbox_id"`
	Source     string    `json:"source"`
	Recipients []string  `json:"recipients"`
	Subject    string    `json:"subject"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	SentAt     time.Time `json:"sent_at"`
}

type ClusterStatus struct {
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	Connected bool      `json:"connected"`
	LastEvent time.Time `json:"last_event,omitempty"`
	SentCount int       `json:"sent_count"`
	FailCount int       `json:"fail_count"`
}

type Store struct {
	mu       sync.RWMutex
	records  []Record
	nextID   int
	maxSize  int
	clusters map[string]*ClusterStatus
}

func NewStore(maxSize int) *Store {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &Store{
		maxSize:  maxSize,
		clusters: make(map[string]*ClusterStatus),
	}
}

func (s *Store) RegisterCluster(name, baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[name]; !ok {
		s.clusters[name] = &ClusterStatus{Name: name, BaseURL: baseURL}
	}
}

func (s *Store) SetConnected(cluster string, connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cs, ok := s.clusters[cluster]; ok {
		cs.Connected = connected
	}
}

func (s *Store) AddRecord(cluster string, outboxID int32, source string, recipients []string, subject, status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	rec := Record{
		ID:         s.nextID,
		Cluster:    cluster,
		OutboxID:   outboxID,
		Source:     source,
		Recipients: recipients,
		Subject:    subject,
		Status:     status,
		Error:      errMsg,
		SentAt:     time.Now(),
	}
	s.records = append(s.records, rec)
	if len(s.records) > s.maxSize {
		s.records = s.records[len(s.records)-s.maxSize:]
	}

	if cs, ok := s.clusters[cluster]; ok {
		cs.LastEvent = rec.SentAt
		if status == "sent" {
			cs.SentCount++
		} else {
			cs.FailCount++
		}
	}
}

func (s *Store) GetRecords(cluster string, limit int) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []Record
	for i := len(s.records) - 1; i >= 0; i-- {
		r := s.records[i]
		if cluster != "" && r.Cluster != cluster {
			continue
		}
		filtered = append(filtered, r)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func (s *Store) GetClusterStatuses() []ClusterStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ClusterStatus, 0, len(s.clusters))
	for _, cs := range s.clusters {
		result = append(result, *cs)
	}
	return result
}
