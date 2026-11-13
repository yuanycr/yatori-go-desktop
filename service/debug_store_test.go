package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	xxt "github.com/yatori-dev/yatori-go-core/aggregation/xuexitong"
)

func TestDebugSnapshotsUniqueBoundedAndSanitized(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy.html")
	if err := os.WriteFile(legacy, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 25; i++ {
		path, err := saveDebugSnapshot(dir, "request: https://test/?token=SENSITIVE", `<script>secret='SENSITIVE'</script><input name="enc" value="SENSITIVE"><a href="https://test/?token=SENSITIVE">question</a>`)
		if err != nil {
			t.Fatal(err)
		}
		if seen[path] {
			t.Fatal("snapshot overwritten")
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "SENSITIVE") || strings.Contains(string(data), "<script") {
			t.Fatal("unsafe snapshot")
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != debugDumpKeep+1 {
		t.Fatalf("retention %d", len(files))
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("removed legacy dump")
	}
	p, err := saveDebugSnapshot(dir, "meta", strings.Repeat("large ", debugDumpLimit))
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	if info.Size() > debugDumpLimit {
		t.Fatal("unbounded snapshot")
	}
}

func TestDebugDumpConfigSwitchAndFailure(t *testing.T) {
	dir := t.TempDir()
	SetDataDir(dir)
	defer SetDataDir("")
	cfg := defaultConfig()
	path := filepath.Join(dir, "config.yaml")
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	exam := &xxt.XXTExam{Name: "same", TaskRefId: "123"}
	if p, err := xxtSaveExamEnterDebugDump(exam, "r", "f", "<p>test</p>"); err != nil || p != "" {
		t.Fatalf("INFO wrote debug: %s %v", p, err)
	}
	cfg.Setting.BasicSetting.LogLevel = "DEBUG"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if p, err := xxtSaveExamEnterDebugDump(exam, "r", "f", "<p>test</p>"); err != nil || p == "" {
		t.Fatalf("DEBUG missing: %s %v", p, err)
	}
	cfg.Setting.BasicSetting.LogOutFileSw = 0
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if p, err := xxtSaveExamEnterDebugDump(exam, "r", "f", "<p>test</p>"); err != nil || p != "" {
		t.Fatalf("disabled wrote debug: %s %v", p, err)
	}
	block := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(block, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveDebugSnapshot(filepath.Join(block, "child"), "meta", "page"); err == nil {
		t.Fatal("write failure swallowed")
	}
}
