package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const debugDumpLimit = 2 << 20
const debugDumpKeep = 20

var debugDumpMu sync.Mutex

// Page snapshots are opt-in diagnostics, not a record of every request.
func debugDumpsEnabled() (bool, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return false, err
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return false, err
	}
	return cfg.Setting.BasicSetting.LogOutFileSw == 1 && strings.EqualFold(cfg.Setting.BasicSetting.LogLevel, "DEBUG"), nil
}

// Diagnostic HTML is deliberately non-executable. Discard scripts, form values
// and comments; sanitize URLs/attributes as well as text. Never store raw cookies.
func sanitizeDebugHTML(raw string) string {
	if len(raw) > debugDumpLimit {
		raw = strings.ToValidUTF8(raw[:debugDumpLimit], "")
	}
	z := html.NewTokenizer(strings.NewReader(raw))
	var out strings.Builder
	skip := false
	for {
		type_ := z.Next()
		if type_ == html.ErrorToken {
			break
		}
		token := z.Token()
		if token.Data == "script" || token.Data == "style" {
			if type_ == html.StartTagToken {
				skip = true
			}
			if type_ == html.EndTagToken {
				skip = false
			}
			continue
		}
		if skip || type_ == html.CommentToken || type_ == html.DoctypeToken {
			continue
		}
		if type_ == html.StartTagToken || type_ == html.SelfClosingTagToken {
			attrs := token.Attr[:0]
			for _, a := range token.Attr {
				if strings.HasPrefix(a.Key, "on") || a.Key == "srcdoc" {
					continue
				}
				if (token.Data == "input" && a.Key == "value") || strings.HasPrefix(a.Key, "data-") {
					a.Val = "[REDACTED]"
				} else {
					a.Val = RedactDiagnosticText(a.Val)
					if a.Key == "src" || a.Key == "href" || a.Key == "action" {
						a.Val = "[URL omitted]"
					}
				}
				attrs = append(attrs, a)
			}
			token.Attr = attrs
		}
		if type_ == html.TextToken {
			token.Data = RedactDiagnosticText(token.Data)
		}
		out.WriteString(token.String())
	}
	return out.String()
}

func saveDebugSnapshot(dir, metadata, page string) (string, error) {
	debugDumpMu.Lock()
	defer debugDumpMu.Unlock()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "xxt-exam-"+time.Now().Format("20060102-150405")+"-*.html")
	if err != nil {
		return "", err
	}
	// CSP also prevents network requests if a snapshot is opened in a browser.
	content := `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">` +
		"<pre>" + html.EscapeString(RedactDiagnosticText(metadata)) + "</pre>\n" + sanitizeDebugHTML(page)
	if len(content) > debugDumpLimit {
		content = strings.ToValidUTF8(content[:debugDumpLimit], "")
	}
	_, writeErr := io.WriteString(f, content)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var files []logFileInfo
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "xxt-exam-") || !strings.HasSuffix(e.Name(), ".html") || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return "", err
		}
		if info.Mode().IsRegular() {
			files = append(files, logFileInfo{filepath.Join(dir, e.Name()), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	kept := 1
	for _, entry := range files {
		if entry.path == f.Name() {
			continue
		}
		if kept < debugDumpKeep {
			kept++
			continue
		}
		if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("debug 清理失败: %w", err)
		}
	}
	return f.Name(), nil
}
