package tuner

// FileSystem defines file system operation interface for mocking in tests
type FileSystem interface {
	// ReadFile reads file content
	ReadFile(filename string) ([]byte, error)
	// WriteFile writes file content
	WriteFile(filename string, data []byte, perm uint32) error
}

// CommandExecutor defines command execution interface for mocking in tests
type CommandExecutor interface {
	// Execute executes a command and returns an error
	Execute(name string, args ...string) error
	// ExecuteWithOutput executes a command and returns output
	ExecuteWithOutput(name string, args ...string) ([]byte, error)
}
