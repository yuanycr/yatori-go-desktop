package service

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

type logCapture struct {
	originalOut, originalErr *os.File
	writer                   *os.File
	done                     chan struct{}
}

func PrepareLogLine(line string) string {
	line = strings.TrimRight(RedactDiagnosticText(NormalizeLogText(line)), "\r\n")
	if strings.TrimSpace(line) == "" {
		return ""
	}
	if len(line) > logLineLimit {
		line = strings.ToValidUTF8(line[:logLineLimit], "") + " [日志过长，已截断]"
	}
	if !strings.HasPrefix(line, "[20") {
		line = time.Now().Format("[2006-01-02 15:04:05] ") + line
	}
	return line
}

// Drain oversize lines rather than letting Scanner's token limit terminate
// the pipe and lose every subsequent log (or block a worker on a full pipe).
func readLogLines(r io.Reader, emit func(string)) error {
	reader := bufio.NewReaderSize(r, 4096)
	var line []byte
	truncated := false
	for {
		part, err := reader.ReadSlice('\n')
		remaining := logLineLimit - len(line)
		if len(part) > remaining {
			line = append(line, part[:remaining]...)
			truncated = true
		} else {
			line = append(line, part...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(line) > 0 || truncated {
			text := strings.TrimRight(string(line), "\r\n")
			if truncated {
				text += " [日志过长，已截断]"
			}
			emit(text)
		}
		line = line[:0]
		truncated = false
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (h *LogHub) consumeLogStream(r io.Reader, emit func(string)) {
	err := readLogLines(r, func(line string) {
		if clean := h.Push(line); clean != "" && emit != nil {
			emit(clean)
		}
	})
	if err != nil {
		h.Push("[系统] 日志读取失败: " + err.Error())
	}
}

func (h *LogHub) stopCapture() {
	h.captureMu.Lock()
	c := h.capture
	h.capture = nil
	if c != nil {
		if os.Stdout == c.writer {
			os.Stdout = c.originalOut
		}
		if os.Stderr == c.writer {
			os.Stderr = c.originalErr
		}
		_ = c.writer.Close()
	}
	h.captureMu.Unlock()
	if c != nil {
		<-c.done
	}
}
