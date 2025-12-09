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
	Tag         string           `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	SimpleValue *float32         `protobuf:"fixed32,2,opt,name=simple_value,json=simpleValue,proto3" json:"simple_value,omitempty"`
	Tensor      *TensorProto     `protobuf:"bytes,8,opt,name=tensor,proto3" json:"tensor,omitempty"`
	Metadata    *SummaryMetadata `protobuf:"bytes,9,opt,name=metadata,proto3" json:"metadata,omitempty"`
}

// Reset resets the value
func (v *SummaryValue) Reset() { *v = SummaryValue{} }

// String returns string representation
func (v *SummaryValue) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*SummaryValue) ProtoMessage() {}

// DataType represents tensor data types
type DataType int32

const (
	DT_INVALID DataType = 0
	DT_FLOAT   DataType = 1
	DT_DOUBLE  DataType = 2
	DT_INT32   DataType = 3
	DT_UINT8   DataType = 4
	DT_INT16   DataType = 5
	DT_INT8    DataType = 6
	DT_STRING  DataType = 7
	DT_INT64   DataType = 9
	DT_BOOL    DataType = 10
)

// TensorProto represents a tensor in TensorBoard
type TensorProto struct {
	Dtype     DataType `protobuf:"varint,1,opt,name=dtype,proto3,enum=tensorflow.DataType" json:"dtype,omitempty"`
	StringVal [][]byte `protobuf:"bytes,8,rep,name=string_val,json=stringVal,proto3" json:"string_val,omitempty"`
}

// Reset resets the tensor
func (t *TensorProto) Reset() { *t = TensorProto{} }

// String returns string representation
func (t *TensorProto) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*TensorProto) ProtoMessage() {}

// SummaryMetadata represents metadata about a summary
type SummaryMetadata struct {
	PluginData         *PluginData `protobuf:"bytes,1,opt,name=plugin_data,json=pluginData,proto3" json:"plugin_data,omitempty"`
	DisplayName        string      `protobuf:"bytes,2,opt,name=display_name,json=displayName,proto3" json:"display_name,omitempty"`
	SummaryDescription string      `protobuf:"bytes,3,opt,name=summary_description,json=summaryDescription,proto3" json:"summary_description,omitempty"`
}

// Reset resets the metadata
func (m *SummaryMetadata) Reset() { *m = SummaryMetadata{} }

// String returns string representation
func (m *SummaryMetadata) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*SummaryMetadata) ProtoMessage() {}

// PluginData represents plugin-specific data
type PluginData struct {
	PluginName string `protobuf:"bytes,1,opt,name=plugin_name,json=pluginName,proto3" json:"plugin_name,omitempty"`
	Content    []byte `protobuf:"bytes,2,opt,name=content,proto3" json:"content,omitempty"`
}

// Reset resets the plugin data
func (p *PluginData) Reset() { *p = PluginData{} }

// String returns string representation
func (p *PluginData) String() string { return "" }

// ProtoMessage marks this as a proto message
func (*PluginData) ProtoMessage() {}
