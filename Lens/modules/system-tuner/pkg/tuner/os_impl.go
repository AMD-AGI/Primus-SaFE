package tuner

import (
	"os"
	"os/exec"
)

// OSFileSystem implements FileSystem interface using real OS calls
type OSFileSystem struct{}

// ReadFile reads file content
func (fs *OSFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteFile writes file content
func (fs *OSFileSystem) WriteFile(filename string, data []byte, perm uint32) error {
	return os.WriteFile(filename, data, os.FileMode(perm))
}

// OSCommandExecutor implements CommandExecutor interface using real command execution
type OSCommandExecutor struct{}

// Execute executes a command
func (ce *OSCommandExecutor) Execute(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// ExecuteWithOutput executes a command and returns output
func (ce *OSCommandExecutor) ExecuteWithOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}
