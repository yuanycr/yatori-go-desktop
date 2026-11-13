package service

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogPersistenceRestartAndSwitches(t *testing.T) {
	dir := t.TempDir()
	h := NewLogHub()
	defer h.Close()
	if err := h.ConfigureFiles(dir, true, true); err != nil {
		t.Fatal(err)
	}
	line := h.Push("[INFO] 登录完成 token=secret-abc")
	if strings.Contains(line, "secret-abc") {
		t.Fatal("secret retained")
	}
	for _, folder := range []string{"assets/log", "debug/logs"} {
		lines, err := tailLogsInDir(filepath.Join(dir, folder), 20)
		if err != nil || !reflect.DeepEqual(lines, []string{line}) {
			t.Fatalf("%s: %q %v", folder, lines, err)
		}
	}
	if err := h.ConfigureFiles(dir, false, true); err != nil {
		t.Fatal(err)
	}
	h.Push("not persisted")
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	next := NewLogHub()
	defer next.Close()
	if err := next.ConfigureFiles(dir, true, false); err != nil {
		t.Fatal(err)
	}
	if got := next.Recent(20); !reflect.DeepEqual(got, []string{line}) {
		t.Fatalf("restart: %q", got)
	}
	next.Push("new session")
	if err := next.ConfigureFiles(dir, true, false); err != nil {
		t.Fatal(err)
	}
	if got := next.Recent(20); len(got) != 2 {
		t.Fatalf("duplicate reload: %q", got)
	}
	lines, err := tailLogsInDir(filepath.Join(dir, "debug/logs"), 20)
	if err != nil || len(lines) != 1 {
		t.Fatalf("DEBUG disable failed: %q %v", lines, err)
	}
}

func TestLogRotationRetentionAndLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "log-old.txt")
	if err := os.WriteFile(legacy, []byte("legacy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := &rollingLog{dir: dir, limit: 30, keep: 3}
	defer w.close()
	for i := 0; i < 10; i++ {
		if err := w.write(fmt.Sprintf("%02d-%s\n", i, strings.Repeat("x", 25))); err != nil {
			t.Fatal(err)
		}
	}
	files, err := orderedLogFiles(dir, true)
	if err != nil || len(files) != 3 {
		t.Fatalf("retention=%d err=%v", len(files), err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("deleted foreign log", err)
	}
	for _, f := range files {
		info, _ := os.Stat(f.path)
		if info.Size() > 30 {
			t.Fatal("oversize", info.Size())
		}
	}
}

func TestLogHistoryOrderAndBounds(t *testing.T) {
	dir := t.TempDir()
	for i, body := range []string{"a\nb\n", "c\nd\n"} {
		p := filepath.Join(dir, fmt.Sprintf("log-%d.txt", i))
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		ts := time.Unix(int64(100+i), 0)
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	got, err := tailLogsInDir(dir, 3)
	if err != nil || !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Fatalf("%q %v", got, err)
	}
	h := NewLogHub()
	for i := 0; i < 2000; i++ {
		h.Push(strings.Repeat("x", logLineLimit))
	}
	bytes := 0
	for _, s := range h.Recent(0) {
		bytes += len(s)
	}
	if bytes > logMemoryLimit || len(h.Recent(0)) > logBufSize {
		t.Fatal("unbounded memory", bytes)
	}
}

func TestLogWriteFailureAndRecovery(t *testing.T) {
	dir := t.TempDir()
	block := filepath.Join(dir, "block")
	if err := os.WriteFile(block, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	h := NewLogHub()
	defer h.Close()
	if err := h.ConfigureFiles(block, true, true); err == nil {
		t.Fatal("expected configuration error")
	}
	h.disk = &rollingLog{dir: filepath.Join(block, "logs"), limit: 64, keep: 2}
	h.Push("still visible")
	if !strings.Contains(strings.Join(h.Recent(20), "\n"), "日志写入失败") {
		t.Fatal("write error hidden")
	}
	if err := h.ConfigureFiles(dir, true, false); err != nil {
		t.Fatal(err)
	}
	h.Push("recovered")
	got, err := tailLogsInDir(filepath.Join(dir, "assets/log"), 20)
	if err != nil || len(got) != 1 || !strings.Contains(got[0], "recovered") {
		t.Fatalf("%q %v", got, err)
	}
}

func TestLogConcurrentAccounts(t *testing.T) {
	dir := t.TempDir()
	h := NewLogHub()
	defer h.Close()
	if err := h.ConfigureFiles(dir, true, false); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for account := 0; account < 5; account++ {
		wg.Add(1)
		go func(a int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				h.Push(fmt.Sprintf("account-%d entry-%d", a, i))
				h.Recent(20)
			}
		}(account)
	}
	wg.Wait()
	got, err := tailLogsInDir(filepath.Join(dir, "assets/log"), 1000)
	if err != nil || len(got) != 250 {
		t.Fatalf("lines=%d err=%v", len(got), err)
	}
}

func TestLogStreamOversizeContinuesAndRedactsEvents(t *testing.T) {
	h := NewLogHub()
	var emitted []string
	h.consumeLogStream(strings.NewReader(strings.Repeat("x", 2<<20)+"\nAuthorization: Bearer private-secret\nlast line"), func(s string) { emitted = append(emitted, s) })
	if len(emitted) != 3 || !strings.Contains(emitted[0], "截断") || !strings.Contains(emitted[2], "last line") {
		t.Fatalf("bad stream: %d", len(emitted))
	}
	if strings.Contains(strings.Join(emitted, "\n"), "private-secret") {
		t.Fatal("event secret leak")
	}
	if !reflect.DeepEqual(emitted, h.Recent(10)) {
		t.Fatal("event/history mismatch")
	}
}

func TestLogCaptureCloseDrainsStdoutAndStderr(t *testing.T) {
	h := NewLogHub()
	dir := t.TempDir()
	if err := h.ConfigureFiles(dir, true, true); err != nil {
		t.Fatal(err)
	}
	beforeOut, beforeErr := os.Stdout, os.Stderr
	h.HijackStdout(nil)
	_, _ = fmt.Fprintln(os.Stdout, "stdout-marker")
	_, _ = fmt.Fprintln(os.Stderr, "stderr-marker")
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if os.Stdout != beforeOut || os.Stderr != beforeErr {
		t.Fatal("streams not restored")
	}
	got, err := tailLogsInDir(filepath.Join(dir, "assets/log"), 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("lost trailing output: %q %v", got, err)
	}
	h.Push("after close")
	got, _ = tailLogsInDir(filepath.Join(dir, "assets/log"), 10)
	if len(got) != 2 {
		t.Fatal("closed logger reopened")
	}
}

func TestDashboardUsesLiveLogHubWithoutFiles(t *testing.T) {
	defer setupTestDB(t)()
	h := NewLogHub()
	line := h.Push("dashboard-live")
	d, err := GetDashboard(NewTaskManager(nil), filepath.Join(t.TempDir(), "config.yaml"), h)
	if err != nil || !reflect.DeepEqual(d.RecentLogs, []string{line}) {
		t.Fatalf("%q %v", d.RecentLogs, err)
	}
}

func TestWorkerDrainsStoppedLogsWithoutChangingNewRun(t *testing.T) {
	h := NewLogHub()
	mgr := NewTaskManager(func(_ string, msg string) { h.Push(msg) })
	old := &taskEntry{stopped: true}
	next := &taskEntry{status: TaskStatus{LastLog: "new-run"}}
	mgr.tasks["a"] = next
	mgr.pipeLog("a", old, strings.NewReader("final error token=hidden\n"))
	if next.status.LastLog != "new-run" {
		t.Fatal("old worker overwrote new status")
	}
	got := h.Recent(10)
	if len(got) != 1 || strings.Contains(got[0], "hidden") {
		t.Fatalf("trailing logs lost or unredacted: %q", got)
	}
}

func TestLogRedaction(t *testing.T) {
	for _, s := range []string{`password="SENSITIVE"`, `{"API_KEY":"SENSITIVE"}`, `https://example.test/?enc=SENSITIVE&x=2`, "Cookie: sid=SENSITIVE; other=SENSITIVE", "authorization=Bearer SENSITIVE", `pwd='SENSITIVE'`} {
		if got := RedactDiagnosticText(s); strings.Contains(got, "SENSITIVE") {
			t.Errorf("redaction failed: %s", got)
		}
	}
}
