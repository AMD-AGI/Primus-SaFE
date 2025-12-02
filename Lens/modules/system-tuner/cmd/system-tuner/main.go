package main

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/system-tuner/pkg/tuner"
)

const (
	// CheckInterval 检查间隔时间
	CheckInterval = 30 * time.Second
)

func main() {
	fmt.Println("System-Tuner v0.2")

	// 创建真实的文件系统和命令执行器
	fs := &tuner.OSFileSystem{}
	cmdExec := &tuner.OSCommandExecutor{}

	// 使用默认配置创建系统调优器
	config := tuner.DefaultConfig()
	systemTuner := tuner.NewSystemTuner(config, fs, cmdExec)

	// 持续循环检查和调优系统参数
	for {
		// 检查并设置 vm.max_map_count
		if err := systemTuner.CheckAndSetMaxMapCount(); err != nil {
			fmt.Printf("错误: %v\n", err)
		}

		// 检查并设置最大打开文件数
		if err := systemTuner.CheckAndSetMaxOpenFiles(); err != nil {
			fmt.Printf("错误: %v\n", err)
		}

		// 等待下一次检查
		time.Sleep(CheckInterval)
	}
}
