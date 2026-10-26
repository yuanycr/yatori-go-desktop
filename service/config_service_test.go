package service

import (
	"os"
	"testing"
)

func TestValidateConfig_OK(t *testing.T) {
	cfg := defaultConfig()
	if errs := ValidateConfig(cfg); len(errs) != 0 {
		t.Fatalf("期望无错误，得到: %v", errs)
	}
}

func TestNormalizeCoursesCustom_DefaultTaskSwitches(t *testing.T) {
	cc := CoursesCustom{}
	NormalizeCoursesCustom("XUEXITONG", &cc)
	if cc.CxChapterTestSw == nil || *cc.CxChapterTestSw != 1 {
		t.Fatalf("chapter test default = %v, want 1", cc.CxChapterTestSw)
	}
	if cc.CxWorkSw == nil || *cc.CxWorkSw != 0 {
		t.Fatalf("work default = %v, want 0", cc.CxWorkSw)
	}
	if cc.CxExamSw == nil || *cc.CxExamSw != 0 {
		t.Fatalf("exam default = %v, want 0", cc.CxExamSw)
	}
}

func TestNormalizeCoursesCustom_PreservesConfiguredTaskSwitches(t *testing.T) {
	off, on := 0, 1
	cc := CoursesCustom{CxChapterTestSw: &off, CxWorkSw: &on, CxExamSw: &on}
	NormalizeCoursesCustom("XUEXITONG", &cc)
	if *cc.CxChapterTestSw != 0 || *cc.CxWorkSw != 1 || *cc.CxExamSw != 1 {
		t.Fatalf("normalization changed explicit task switches: chapter=%d work=%d exam=%d", *cc.CxChapterTestSw, *cc.CxWorkSw, *cc.CxExamSw)
	}
}

func TestBuildUser_DefaultsMissingTaskSwitches(t *testing.T) {
	user := buildUser(AccountPO{})
	if user.CoursesCustom.CxChapterTestSw == nil || *user.CoursesCustom.CxChapterTestSw != 1 {
		t.Fatalf("worker chapter-test default = %v, want 1", user.CoursesCustom.CxChapterTestSw)
	}
	if user.CoursesCustom.CxWorkSw == nil || *user.CoursesCustom.CxWorkSw != 0 {
		t.Fatalf("worker work default = %v, want 0", user.CoursesCustom.CxWorkSw)
	}
	if user.CoursesCustom.CxExamSw == nil || *user.CoursesCustom.CxExamSw != 0 {
		t.Fatalf("worker exam default = %v, want 0", user.CoursesCustom.CxExamSw)
	}
}

func TestValidateConfig_BadLogLevel(t *testing.T) {
	cfg := defaultConfig()
	cfg.Setting.BasicSetting.LogLevel = "VERBOSE"
	if errs := ValidateConfig(cfg); len(errs) == 0 {
		t.Fatal("期望有错误，但无错误返回")
	}
}

func TestValidateConfig_EmailMissingHost(t *testing.T) {
	cfg := defaultConfig()
	cfg.Setting.EmailInform.Sw = 1
	cfg.Setting.EmailInform.SMTPPort = 465
	if errs := ValidateConfig(cfg); len(errs) == 0 {
		t.Fatal("期望有 SMTP Host 错误")
	}
}

func TestLoadConfig_NotExist(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("不存在时应返回默认配置，得到错误: %v", err)
	}
	if cfg.Setting.BasicSetting.LogLevel != "INFO" {
		t.Error("默认 LogLevel 应为 INFO")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	f, err := os.CreateTemp("", "yatori-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	cfg := defaultConfig()
	cfg.Setting.BasicSetting.LogLevel = "DEBUG"
	cfg.Users = []User{{AccountType: "YINGHUA", Account: "test@test.com", Password: "pw"}}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if loaded.Setting.BasicSetting.LogLevel != "DEBUG" {
		t.Error("LogLevel 未正确保存/加载")
	}
	if len(loaded.Users) != 1 || loaded.Users[0].Account != "test@test.com" {
		t.Error("Users 未正确保存/加载")
	}
}
