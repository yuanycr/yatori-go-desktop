package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const logFileLimit int64 = 4 << 20
const logFileKeep = 12
const logLineLimit = 16 << 10
const logMemoryLimit = 2 << 20

// Each desktop process owns its files. Workers forward stdout/stderr to the
// desktop, avoiding duplicate writes and timestamp collisions between workers.
type rollingLog struct {
	dir   string
	file  *os.File
	size  int64
	limit int64
	keep  int
}

func (w *rollingLog) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Sync()
	closeErr := w.file.Close()
	if err == nil {
		err = closeErr
	}
	w.file = nil
	return err
}

func (w *rollingLog) write(line string) error {
	if w.file == nil || w.size+int64(len(line)) > w.limit {
		if err := w.close(); err != nil {
			return err
		}
		if err := os.MkdirAll(w.dir, 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(w.dir, "desktop-"+time.Now().Format("20060102-150405")+"-*.log")
		if err != nil {
			return err
		}
		w.file, w.size = f, 0
		if err := w.prune(); err != nil {
			_ = w.close()
			return err
		}
	}
	n, err := io.WriteString(w.file, line)
	w.size += int64(n)
	return err
}

func (w *rollingLog) prune() error {
	files, err := orderedLogFiles(w.dir, true)
	if err != nil {
		return err
	}
	keep := 0
	for _, f := range files {
		// Never remove a file that was not created by this logger, or our open file.
		if w.file != nil && f.path == w.file.Name() {
			continue
		}
		keep++
		if keep >= w.keep {
			if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

type logFileInfo struct {
	path string
	mod  time.Time
}

func orderedLogFiles(dir string, ownedOnly bool) ([]logFileInfo, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []logFileInfo
	for _, e := range entries {
		name := e.Name()
		owned := strings.HasPrefix(name, "desktop-") && strings.HasSuffix(name, ".log")
		legacy := strings.HasPrefix(name, "log") && (strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".log"))
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 || (!owned && (ownedOnly || !legacy)) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logFileInfo{filepath.Join(dir, name), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].path > files[j].path
		}
		return files[i].mod.After(files[j].mod)
	})
	return files, nil
}

// ConfigureFiles applies the file-output switch without clearing live logs.
// DEBUG additionally mirrors diagnostics to debug/logs (not HTML page dumps).
func (h *LogHub) ConfigureFiles(dataDir string, enabled, debug bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return fmt.Errorf("日志服务已关闭")
	}
	if h.disk != nil {
		if err := h.disk.close(); err != nil {
			return err
		}
		h.disk = nil
	}
	if h.debugDisk != nil {
		if err := h.debugDisk.close(); err != nil {
			return err
		}
		h.debugDisk = nil
	}
	h.diskError = ""
	if !h.historyLoaded {
		lines, err := tailLogsInDir(filepath.Join(dataDir, "assets", "log"), logBufSize)
		if err != nil {
			h.persistenceErrorLocked(fmt.Errorf("读取历史日志: %w", err))
		}
		h.lines = append(lines, h.lines...)
		h.trimLocked()
		h.historyLoaded = true
	}
	if !enabled {
		return nil
	}
	dir := filepath.Join(dataDir, "assets", "log")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	h.disk = &rollingLog{dir: dir, limit: logFileLimit, keep: logFileKeep}
	if debug {
		dir = filepath.Join(dataDir, "debug", "logs")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		h.debugDisk = &rollingLog{dir: dir, limit: logFileLimit, keep: logFileKeep}
	}
	return nil
}

func (h *LogHub) trimLocked() {
	bytes, start := 0, len(h.lines)
	for start > 0 && len(h.lines)-start < logBufSize {
		if bytes+len(h.lines[start-1]) > logMemoryLimit {
			break
		}
		start--
		bytes += len(h.lines[start])
	}
	if start > 0 {
		copy(h.lines, h.lines[start:])
		kept := len(h.lines) - start
		clear(h.lines[kept:])
		h.lines = h.lines[:kept]
	}
}

func (h *LogHub) Close() error {
	h.stopCapture()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	var first error
	for _, w := range []*rollingLog{h.disk, h.debugDisk} {
		if w != nil {
			if err := w.close(); first == nil {
				first = err
			}
		}
	}
	h.disk, h.debugDisk = nil, nil
	return first
}

func (h *LogHub) persistenceErrorLocked(err error) {
	if err == nil || h.diskError == err.Error() {
		return
	}
	h.diskError = err.Error()
	h.lines = append(h.lines, fmt.Sprintf("[系统] 日志写入失败: %v", err))
	h.trimLocked()
}

var credentialAssignment = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|api[_-]?key|access[_-]?token|refresh[_-]?token|token|cookie|authorization|enc|enc_task|signature|sign|secret|validate|captcha|exam[_-]?code)["']?\s*[:=]\s*)("[^"\r\n]*"|'[^'\r\n]*'|[^\s&,;<>]+)`)
var sensitiveHeader = regexp.MustCompile(`(?im)\b(?:authorization|cookie|set-cookie)\s*:\s*[^\r\n]+`)
var bearerCredential = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+`)

// Best-effort field redaction; do not persist full config/account payloads.
func RedactDiagnosticText(text string) string {
	text = bearerCredential.ReplaceAllString(text, "${1} [REDACTED]")
	text = sensitiveHeader.ReplaceAllStringFunc(text, func(s string) string {
		return s[:strings.IndexByte(s, ':')+1] + " [REDACTED]"
	})
	return credentialAssignment.ReplaceAllString(text, "${1}[REDACTED]")
}

func tailLogsInDir(dir string, n int) ([]string, error) {
	if n <= 0 || n > logBufSize {
		n = logBufSize
	}
	files, err := orderedLogFiles(dir, false)
	if err != nil {
		return nil, err
	}
	lines := []string{}
	for _, f := range files {
		part, err := tailFile(f.path, n-len(lines))
		if os.IsNotExist(err) { // a rotation may have removed it
			continue
		}
		if err != nil {
			return lines, err
		}
		lines = append(part, lines...)
		if len(lines) >= n {
			break
		}
	}
	return lines, nil
}
