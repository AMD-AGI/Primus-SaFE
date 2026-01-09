// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tensorboard

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
)

// TestEventWriter is a helper for creating TensorBoard event files for testing
type TestEventWriter struct {
	crcTable *crc32.Table
	buffer   bytes.Buffer
}

// NewTestEventWriter creates a new test event writer
func NewTestEventWriter() *TestEventWriter {
	return &TestEventWriter{
		crcTable: crc32.MakeTable(crc32.Castagnoli),
	}
}

// maskedCRC32 computes the masked CRC32 checksum (same as TensorBoard format)
func (w *TestEventWriter) maskedCRC32(data []byte) uint32 {
	crc := crc32.Checksum(data, w.crcTable)
	return ((crc >> 15) | (crc << 17)) + 0xa282ead8
}

// writeVarint writes a variable-length integer
func (w *TestEventWriter) writeVarint(buf *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		buf.WriteByte(byte(value) | 0x80)
		value >>= 7
	}
	buf.WriteByte(byte(value))
}

// buildEventData builds protobuf-encoded Event data
func (w *TestEventWriter) buildEventData(wallTime float64, step int64, fileVersion string, scalars map[string]float32) []byte {
	var buf bytes.Buffer

	// Field 1: wall_time (fixed64, wire type 1)
	buf.WriteByte((1 << 3) | 1) // field 1, wire type 1 (fixed64)
	binary.Write(&buf, binary.LittleEndian, wallTime)

	// Field 2: step (varint, wire type 0)
	buf.WriteByte((2 << 3) | 0) // field 2, wire type 0 (varint)
	w.writeVarint(&buf, uint64(step))

	// Field 3: file_version (string, wire type 2)
	if fileVersion != "" {
		buf.WriteByte((3 << 3) | 2) // field 3, wire type 2 (length-delimited)
		w.writeVarint(&buf, uint64(len(fileVersion)))
		buf.WriteString(fileVersion)
	}

	// Field 5: summary (message, wire type 2)
	if len(scalars) > 0 {
		summaryData := w.buildSummaryData(scalars)
		buf.WriteByte((5 << 3) | 2) // field 5, wire type 2 (length-delimited)
		w.writeVarint(&buf, uint64(len(summaryData)))
		buf.Write(summaryData)
	}

	return buf.Bytes()
}

// buildSummaryData builds protobuf-encoded Summary data
func (w *TestEventWriter) buildSummaryData(scalars map[string]float32) []byte {
	var buf bytes.Buffer

	for tag, value := range scalars {
		valueData := w.buildSummaryValueData(tag, value)
		buf.WriteByte((1 << 3) | 2) // field 1, wire type 2 (length-delimited)
		w.writeVarint(&buf, uint64(len(valueData)))
		buf.Write(valueData)
	}

	return buf.Bytes()
}

// buildSummaryValueData builds protobuf-encoded SummaryValue data
func (w *TestEventWriter) buildSummaryValueData(tag string, value float32) []byte {
	var buf bytes.Buffer

	// Field 1: tag (string, wire type 2)
	buf.WriteByte((1 << 3) | 2) // field 1, wire type 2 (length-delimited)
	w.writeVarint(&buf, uint64(len(tag)))
	buf.WriteString(tag)

	// Field 2: simple_value (float32, wire type 5 - fixed32)
	buf.WriteByte((2 << 3) | 5) // field 2, wire type 5 (fixed32)
	binary.Write(&buf, binary.LittleEndian, value)

	return buf.Bytes()
}

// WriteEvent writes a single TensorBoard event record
func (w *TestEventWriter) WriteEvent(wallTime float64, step int64, fileVersion string, scalars map[string]float32) {
	eventData := w.buildEventData(wallTime, step, fileVersion, scalars)

	// Write length (8 bytes, little-endian)
	lengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lengthBytes, uint64(len(eventData)))
	w.buffer.Write(lengthBytes)

	// Write length CRC (4 bytes)
	lengthCRC := w.maskedCRC32(lengthBytes)
	binary.Write(&w.buffer, binary.LittleEndian, lengthCRC)

	// Write event data
	w.buffer.Write(eventData)

	// Write event data CRC (4 bytes)
	dataCRC := w.maskedCRC32(eventData)
	binary.Write(&w.buffer, binary.LittleEndian, dataCRC)
}

// Bytes returns the complete event file data
func (w *TestEventWriter) Bytes() []byte {
	return w.buffer.Bytes()
}

// CreateTestEventFile creates a test TensorBoard event file with sample data
func CreateTestEventFile(dir string) (string, error) {
	writer := NewTestEventWriter()

	// Write file version event (first event in TensorBoard files)
	writer.WriteEvent(1700000000.0, 0, "brain.Event:2", nil)

	// Write some scalar events
	for i := 1; i <= 100; i++ {
		scalars := map[string]float32{
			"loss":          float32(2.0 - float64(i)*0.015),
			"learning-rate": 0.001,
			"accuracy":      float32(float64(i) * 0.008),
		}
		writer.WriteEvent(1700000000.0+float64(i)*10, int64(i), "", scalars)
	}

	// Write to temp file
	filePath := filepath.Join(dir, "events.out.tfevents.test.0")
	err := os.WriteFile(filePath, writer.Bytes(), 0644)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

