package tuner

import (
	"errors"
	"fmt"
)

// MockFileSystem 模拟文件系统，用于测试
type MockFileSystem struct {
	files map[string][]byte
	readErrors  map[string]error
	writeErrors map[string]error
}

// NewMockFileSystem 创建模拟文件系统
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files:       make(map[string][]byte),
		readErrors:  make(map[string]error),
		writeErrors: make(map[string]error),
	}
}

// ReadFile 模拟读取文件
func (mfs *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if err, ok := mfs.readErrors[filename]; ok {
		return nil, err
	}
	if data, ok := mfs.files[filename]; ok {
		return data, nil
	}
	return nil, errors.New("file not found")
}

// WriteFile 模拟写入文件
func (mfs *MockFileSystem) WriteFile(filename string, data []byte, perm uint32) error {
	if err, ok := mfs.writeErrors[filename]; ok {
		return err
	}
	mfs.files[filename] = data
	return nil
}

// SetFileContent 设置文件内容（测试用）
func (mfs *MockFileSystem) SetFileContent(filename string, content []byte) {
	mfs.files[filename] = content
}

// GetFileContent 获取文件内容（测试用）
func (mfs *MockFileSystem) GetFileContent(filename string) []byte {
	return mfs.files[filename]
}

// SetReadError 设置读取错误（测试用）
func (mfs *MockFileSystem) SetReadError(filename string, err error) {
	mfs.readErrors[filename] = err
}

// SetWriteError 设置写入错误（测试用）
func (mfs *MockFileSystem) SetWriteError(filename string, err error) {
	mfs.writeErrors[filename] = err
}

// MockCommandExecutor 模拟命令执行器，用于测试
type MockCommandExecutor struct {
	executeCalls [][]string
	executeError error
	outputMap    map[string][]byte
	outputError  error
}

// NewMockCommandExecutor 创建模拟命令执行器
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{
		executeCalls: make([][]string, 0),
		outputMap:    make(map[string][]byte),
	}
}

// Execute 模拟执行命令
func (mce *MockCommandExecutor) Execute(name string, args ...string) error {
	cmd := append([]string{name}, args...)
	mce.executeCalls = append(mce.executeCalls, cmd)
	return mce.executeError
}

// ExecuteWithOutput 模拟执行命令并返回输出
func (mce *MockCommandExecutor) ExecuteWithOutput(name string, args ...string) ([]byte, error) {
	cmd := append([]string{name}, args...)
	mce.executeCalls = append(mce.executeCalls, cmd)
	
	key := fmt.Sprintf("%s %v", name, args)
	if output, ok := mce.outputMap[key]; ok {
		return output, mce.outputError
	}
	return []byte(""), mce.outputError
}

// SetExecuteError 设置执行错误（测试用）
func (mce *MockCommandExecutor) SetExecuteError(err error) {
	mce.executeError = err
}

// GetExecuteCalls 获取所有执行的命令（测试用）
func (mce *MockCommandExecutor) GetExecuteCalls() [][]string {
	return mce.executeCalls
}

// SetOutput 设置命令输出（测试用）
func (mce *MockCommandExecutor) SetOutput(name string, args []string, output []byte) {
	key := fmt.Sprintf("%s %v", name, args)
	mce.outputMap[key] = output
}

