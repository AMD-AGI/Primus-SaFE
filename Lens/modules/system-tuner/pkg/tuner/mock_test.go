package tuner

import (
	"errors"
	"fmt"
)

// MockFileSystem mock file system for testing
type MockFileSystem struct {
	files       map[string][]byte
	readErrors  map[string]error
	writeErrors map[string]error
}

// NewMockFileSystem creates a mock file system
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files:       make(map[string][]byte),
		readErrors:  make(map[string]error),
		writeErrors: make(map[string]error),
	}
}

// ReadFile mock reading a file
func (mfs *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if err, ok := mfs.readErrors[filename]; ok {
		return nil, err
	}
	if data, ok := mfs.files[filename]; ok {
		return data, nil
	}
	return nil, errors.New("file not found")
}

// WriteFile mock writing a file
func (mfs *MockFileSystem) WriteFile(filename string, data []byte, perm uint32) error {
	if err, ok := mfs.writeErrors[filename]; ok {
		return err
	}
	mfs.files[filename] = data
	return nil
}

// SetFileContent sets file content (for testing)
func (mfs *MockFileSystem) SetFileContent(filename string, content []byte) {
	mfs.files[filename] = content
}

// GetFileContent gets file content (for testing)
func (mfs *MockFileSystem) GetFileContent(filename string) []byte {
	return mfs.files[filename]
}

// SetReadError sets read error (for testing)
func (mfs *MockFileSystem) SetReadError(filename string, err error) {
	mfs.readErrors[filename] = err
}

// SetWriteError sets write error (for testing)
func (mfs *MockFileSystem) SetWriteError(filename string, err error) {
	mfs.writeErrors[filename] = err
}

// MockCommandExecutor mock command executor for testing
type MockCommandExecutor struct {
	executeCalls [][]string
	executeError error
	outputMap    map[string][]byte
	outputError  error
}

// NewMockCommandExecutor creates a mock command executor
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{
		executeCalls: make([][]string, 0),
		outputMap:    make(map[string][]byte),
	}
}

// Execute mock executing a command
func (mce *MockCommandExecutor) Execute(name string, args ...string) error {
	cmd := append([]string{name}, args...)
	mce.executeCalls = append(mce.executeCalls, cmd)
	return mce.executeError
}

// ExecuteWithOutput mock executing a command and returning output
func (mce *MockCommandExecutor) ExecuteWithOutput(name string, args ...string) ([]byte, error) {
	cmd := append([]string{name}, args...)
	mce.executeCalls = append(mce.executeCalls, cmd)

	key := fmt.Sprintf("%s %v", name, args)
	if output, ok := mce.outputMap[key]; ok {
		return output, mce.outputError
	}
	return []byte(""), mce.outputError
}

// SetExecuteError sets execution error (for testing)
func (mce *MockCommandExecutor) SetExecuteError(err error) {
	mce.executeError = err
}

// GetExecuteCalls gets all executed commands (for testing)
func (mce *MockCommandExecutor) GetExecuteCalls() [][]string {
	return mce.executeCalls
}

// SetOutput sets command output (for testing)
func (mce *MockCommandExecutor) SetOutput(name string, args []string, output []byte) {
	key := fmt.Sprintf("%s %v", name, args)
	mce.outputMap[key] = output
}
