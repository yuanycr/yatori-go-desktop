package service

import (
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const logBufSize = 1000

type LogHub struct {
	mu            sync.Mutex
	lines         []string
	disk          *rollingLog
	debugDisk     *rollingLog
	diskError     string
	historyLoaded bool
	closed        bool
	captureMu     sync.Mutex
	capture       *logCapture
}

func NewLogHub() *LogHub { return &LogHub{} }

// ansiRe 匹配所有 ANSI 转义序列
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// Push returns exactly the sanitized text retained in memory and on disk.
// Event publishers must use this return value, never the original payload.
func (h *LogHub) Push(msg string) string {
	msg = PrepareLogLine(msg)
	if strings.TrimSpace(msg) == "" {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, msg)
	h.trimLocked()
	if h.closed {
		return msg
	}
	for _, w := range []*rollingLog{h.disk, h.debugDisk} {
		if w != nil {
			h.persistenceErrorLocked(w.write(msg + "\n"))
		}
	}
	return msg
}

func (h *LogHub) Recent(n int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n <= 0 || n >= len(h.lines) {
		cp := make([]string, len(h.lines))
		copy(cp, h.lines)
		return cp
	}
	src := h.lines[len(h.lines)-n:]
	cp := make([]string, len(src))
	copy(cp, src)
	return cp
}

// HijackStdout keeps diagnostic lines as well as business logs.
func (h *LogHub) HijackStdout(emit func(string)) {
	h.captureMu.Lock()
	defer h.captureMu.Unlock()
	if h.capture != nil {
		return
	}
	r, w, err := os.Pipe()
	if err != nil {
		h.Push("[系统] 日志捕获初始化失败: " + err.Error())
		return
	}
	c := &logCapture{originalOut: os.Stdout, originalErr: os.Stderr, writer: w, done: make(chan struct{})}
	h.capture = c
	os.Stdout = w
	os.Stderr = w

	go func() {
		defer close(c.done)
		defer r.Close()
		h.consumeLogStream(r, emit)
	}()
}

// HijackWriter 返回一个写入 hub 的 io.Writer（备用方案）
func (h *LogHub) HijackWriter(emit func(string)) io.Writer {
	pr, pw := io.Pipe()
	go func() {
		defer pr.Close()
		h.consumeLogStream(pr, emit)
	}()
	return pw
}

// toUTF8 尝试把 GBK 编码的字符串转为 UTF-8；如果已是合法 UTF-8 则原样返回
func toUTF8(s string) string { return NormalizeLogText(s) }
func ToUTF8(s string) string { return NormalizeLogText(s) }

// NormalizeLogText 修复常见编码问题：
//  1. 非法 UTF-8（GBK 字节流被当 UTF-8）→ GBK decode
//  2. mojibake（UTF-8 字节被 Latin-1/Windows-1252 误读后存成 UTF-8）→ 逐 rune codepoint 还原
func NormalizeLogText(s string) string {
	s = stripANSI(s)

	// 已是合法 UTF-8：检查是否包含 mojibake 特征字符
	if utf8.ValidString(s) {
		// 尝试 Latin-1 反解：把每个 rune 的 codepoint 当字节，重新 UTF-8 decode
		if fixed := fixMojibake(s); fixed != s {
			return fixed
		}
		// CJK 字符被 GBK 误解码为其他 CJK 字符（UTF-8→GBK→UTF-8）
		if fixed := fixGBKasMojibake(s); fixed != s {
			return applyResidualMojibakeMap(fixed)
		}
		return s
	}

	// 非法 UTF-8：尝试 GBK decode
	out, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), s)
	if err == nil && utf8.ValidString(out) {
		return out
	}

	return strings.ToValidUTF8(s, "?")
}

// fixMojibake 把每个 rune 的 codepoint 当 Latin-1 字节，尝试还原中文。
// 覆盖两种情况：
//  1. GBK bytes 被 Latin-1 误读后存成 UTF-8（codepoints 均 ≤0xFF，还原字节后可 GBK decode）
//  2. GBK bytes 被 Latin-1 误读后存成 UTF-8，还原字节后直接是合法 UTF-8
func fixMojibake(s string) string {
	runes := []rune(s)
	raw := make([]byte, 0, len(runes))
	for _, r := range runes {
		if r > 0xFF {
			return s // 含非 Latin-1 字符，不是 mojibake
		}
		raw = append(raw, byte(r))
	}
	// 情况1：还原字节后是合法 UTF-8 且含 CJK
	if utf8.Valid(raw) {
		if result := string(raw); hasCJK(result) {
			return result
		}
	}
	// 情况2：还原字节后是 GBK，尝试 GBK decode
	out, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), string(raw))
	if err == nil && utf8.ValidString(out) && hasCJK(out) {
		return out
	}
	return s
}

// fixGBKasMojibake 修复 UTF-8 字节被当 GBK 解码后产生的乱码。
// 逐段处理 CJK 字符，尝试 GBK 编码后作为 UTF-8 解码。
func fixGBKasMojibake(s string) string {
	runes := []rune(s)
	var result strings.Builder
	changed := false
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r < 0x4E00 || r > 0x9FFF {
			result.WriteRune(r)
			i++
			continue
		}
		j := i
		for j < len(runes) && runes[j] >= 0x4E00 && runes[j] <= 0x9FFF {
			j++
		}
		chunk := string(runes[i:j])
		gbkBytes, _, err := transform.String(simplifiedchinese.GBK.NewEncoder(), chunk)
		if err == nil && utf8.ValidString(gbkBytes) && hasCJK(gbkBytes) && gbkBytes != chunk {
			result.WriteString(gbkBytes)
			changed = true
		} else {
			result.WriteString(chunk)
		}
		i = j
	}
	if !changed {
		return s
	}
	return result.String()
}

// applyResidualMojibakeMap 修复 fixGBKasMojibake 后残留的已知乱码词。
var residualMojibakeMap = map[string]string{
	"缁统": "系统",
}

func applyResidualMojibakeMap(s string) string {
	for bad, good := range residualMojibakeMap {
		s = strings.ReplaceAll(s, bad, good)
	}
	return s
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// isCoreLog 判断是否为原核心日志，过滤 Wails/Go runtime 噪音
func isCoreLog(line string) bool {
	for _, kw := range []string{
		"课程", "任务点", "视频", "学时", "提交", "学习", "登录",
		"拉取", "章节", "考试", "完成", "失败", "错误",
		"INFO", "DEBUG", "WARN", "ERROR", "[系统]",
	} {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// TailLog 读取最新日志文件末尾 n 行
func TailLog(n int) ([]string, error) {
	logDir, err := LogDir()
	if err != nil {
		return nil, err
	}
	return tailLogsInDir(logDir, n)
}

func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if n <= 0 || n > logBufSize {
		n = logBufSize
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Read only the end of each file, not an unbounded historic log into RAM.
	start := info.Size() - (2 << 20)
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if i := strings.IndexByte(string(b), '\n'); i >= 0 {
			b = b[i+1:]
		} else {
			return []string{}, nil
		}
	}
	lines := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = RedactDiagnosticText(NormalizeLogText(strings.TrimSuffix(line, "\r")))
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func fixUTF8AsGBKMojibake(s string) string { return fixMojibake(s) }
func normalizeLogText(s string) string     { return NormalizeLogText(s) }
