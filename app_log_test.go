package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"yatori-go-desktop/service"
)

func TestAppLogAPIUsesOneHistorySource(t *testing.T) {
	h := service.NewLogHub()
	defer h.Close()
	if err := h.ConfigureFiles(t.TempDir(), true, true); err != nil {
		t.Fatal(err)
	}
	line := h.Push("[INFO] saved once")
	a := &App{logHub: h}
	for i := 0; i < 3; i++ {
		result := a.GetRecentLogs(1000)
		if !result.Ok || !reflect.DeepEqual(result.Data, []string{line}) {
			t.Fatalf("duplicate history: %+v", result)
		}
	}
	if got := a.TailLogFile(0); !got.Ok || len(got.Data) != 1 {
		t.Fatalf("zero limit: %+v", got)
	}
}

func TestAppLogConfigUsesIsolatedDataDirectory(t *testing.T) {
	dir := t.TempDir()
	service.SetDataDir(dir)
	defer service.SetDataDir("")
	cfg, err := service.LoadConfig(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Setting.BasicSetting.LogLevel = "DEBUG"
	a := &App{logHub: service.NewLogHub()}
	defer a.logHub.Close()
	if err := a.configureLogging(cfg); err != nil {
		t.Fatal(err)
	}
	line := a.logHub.Push("configuration applied")
	restarted := service.NewLogHub()
	defer restarted.Close()
	if err := restarted.ConfigureFiles(dir, true, true); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Recent(20); !reflect.DeepEqual(got, []string{line}) {
		t.Fatalf("history: %q", got)
	}
}
