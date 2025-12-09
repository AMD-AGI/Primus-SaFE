package tensorboard

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestParseRealEventFile(t *testing.T) {
	// Test parsing a real TensorBoard event file
	// This file contains both metadata events (text_summary) and scalar metric events
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
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
	// 读取实际的 event 文件
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing chunked parsing ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	// 创建解析器
	parser := NewEventParser()

	// 模拟流式读取：每次读取 64KB
	chunkSize := 64 * 1024
	buffer := make([]byte, 0)
	totalEvents := 0
	offset := 0

	for offset < len(data) {
		// 读取一块数据
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		offset = end

		// 追加到缓冲区
		buffer = append(buffer, chunk...)
		fmt.Printf("\nChunk: added %d bytes, buffer now %d bytes\n", len(chunk), len(buffer))

		// 解析缓冲区
		events, consumed, err := parser.ParseEventsWithBuffer(buffer)
		if err != nil {
			t.Errorf("Parse error at offset %d: %v", offset, err)
		}

		fmt.Printf("Parsed %d events, consumed %d bytes, remaining %d bytes\n",
			len(events), consumed, len(buffer)-consumed)

		// 更新缓冲区
		if consumed > 0 {
			buffer = buffer[consumed:]
		}

		totalEvents += len(events)

		// 打印第一个事件
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

	// 读取实际文件的第一个 record
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	if len(data) < 12 {
		t.Fatal("File too small")
	}

	// 读取前 12 字节
	fmt.Printf("\nFirst 20 bytes (hex): ")
	for i := 0; i < 20 && i < len(data); i++ {
		fmt.Printf("%02x ", data[i])
	}
	fmt.Printf("\n")

	// 解析第一个 record 的 header
	lengthRead := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
		uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56

	lengthCRC := uint32(data[8]) | uint32(data[9])<<8 | uint32(data[10])<<16 | uint32(data[11])<<24

	fmt.Printf("Length: %d (0x%x)\n", lengthRead, lengthRead)
	fmt.Printf("Length CRC from file: 0x%08x\n", lengthCRC)

	// 计算 CRC
	computedCRC := parser.maskedCRC32(data[0:8])
	fmt.Printf("Computed CRC: 0x%08x\n", computedCRC)

	if computedCRC != lengthCRC {
		fmt.Printf("CRC MISMATCH! Expected 0x%08x, got 0x%08x\n", lengthCRC, computedCRC)
	} else {
		fmt.Printf("CRC MATCH!\n")
	}
}

func TestParseFromMiddleOffset(t *testing.T) {
	// 测试从文件中间开始解析的情况
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing parsing from middle offset ===\n")

	// 先完整解析一次，找到某个合适的 offset
	parser := NewEventParser()
	_, consumedFirst, _ := parser.ParseEventsWithBuffer(data[:100000])

	fmt.Printf("First 100KB parsed, consumed %d bytes\n", consumedFirst)

	// 现在从 consumedFirst 位置开始解析剩余数据
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
	// This test demonstrates that TensorBoard records metrics with TWO different step values:
	// 1. Training iteration number (step=1,2,3,...)
	// 2. Total samples processed (step=128,256,384,... for batch_size=128)
	// Metrics with "vs samples" suffix use sample count as step

	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	parser := NewEventParser()
	events, _, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("\n=== TensorBoard Step Semantics ===\n")
	fmt.Printf("TensorBoard records each metric twice with different step values:\n")
	fmt.Printf("  1. By iteration: 'metric' uses training step (1,2,3,...)\n")
	fmt.Printf("  2. By samples: 'metric vs samples' uses total samples (batch_size * step)\n\n")

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

	fmt.Printf("Step progression (first 5 occurrences):\n\n")
	fmt.Printf("By iteration:\n")
	for tag, steps := range tagSteps {
		if len(steps) > 0 && steps[0] < 100 {
			fmt.Printf("  %-40s: %v\n", tag, steps)
		}
	}

	fmt.Printf("\nBy samples (note: step = iteration * batch_size):\n")
	for tag, steps := range tagSteps {
		if len(steps) > 0 && steps[0] >= 100 {
			fmt.Printf("  %-40s: %v\n", tag, steps)
		}
	}

	// Verify the relationship: step(vs samples) = step(normal) * batch_size
	fmt.Printf("\n=== Verifying Relationship: samples_step = iteration_step * batch_size ===\n")

	// Find matching events at same wall_time
	const tolerance = 0.1 // seconds
	matchCount := 0
	for i, event1 := range events {
		if matchCount >= 3 {
			break
		}

		if len(event1.Scalars) > 0 {
			for tag1 := range event1.Scalars {
				if tag1 == "learning-rate" && event1.Step > 100 && event1.Step < 200 {
					// Find corresponding "vs samples" event
					for j, event2 := range events {
						if j <= i {
							continue
						}
						for tag2 := range event2.Scalars {
							if tag2 == "learning-rate vs samples" {
								timeDiff := event2.WallTime - event1.WallTime
								if timeDiff >= 0 && timeDiff < tolerance {
									expectedSamples := event1.Step * 128
									fmt.Printf("Iteration %d: step=%d, samples_step=%d, expected=%d, match=%v\n",
										matchCount+1, event1.Step, event2.Step, expectedSamples,
										event2.Step == expectedSamples)
									matchCount++
									goto nextMatch
								}
							}
						}
					}
				}
			}
		}
	nextMatch:
	}
}

func TestVerifyFileIntegrity(t *testing.T) {
	// Test if the file itself has any corruption
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
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
	// Analyze the specific offset where CRC errors occur
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Analyzing offset 259628 error ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	// Check if offset is valid
	errorOffset := 259628
	if errorOffset > len(data) {
		t.Fatalf("Error offset %d exceeds file size %d", errorOffset, len(data))
	}

	// First, parse the file completely to find valid events around this offset
	parser := NewEventParser()
	events, consumed, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("\nComplete parse: %d events, consumed %d/%d bytes\n", len(events), consumed, len(data))

	// Now simulate the streaming scenario that causes the error
	// The error likely occurs when we read a chunk that ends at offset 259628
	fmt.Printf("\n=== Simulating stream read ending at offset %d ===\n", errorOffset)

	// Parse up to the error offset
	chunk1 := data[:errorOffset]
	events1, consumed1, err := parser.ParseEventsWithBuffer(chunk1)
	if err != nil {
		t.Errorf("Parse error for chunk1: %v", err)
	}

	fmt.Printf("Chunk1 (0-%d): parsed %d events, consumed %d bytes\n",
		errorOffset, len(events1), consumed1)
	fmt.Printf("Remaining in buffer: %d bytes\n", len(chunk1)-consumed1)

	// Show the bytes around the error offset
	start := errorOffset - 50
	if start < 0 {
		start = 0
	}
	end := errorOffset + 50
	if end > len(data) {
		end = len(data)
	}

	fmt.Printf("\nBytes around offset %d:\n", errorOffset)
	fmt.Printf("Offset %d-%d (hex): ", start, end)
	for i := start; i < end; i++ {
		if i == errorOffset {
			fmt.Printf("[%02x] ", data[i])
		} else {
			fmt.Printf("%02x ", data[i])
		}
	}
	fmt.Printf("\n")

	// The problem: if we try to parse starting from an incomplete event
	fmt.Printf("\n=== The Issue ===\n")
	fmt.Printf("If stream read returns chunk ending at offset %d,\n", errorOffset)
	fmt.Printf("and consumed only %d bytes, there are %d bytes remaining.\n",
		consumed1, len(chunk1)-consumed1)
	fmt.Printf("These %d bytes are an incomplete event that should be:\n", len(chunk1)-consumed1)
	fmt.Printf("  1. Kept in buffer\n")
	fmt.Printf("  2. NOT parsed until next chunk arrives\n")
}

func TestStreamReaderWithIncompleteEvents(t *testing.T) {
	// Test that demonstrates the proper handling of incomplete events at chunk boundaries
	// This simulates the exact scenario that caused the CRC errors reported
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Testing Stream Reader with Incomplete Event Handling ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()

	// Simulate stream reader state
	fileOffset := int64(0)
	incompleteBuffer := make([]byte, 0)
	chunkSize := 80000 // Typical chunk size that caused the issue

	totalEvents := 0
	iteration := 0

	for fileOffset < int64(len(data)) {
		iteration++

		// Simulate reading a chunk from file
		remaining := int64(len(data)) - fileOffset
		toRead := int64(chunkSize)
		if toRead > remaining {
			toRead = remaining
		}

		// Read from file
		newData := data[fileOffset : fileOffset+toRead]

		// Combine with incomplete buffer from previous read
		combinedData := append(incompleteBuffer, newData...)

		fmt.Printf("\nIteration %d:\n", iteration)
		fmt.Printf("  File offset: %d\n", fileOffset)
		fmt.Printf("  Read %d bytes from file\n", len(newData))
		fmt.Printf("  Incomplete buffer: %d bytes\n", len(incompleteBuffer))
		fmt.Printf("  Combined data: %d bytes\n", len(combinedData))

		// Parse events
		events, consumed, parseErr := parser.ParseEventsWithBuffer(combinedData)
		if parseErr != nil {
			t.Errorf("Parse error at iteration %d: %v", iteration, parseErr)
		}

		fmt.Printf("  Parsed: %d events, consumed %d bytes\n", len(events), consumed)

		// Calculate how much of the file data was consumed
		consumedFromBuffer := 0
		if len(incompleteBuffer) > 0 {
			if consumed >= len(incompleteBuffer) {
				consumedFromBuffer = len(incompleteBuffer)
			} else {
				consumedFromBuffer = consumed
			}
		}
		consumedFromFile := consumed - consumedFromBuffer

		// Update file offset only for consumed data
		fileOffset += int64(consumedFromFile)

		// Save incomplete data for next iteration
		if consumed < len(combinedData) {
			incompleteBuffer = make([]byte, len(combinedData)-consumed)
			copy(incompleteBuffer, combinedData[consumed:])
			fmt.Printf("  Updated: file_offset=%d, incomplete_buffer=%d bytes\n",
				fileOffset, len(incompleteBuffer))
		} else {
			incompleteBuffer = nil
			fmt.Printf("  Updated: file_offset=%d, no incomplete data\n", fileOffset)
		}

		totalEvents += len(events)

		// Safety check
		if iteration > 100 {
			t.Fatalf("Too many iterations, possible infinite loop")
		}

		// Check for progress
		if consumed == 0 && len(newData) > 0 {
			t.Errorf("No progress at iteration %d, offset %d", iteration, fileOffset)
			break
		}
	}

	fmt.Printf("\n=== Final Results ===\n")
	fmt.Printf("Total events parsed: %d\n", totalEvents)
	fmt.Printf("Final file offset: %d/%d\n", fileOffset, len(data))
	fmt.Printf("Incomplete buffer: %d bytes\n", len(incompleteBuffer))

	if len(incompleteBuffer) > 0 {
		t.Errorf("Incomplete buffer not empty at end: %d bytes", len(incompleteBuffer))
	}

	if fileOffset != int64(len(data)) {
		t.Errorf("Did not read entire file: %d/%d bytes", fileOffset, len(data))
	}
}

func TestSimulateStreamingWithBuffer(t *testing.T) {
	// 完全模拟流式读取的场景，包括缓冲区管理
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-vl4wv-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("\n=== Simulating streaming with buffer management ===\n")
	fmt.Printf("File size: %d bytes\n", len(data))

	parser := NewEventParser()

	// 模拟文件解析状态
	buffer := make([]byte, 0)
	lastValidOffset := int64(0)
	totalEvents := 0

	// 模拟从文件读取数据，每次读取 80000 字节（接近用户报告的大小）
	chunkSize := 80000
	fileReadOffset := int64(0)

	for fileReadOffset < int64(len(data)) {
		// 读取一块数据
		end := fileReadOffset + int64(chunkSize)
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		chunk := data[fileReadOffset:end]

		fmt.Printf("\n--- Read chunk: offset=%d, size=%d ---\n", fileReadOffset, len(chunk))

		// 追加到缓冲区
		buffer = append(buffer, chunk...)
		fmt.Printf("Buffer: %d bytes total\n", len(buffer))

		// 解析缓冲区
		events, consumed, err := parser.ParseEventsWithBuffer(buffer)
		if err != nil {
			t.Errorf("Parse error at file_offset=%d: %v", fileReadOffset, err)
		}

		fmt.Printf("Parsed: %d events, consumed %d/%d bytes\n",
			len(events), consumed, len(buffer))

		if len(events) > 0 {
			fmt.Printf("Sample event: step=%d\n", events[len(events)-1].Step)
		}

		// 更新状态
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
