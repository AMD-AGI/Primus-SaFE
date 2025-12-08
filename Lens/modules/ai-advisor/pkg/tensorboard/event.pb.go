// Simple protobuf implementation for TensorBoard Event format
package tensorboard

// Event represents a TensorBoard event
type Event struct {
	WallTime    float64  `protobuf:"fixed64,1,opt,name=wall_time,json=wallTime,proto3" json:"wall_time,omitempty"`
	Step        int64    `protobuf:"varint,2,opt,name=step,proto3" json:"step,omitempty"`
	Summary     *Summary `protobuf:"bytes,5,opt,name=summary,proto3" json:"summary,omitempty"`
	FileVersion string   `protobuf:"bytes,3,opt,name=file_version,json=fileVersion,proto3" json:"file_version,omitempty"`
}

// Reset resets the event
func (e *Event) Reset() { *e = Event{} }

// String returns string representation
func (e *Event) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*Event) ProtoMessage() {}

// Summary represents a summary in an event
type Summary struct {
	Value []*SummaryValue `protobuf:"bytes,1,rep,name=value,proto3" json:"value,omitempty"`
}

// Reset resets the summary
func (s *Summary) Reset() { *s = Summary{} }

// String returns string representation
func (s *Summary) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*Summary) ProtoMessage() {}

// SummaryValue represents a value in a summary
type SummaryValue struct {
	Tag         string   `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	SimpleValue *float32 `protobuf:"fixed32,2,opt,name=simple_value,json=simpleValue,proto3" json:"simple_value,omitempty"`
}

// Reset resets the value
func (v *SummaryValue) Reset() { *v = SummaryValue{} }

// String returns string representation
func (v *SummaryValue) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*SummaryValue) ProtoMessage() {}
