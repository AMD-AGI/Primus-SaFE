package tuner

import (
	"os"
	"os/exec"
)

// OSFileSystem 实现 FileSystem 接口，使用真实的操作系统调用
type OSFileSystem struct{}

// ReadFile 读取文件内容
func (fs *OSFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteFile 写入文件内容
func (fs *OSFileSystem) WriteFile(filename string, data []byte, perm uint32) error {
	return os.WriteFile(filename, data, os.FileMode(perm))
}

// OSCommandExecutor 实现 CommandExecutor 接口，使用真实的命令执行
type OSCommandExecutor struct{}

// Execute 执行命令
func (ce *OSCommandExecutor) Execute(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// ExecuteWithOutput 执行命令并返回输出
func (ce *OSCommandExecutor) ExecuteWithOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

