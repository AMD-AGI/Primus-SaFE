// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tensorboard

import (
	"fmt"
	"os"
	"testing"
)

// getTestEventFile creates or returns the path to a test event file
func getTestEventFile(t *testing.T) string {
	// Create a temp directory for test files
	tempDir := t.TempDir()

	// Create test event file
	filePath, err := CreateTestEventFile(tempDir)
	if err != nil {
		t.Fatalf("Failed to create test event file: %v", err)
	}

	return filePath
}

func TestParseRealEventFile(t *testing.T) {
	// Test parsing a TensorBoard event file
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()

	events, consumed, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("Parsed %d events, consumed %d/%d bytes\n", len(events), consumed, len(data))

	// Print first metadata event
	fmt.Printf("\n=== First Event (Metadata) ===\n")
	if len(events) > 0 {
		event := events[0]
		fmt.Printf("Step: %d, WallTime: %.2f\n", event.Step, event.WallTime)
		if event.RawEvent != nil {
			fmt.Printf("FileVersion: %s\n", event.RawEvent.FileVersion)
		}
	}

	// Count events with scalars and collect sample scalar events
	scalarEventCount := 0
	var scalarEvents []*ParsedEvent
	for _, event := range events {
		if len(event.Scalars) > 0 {
			scalarEventCount++
			if len(scalarEvents) < 10 {
				scalarEvents = append(scalarEvents, event)
			}
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total events: %d\n", len(events))
	fmt.Printf("Events with scalars: %d\n", scalarEventCount)

	if len(scalarEvents) > 0 {
		fmt.Printf("\n=== Sample Scalar Events ===\n")
		for i, event := range scalarEvents {
			fmt.Printf("\nScalar Event %d:\n", i+1)
			fmt.Printf("  Step: %d\n", event.Step)
			fmt.Printf("  WallTime: %.2f\n", event.WallTime)
			fmt.Printf("  Scalars: %v\n", event.Scalars)
		}
	}

	// Collect unique scalar tags
	tagSet := make(map[string]bool)
	for _, event := range events {
		for tag := range event.Scalars {
			tagSet[tag] = true
		}
	}

	fmt.Printf("\n=== Unique Scalar Tags (%d) ===\n", len(tagSet))
	tagCount := 0
	for tag := range tagSet {
		fmt.Printf("  - %s\n", tag)
		tagCount++
		if tagCount >= 20 {
			fmt.Printf("  ... and %d more\n", len(tagSet)-20)
			break
		}
	}

	if len(events) == 0 {
		t.Error("Expected to parse at least some events")
	}
}

func TestParseEventFileInChunks(t *testing.T) {
	// Read actual event file
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing chunked parsing ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	// Create parser
	parser := NewEventParser()

	// Simulate streaming read: read 1KB each time
	chunkSize := 1024
	buffer := make([]byte, 0)
	totalEvents := 0
	offset := 0

	for offset < len(data) {
		// Read a chunk of data
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		offset = end

		// Append to buffer
		buffer = append(buffer, chunk...)
		fmt.Printf("\nChunk: added %d bytes, buffer now %d bytes\n", len(chunk), len(buffer))

		// Parse buffer
		events, consumed, err := parser.ParseEventsWithBuffer(buffer)
		if err != nil {
			t.Errorf("Parse error at offset %d: %v", offset, err)
		}

		fmt.Printf("Parsed %d events, consumed %d bytes, remaining %d bytes\n",
			len(events), consumed, len(buffer)-consumed)

		// Update buffer
		if consumed > 0 {
			buffer = buffer[consumed:]
		}

		totalEvents += len(events)

		// Print first event
		if len(events) > 0 {
			fmt.Printf("First event: step=%d, scalars=%v\n",
				events[0].Step, events[0].Scalars)
		}
	}

	fmt.Printf("\n=== Total: %d events parsed ===\n", totalEvents)

	if totalEvents == 0 {
		t.Error("Expected to parse at least some events")
	}
}

func TestCRC32Implementation(t *testing.T) {
	parser := NewEventParser()

	// Test case from TensorFlow documentation
	// Length value: 18 (0x12 in varint)
	lengthBytes := []byte{0x12, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	crc := parser.maskedCRC32(lengthBytes)

	fmt.Printf("Test CRC32 for length=18: 0x%08x\n", crc)

	// Read first record of actual file
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	if len(data) < 12 {
		t.Fatal("File too small")
	}

	// Read first 12 bytes
	fmt.Printf("\nFirst 20 bytes (hex): ")
	for i := 0; i < 20 && i < len(data); i++ {
		fmt.Printf("%02x ", data[i])
	}
	fmt.Printf("\n")

	// Parse first record's header
	lengthRead := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
		uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56

	lengthCRC := uint32(data[8]) | uint32(data[9])<<8 | uint32(data[10])<<16 | uint32(data[11])<<24

	fmt.Printf("Length: %d (0x%x)\n", lengthRead, lengthRead)
	fmt.Printf("Length CRC from file: 0x%08x\n", lengthCRC)

	// Calculate CRC
	computedCRC := parser.maskedCRC32(data[0:8])
	fmt.Printf("Computed CRC: 0x%08x\n", computedCRC)

	if computedCRC != lengthCRC {
		t.Errorf("CRC MISMATCH! Expected 0x%08x, got 0x%08x\n", lengthCRC, computedCRC)
	} else {
		fmt.Printf("CRC MATCH!\n")
	}
}

func TestParseFromMiddleOffset(t *testing.T) {
	// Test parsing starting from middle of file
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing parsing from middle offset ===\n")

	// First parse completely to find a suitable offset
	parser := NewEventParser()

	// Use a smaller portion for the first parse
	firstPortion := len(data) / 2
	if firstPortion > len(data) {
		firstPortion = len(data)
	}

	_, consumedFirst, _ := parser.ParseEventsWithBuffer(data[:firstPortion])

	fmt.Printf("First %d bytes parsed, consumed %d bytes\n", firstPortion, consumedFirst)

	// Now parse remaining data starting from consumedFirst position
	fmt.Printf("\nNow parsing from offset %d (simulating resume)\n", consumedFirst)

	remainingData := data[consumedFirst:]
	events, consumed, err := parser.ParseEventsWithBuffer(remainingData)

	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("Parsed %d events from offset %d, consumed %d/%d bytes\n",
		len(events), consumedFirst, consumed, len(remainingData))

	if len(events) > 0 {
		fmt.Printf("First event after resume: step=%d, scalars=%v\n",
			events[0].Step, events[0].Scalars)
	}
}

func TestAnalyzeStepValues(t *testing.T) {
	// This test demonstrates TensorBoard event parsing with step values

	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	parser := NewEventParser()
	events, _, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("\n=== TensorBoard Step Semantics ===\n")
	fmt.Printf("Total events parsed: %d\n", len(events))

	// Group events by tag and show their step progression
	tagSteps := make(map[string][]int64)
	for _, event := range events {
		if len(event.Scalars) > 0 {
			for tag := range event.Scalars {
				if len(tagSteps[tag]) < 5 {
					tagSteps[tag] = append(tagSteps[tag], event.Step)
				}
			}
		}
	}

	fmt.Printf("\nStep progression (first 5 occurrences):\n")
	for tag, steps := range tagSteps {
		fmt.Printf("  %-40s: %v\n", tag, steps)
	}

	if len(events) == 0 {
		t.Error("Expected to parse at least some events")
	}
}

func TestVerifyFileIntegrity(t *testing.T) {
	// Test if the file itself has any corruption
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== File Integrity Check ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()
	events, consumed, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("Complete parse: %d events, consumed %d/%d bytes\n", len(events), consumed, len(data))

	if consumed != len(data) {
		t.Errorf("File not completely consumed: %d/%d bytes (%.2f%%)",
			consumed, len(data), float64(consumed)*100/float64(len(data)))

		// Analyze unconsumed portion
		remaining := len(data) - consumed
		fmt.Printf("\nUnconsumed data: %d bytes\n", remaining)
		if remaining > 0 && remaining < 200 {
			fmt.Printf("Last bytes (hex): ")
			for i := consumed; i < len(data); i++ {
				fmt.Printf("%02x ", data[i])
			}
			fmt.Printf("\n")
		}
	} else {
		fmt.Printf("✓ File completely parsed, no corruption detected\n")
	}
}

func TestAnalyzeOffsetError(t *testing.T) {
	// Analyze parsing behavior at specific offsets
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Analyzing offset behavior ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	// Parse the file completely
	parser := NewEventParser()
	events, consumed, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("\nComplete parse: %d events, consumed %d/%d bytes\n", len(events), consumed, len(data))

	// Simulate parsing at a mid-point offset
	midPoint := len(data) / 2
	fmt.Printf("\n=== Simulating stream read ending at offset %d ===\n", midPoint)

	// Parse up to the mid-point
	chunk1 := data[:midPoint]
	events1, consumed1, err := parser.ParseEventsWithBuffer(chunk1)
	if err != nil {
		t.Errorf("Parse error for chunk1: %v", err)
	}

	fmt.Printf("Chunk1 (0-%d): parsed %d events, consumed %d bytes\n",
		midPoint, len(events1), consumed1)
	fmt.Printf("Remaining in buffer: %d bytes\n", len(chunk1)-consumed1)
}

func TestStreamReaderWithIncompleteEvents(t *testing.T) {
	// Test that demonstrates the proper handling of incomplete events at chunk boundaries
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing Stream Reader with Incomplete Event Handling ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()

	// Simulate stream reader state
	readOffset := int64(0)
	buffer := make([]byte, 0)
	chunkSize := 500 // Small chunk size to test boundary handling

	totalEvents := 0
	iteration := 0

	for readOffset < int64(len(data)) || len(buffer) > 0 {
		iteration++

		// Read more data if available
		if readOffset < int64(len(data)) {
			remaining := int64(len(data)) - readOffset
			toRead := int64(chunkSize)
			if toRead > remaining {
				toRead = remaining
			}

			// Read from file
			newData := data[readOffset : readOffset+toRead]
			buffer = append(buffer, newData...)
			readOffset += toRead

			fmt.Printf("\nIteration %d:\n", iteration)
			fmt.Printf("  Read offset: %d\n", readOffset)
			fmt.Printf("  Read %d bytes from file\n", len(newData))
			fmt.Printf("  Buffer: %d bytes\n", len(buffer))
		}

		// Parse events from buffer
		events, consumed, parseErr := parser.ParseEventsWithBuffer(buffer)
		if parseErr != nil {
			t.Errorf("Parse error at iteration %d: %v", iteration, parseErr)
		}

		fmt.Printf("  Parsed: %d events, consumed %d bytes\n", len(events), consumed)

		// Update buffer
		if consumed > 0 {
			buffer = buffer[consumed:]
			fmt.Printf("  Updated: buffer remaining=%d bytes\n", len(buffer))
		}

		totalEvents += len(events)

		// Safety check
		if iteration > 100 {
			t.Fatalf("Too many iterations, possible infinite loop")
		}

		// Check for progress - if we've read all data and can't consume anything, we're done
		if consumed == 0 && readOffset >= int64(len(data)) {
			fmt.Printf("  No more progress possible, ending\n")
			break
		}
	}

	fmt.Printf("\n=== Final Results ===\n")
	fmt.Printf("Total events parsed: %d\n", totalEvents)
	fmt.Printf("Read offset: %d/%d\n", readOffset, len(data))
	fmt.Printf("Remaining buffer: %d bytes\n", len(buffer))

	// The remaining buffer should be empty or very small (incomplete record at EOF is OK)
	if len(buffer) > 20 {
		t.Errorf("Unexpected large remaining buffer: %d bytes", len(buffer))
	}

	if readOffset != int64(len(data)) {
		t.Errorf("Did not read entire file: %d/%d bytes", readOffset, len(data))
	}

	if totalEvents == 0 {
		t.Errorf("Expected to parse at least some events")
	}
}

func TestSimulateStreamingWithBuffer(t *testing.T) {
	// Completely simulate streaming read scenario, including buffer management
	filePath := getTestEventFile(t)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Simulating streaming with buffer management ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()

	// Simulate file parsing state
	buffer := make([]byte, 0)
	lastValidOffset := int64(0)
	totalEvents := 0

	// Simulate reading data from file, 800 bytes each time
	chunkSize := 800
	fileReadOffset := int64(0)

	for fileReadOffset < int64(len(data)) {
		// Read a chunk of data
		end := fileReadOffset + int64(chunkSize)
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		chunk := data[fileReadOffset:end]

		fmt.Printf("\n--- Read chunk: offset=%d, size=%d ---\n", fileReadOffset, len(chunk))

		// Append to buffer
		buffer = append(buffer, chunk...)
		fmt.Printf("Buffer: %d bytes total\n", len(buffer))

		// Parse buffer
		events, consumed, err := parser.ParseEventsWithBuffer(buffer)
		if err != nil {
			t.Errorf("Parse error at file_offset=%d: %v", fileReadOffset, err)
		}

		fmt.Printf("Parsed: %d events, consumed %d/%d bytes\n",
			len(events), consumed, len(buffer))

		if len(events) > 0 {
			fmt.Printf("Sample event: step=%d\n", events[len(events)-1].Step)
		}

		// Update state
		if consumed > 0 {
			lastValidOffset += int64(consumed)
			buffer = buffer[consumed:]
			fmt.Printf("Updated: lastValidOffset=%d, buffer remaining=%d bytes\n",
				lastValidOffset, len(buffer))
		}

		totalEvents += len(events)
		fileReadOffset = end
	}

	fmt.Printf("\n=== Total: %d events, lastValidOffset=%d ===\n", totalEvents, lastValidOffset)

	if len(buffer) > 0 {
		t.Errorf("Buffer not empty at end: %d bytes remaining", len(buffer))
	}
}
