package tensorboard

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestParseRealEventFile(t *testing.T) {
	// 读取实际的 event 文件
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-b4wgk-master-0.335.0"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read event file: %v", err)
	}

	fmt.Printf("File size: %d bytes\n", len(data))

	// 创建解析器
	parser := NewEventParser()

	// 使用带缓冲的解析
	events, consumed, err := parser.ParseEventsWithBuffer(data)
	if err != nil {
		t.Errorf("Parse error: %v", err)
	}

	fmt.Printf("Parsed %d events, consumed %d/%d bytes\n", len(events), consumed, len(data))

	// 打印前几个事件
	for i, event := range events {
		if i >= 5 {
			break
		}
		fmt.Printf("Event %d: step=%d, wall_time=%.2f, scalars=%v\n",
			i, event.Step, event.WallTime, event.Scalars)
	}

	if len(events) == 0 {
		t.Error("Expected to parse at least some events")
	}
}

func TestParseEventFileInChunks(t *testing.T) {
	// 读取实际的 event 文件
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-b4wgk-master-0.335.0"
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
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-b4wgk-master-0.335.0"
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
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-b4wgk-master-0.335.0"
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

func TestSimulateStreamingWithBuffer(t *testing.T) {
	// 完全模拟流式读取的场景，包括缓冲区管理
	filePath := "../../data/events.out.tfevents.1765203259.primus-exporter-test-b4wgk-master-0.335.0"
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

