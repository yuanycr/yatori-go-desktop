//go:build android

package mobilecore

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"sync"
)

var captureOnce sync.Once

// Upstream platform runners use fmt.Println rather than the mobile emit callback.
// Drain their output once per process so progress reaches the mobile log buffer.
func initPlatformLogs() {
	captureOnce.Do(func() {
		reader, writer, err := os.Pipe()
		if err != nil {
			pushLog("system", "平台日志接入失败: "+err.Error())
			return
		}
		os.Stdout = writer
		go func() {
			defer reader.Close()
			ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 4096), 1024*1024)
			for scanner.Scan() {
				line := ansi.ReplaceAllString(scanner.Text(), "")
				state.Lock()
				state.logs = appendLimited(state.logs, line)
				for uid, task := range state.tasks {
					if task.State == "running" && task.Account != "" && strings.Contains(line, "["+task.Account+"]") {
						task.LastLog = line
						state.tasks[uid] = task
					}
				}
				state.Unlock()
			}
			if err := scanner.Err(); err != nil {
				pushLog("system", "平台日志读取失败: "+err.Error())
			}
		}()
	})
}
