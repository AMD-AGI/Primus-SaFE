package tuner

// FileSystem 定义文件系统操作接口，便于测试时 mock
type FileSystem interface {
	// ReadFile 读取文件内容
	ReadFile(filename string) ([]byte, error)
	// WriteFile 写入文件内容
	WriteFile(filename string, data []byte, perm uint32) error
}

// CommandExecutor 定义命令执行接口，便于测试时 mock
type CommandExecutor interface {
	// Execute 执行命令并返回错误
	Execute(name string, args ...string) error
	// ExecuteWithOutput 执行命令并返回输出
	ExecuteWithOutput(name string, args ...string) ([]byte, error)
}
