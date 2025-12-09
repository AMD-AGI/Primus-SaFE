package tensorboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// EventParser parses TensorBoard event files
type EventParser struct {
	crcTable *crc32.Table
}

// NewEventParser creates a new event parser
func NewEventParser() *EventParser {
	return &EventParser{
		crcTable: crc32.MakeTable(crc32.Castagnoli),
	}
}

// ParsedEvent represents a parsed TensorBoard event
type ParsedEvent struct {
	WallTime float64            `json:"wall_time"`
	Step     int64              `json:"step"`
	Scalars  map[string]float32 `json:"scalars,omitempty"`
	Texts    map[string]string  `json:"texts,omitempty"` // Text metadata
	Tags     []string           `json:"tags,omitempty"`
	RawEvent *Event             `json:"-"` // Raw protobuf event
}

// ParseEvents parses TensorBoard events from binary content
func (p *EventParser) ParseEvents(content []byte) ([]*ParsedEvent, error) {
	if len(content) == 0 {
		return nil, nil
	}

	reader := bytes.NewReader(content)
	var events []*ParsedEvent

	for {
		event, err := p.readEvent(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Warnf("Failed to parse event: %v", err)
			// Continue to next event instead of failing completely
			continue
		}

		if event != nil {
			events = append(events, event)
		}
	}

	return events, nil
}

// ParseEventsWithBuffer 从缓冲区解析完整的 events，返回解析的 events 和已消费的字节数
// 如果缓冲区包含不完整的 record，会停止解析并返回已解析的部分
// 返回值: (events, consumedBytes, error)
func (p *EventParser) ParseEventsWithBuffer(buffer []byte) ([]*ParsedEvent, int, error) {
	if len(buffer) == 0 {
		return nil, 0, nil
	}

	var events []*ParsedEvent
	totalConsumed := 0
	offset := 0

	for offset < len(buffer) {
		// 尝试读取一个完整的 event
		event, consumed, err := p.tryReadEventAt(buffer[offset:])

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// 数据不完整，停止解析
			log.Debugf("Incomplete event at offset %d, need more data", offset)
			break
		}

		if err != nil {
			// 解析出错，记录详细信息并跳过
			log.Warnf("Event parse error at offset %d: %v (will skip %d bytes)", offset, err, consumed)
			if consumed > 0 {
				offset += consumed
				totalConsumed += consumed
			} else {
				// 无法确定跳过多少，停止解析
				log.Warnf("Cannot determine bytes to skip at offset %d, stopping parse", offset)
				break
			}
			continue
		}

		if event != nil {
			events = append(events, event)
		}

		offset += consumed
		totalConsumed += consumed
	}

	log.Debugf("Parsed %d events, consumed %d/%d bytes", len(events), totalConsumed, len(buffer))
	return events, totalConsumed, nil
}

// tryReadEventAt 尝试从指定位置读取一个完整的 event
// 返回: (event, consumedBytes, error)
// 如果数据不完整，返回 io.ErrUnexpectedEOF
func (p *EventParser) tryReadEventAt(data []byte) (*ParsedEvent, int, error) {
	if len(data) < 12 {
		// 至少需要 length(8) + length_crc(4) = 12 字节
		return nil, 0, io.ErrUnexpectedEOF
	}

	offset := 0

	// 1. Read length (8 bytes, little-endian)
	length := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// 2. Read length CRC (4 bytes)
	lengthCRC := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// 3. Verify length CRC
	lengthBytes := data[0:8]
	computedCRC := p.maskedCRC32(lengthBytes)
	if computedCRC != lengthCRC {
		return nil, offset, fmt.Errorf("length CRC mismatch: expected %d, got %d", lengthCRC, computedCRC)
	}

	// 4. Check if we have enough data for the event data + data CRC
	totalNeeded := offset + int(length) + 4 // current offset + data length + data CRC (4 bytes)
	if len(data) < totalNeeded {
		// 数据不完整
		return nil, 0, io.ErrUnexpectedEOF
	}

	// 5. Read event data
	eventData := data[offset : offset+int(length)]
	offset += int(length)

	// 6. Read event data CRC (4 bytes)
	dataCRC := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// 7. Verify event data CRC
	computedDataCRC := p.maskedCRC32(eventData)
	if computedDataCRC != dataCRC {
		return nil, offset, fmt.Errorf("data CRC mismatch: expected %d, got %d", dataCRC, computedDataCRC)
	}

	// 8. Parse protobuf event
	event, err := p.parseProtobufEvent(eventData)
	if err != nil {
		return nil, offset, fmt.Errorf("failed to parse event: %w", err)
	}

	// 9. Convert to ParsedEvent
	parsedEvent := p.convertEvent(event)

	return parsedEvent, offset, nil
}

// readEvent reads a single event from the reader
// TensorBoard event format:
// - uint64: length (little-endian)
// - uint32: masked CRC32 of length
// - bytes:  event data
// - uint32: masked CRC32 of event data
func (p *EventParser) readEvent(reader io.Reader) (*ParsedEvent, error) {
	// Read length (8 bytes, little-endian)
	var length uint64
	if err := binary.Read(reader, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	// Read length CRC (4 bytes)
	var lengthCRC uint32
	if err := binary.Read(reader, binary.LittleEndian, &lengthCRC); err != nil {
		return nil, err
	}

	// Verify length CRC
	lengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lengthBytes, length)
	computedCRC := p.maskedCRC32(lengthBytes)
	if computedCRC != lengthCRC {
		return nil, fmt.Errorf("length CRC mismatch: expected %d, got %d", lengthCRC, computedCRC)
	}

	// Read event data
	eventData := make([]byte, length)
	if _, err := io.ReadFull(reader, eventData); err != nil {
		return nil, err
	}

	// Read event data CRC (4 bytes)
	var dataCRC uint32
	if err := binary.Read(reader, binary.LittleEndian, &dataCRC); err != nil {
		return nil, err
	}

	// Verify event data CRC
	computedDataCRC := p.maskedCRC32(eventData)
	if computedDataCRC != dataCRC {
		return nil, fmt.Errorf("data CRC mismatch: expected %d, got %d", dataCRC, computedDataCRC)
	}

	// Parse protobuf event manually
	event, err := p.parseProtobufEvent(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse event: %w", err)
	}

	// Convert to ParsedEvent
	return p.convertEvent(event), nil
}

// convertEvent converts a protobuf Event to ParsedEvent
func (p *EventParser) convertEvent(event *Event) *ParsedEvent {
	parsed := &ParsedEvent{
		WallTime: event.WallTime,
		Step:     event.Step,
		Scalars:  make(map[string]float32),
		Texts:    make(map[string]string),
		RawEvent: event,
	}

	// Extract scalar and text values from Summary
	if event.Summary != nil {
		for _, value := range event.Summary.Value {
			tag := value.Tag
			parsed.Tags = append(parsed.Tags, tag)

			// Extract simple scalar value
			if value.SimpleValue != nil {
				parsed.Scalars[tag] = *value.SimpleValue
			}

			// Extract text value from tensor
			if value.Tensor != nil && value.Tensor.Dtype == DT_STRING {
				// Text data is stored in StringVal field
				if len(value.Tensor.StringVal) > 0 {
					parsed.Texts[tag] = string(value.Tensor.StringVal[0])
				}
			}
		}
	}

	return parsed
}

// maskedCRC32 computes the masked CRC32 checksum
// TensorBoard uses a masked CRC32 to avoid issues with certain bit patterns
func (p *EventParser) maskedCRC32(data []byte) uint32 {
	crc := crc32.Checksum(data, p.crcTable)
	// TensorBoard masking: rotate right by 15 bits and add a constant
	return ((crc >> 15) | (crc << 17)) + 0xa282ead8
}

// IsScalarEvent checks if an event contains scalar values
func (e *ParsedEvent) IsScalarEvent() bool {
	return len(e.Scalars) > 0
}

// GetScalar gets a specific scalar value by tag
func (e *ParsedEvent) GetScalar(tag string) (float32, bool) {
	val, ok := e.Scalars[tag]
	return val, ok
}

// IsTextEvent checks if an event contains text values
func (e *ParsedEvent) IsTextEvent() bool {
	return len(e.Texts) > 0
}

// GetText gets a specific text value by tag
func (e *ParsedEvent) GetText(tag string) (string, bool) {
	val, ok := e.Texts[tag]
	return val, ok
}

// parseProtobufEvent parses a protobuf-encoded Event message
// This is a simplified parser that handles the specific structure of TensorFlow Event
func (p *EventParser) parseProtobufEvent(data []byte) (*Event, error) {
	event := &Event{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		// Read field tag
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // wall_time (fixed64 - stored as double)
			if wireType != 1 { // fixed64
				return nil, fmt.Errorf("invalid wire type for wall_time: %d", wireType)
			}
			var val float64
			if err := binary.Read(buf, binary.LittleEndian, &val); err != nil {
				return nil, err
			}
			event.WallTime = val

		case 2: // step (varint)
			if wireType != 0 { // varint
				return nil, fmt.Errorf("invalid wire type for step: %d", wireType)
			}
			step, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			event.Step = int64(step)

		case 3: // file_version (string)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for file_version: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			event.FileVersion = string(strData)

		case 5: // summary (message)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for summary: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			summaryData := make([]byte, length)
			if _, err := io.ReadFull(buf, summaryData); err != nil {
				return nil, err
			}
			summary, err := p.parseSummary(summaryData)
			if err != nil {
				log.Warnf("Failed to parse summary: %v", err)
			} else {
				event.Summary = summary
			}

		default:
			// Skip unknown field
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return event, nil
}

// parseSummary parses a Summary message
func (p *EventParser) parseSummary(data []byte) (*Summary, error) {
	summary := &Summary{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // value (repeated message)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for summary value: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			valueData := make([]byte, length)
			if _, err := io.ReadFull(buf, valueData); err != nil {
				return nil, err
			}
			value, err := p.parseSummaryValue(valueData)
			if err != nil {
				log.Warnf("Failed to parse summary value: %v", err)
			} else {
				summary.Value = append(summary.Value, value)
			}

		default:
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return summary, nil
}

// parseSummaryValue parses a SummaryValue message
func (p *EventParser) parseSummaryValue(data []byte) (*SummaryValue, error) {
	value := &SummaryValue{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // tag (string)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for tag: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			value.Tag = string(strData)

		case 2: // simple_value (float - fixed32)
			if wireType != 5 { // fixed32
				return nil, fmt.Errorf("invalid wire type for simple_value: %d", wireType)
			}
			var floatVal float32
			if err := binary.Read(buf, binary.LittleEndian, &floatVal); err != nil {
				return nil, err
			}
			value.SimpleValue = &floatVal

		case 8: // tensor (message - TensorProto)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for tensor: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			tensorData := make([]byte, length)
			if _, err := io.ReadFull(buf, tensorData); err != nil {
				return nil, err
			}
			tensor, err := p.parseTensorProto(tensorData)
			if err != nil {
				log.Warnf("Failed to parse tensor: %v", err)
			} else {
				value.Tensor = tensor
			}

		case 9: // metadata (message - SummaryMetadata)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for metadata: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			metadataData := make([]byte, length)
			if _, err := io.ReadFull(buf, metadataData); err != nil {
				return nil, err
			}
			metadata, err := p.parseSummaryMetadata(metadataData)
			if err != nil {
				log.Warnf("Failed to parse metadata: %v", err)
			} else {
				value.Metadata = metadata
			}

		default:
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return value, nil
}

// readVarint reads a variable-length integer
func (p *EventParser) readVarint(buf io.Reader) (uint64, error) {
	var result uint64
	var shift uint
	for {
		b := make([]byte, 1)
		if _, err := buf.Read(b); err != nil {
			return 0, err
		}
		result |= uint64(b[0]&0x7f) << shift
		if b[0]&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result, nil
}

// skipField skips a field based on wire type
func (p *EventParser) skipField(buf io.Reader, wireType uint64) error {
	switch wireType {
	case 0: // varint
		_, err := p.readVarint(buf)
		return err
	case 1: // fixed64
		var val uint64
		return binary.Read(buf, binary.LittleEndian, &val)
	case 2: // length-delimited
		length, err := p.readVarint(buf)
		if err != nil {
			return err
		}
		skipData := make([]byte, length)
		_, err = io.ReadFull(buf, skipData)
		return err
	case 5: // fixed32
		var val uint32
		return binary.Read(buf, binary.LittleEndian, &val)
	default:
		return fmt.Errorf("unsupported wire type: %d", wireType)
	}
}

// parseTensorProto parses a TensorProto message
func (p *EventParser) parseTensorProto(data []byte) (*TensorProto, error) {
	tensor := &TensorProto{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // dtype (varint - enum)
			if wireType != 0 {
				return nil, fmt.Errorf("invalid wire type for dtype: %d", wireType)
			}
			dtype, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			tensor.Dtype = DataType(dtype)

		case 8: // string_val (repeated bytes)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for string_val: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			tensor.StringVal = append(tensor.StringVal, strData)

		default:
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return tensor, nil
}

// parseSummaryMetadata parses a SummaryMetadata message
func (p *EventParser) parseSummaryMetadata(data []byte) (*SummaryMetadata, error) {
	metadata := &SummaryMetadata{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // plugin_data (message - PluginData)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for plugin_data: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			pluginData := make([]byte, length)
			if _, err := io.ReadFull(buf, pluginData); err != nil {
				return nil, err
			}
			plugin, err := p.parsePluginData(pluginData)
			if err != nil {
				log.Warnf("Failed to parse plugin data: %v", err)
			} else {
				metadata.PluginData = plugin
			}

		case 2: // display_name (string)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for display_name: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			metadata.DisplayName = string(strData)

		case 3: // summary_description (string)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for summary_description: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			metadata.SummaryDescription = string(strData)

		default:
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return metadata, nil
}

// parsePluginData parses a PluginData message
func (p *EventParser) parsePluginData(data []byte) (*PluginData, error) {
	plugin := &PluginData{}
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		tag, err := p.readVarint(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // plugin_name (string)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for plugin_name: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			strData := make([]byte, length)
			if _, err := io.ReadFull(buf, strData); err != nil {
				return nil, err
			}
			plugin.PluginName = string(strData)

		case 2: // content (bytes)
			if wireType != 2 { // length-delimited
				return nil, fmt.Errorf("invalid wire type for content: %d", wireType)
			}
			length, err := p.readVarint(buf)
			if err != nil {
				return nil, err
			}
			content := make([]byte, length)
			if _, err := io.ReadFull(buf, content); err != nil {
				return nil, err
			}
			plugin.Content = content

		default:
			if err := p.skipField(buf, wireType); err != nil {
				return nil, err
			}
		}
	}

	return plugin, nil
}
