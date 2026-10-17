package service

import (
	"sync"
	"testing"
)

func TestTaskManager_PreventDuplicate(t *testing.T) {
	mgr := NewTaskManager(nil)
	mgr.mu.Lock()
	mgr.tasks["uid-1"] = &taskEntry{status: TaskStatus{UID: "uid-1", State: StateRunning}}
	mgr.mu.Unlock()

	if !mgr.IsRunning("uid-1") {
		t.Fatal("uid-1 应为 running")
	}
	if mgr.IsRunning("uid-2") {
		t.Fatal("uid-2 未启动，应为 false")
	}
}

func TestTaskManager_StopIdempotent(t *testing.T) {
	mgr := NewTaskManager(nil)
	mgr.Stop("not-exist") // 不应 panic

	mgr.mu.Lock()
	mgr.tasks["uid-2"] = &taskEntry{
		status:  TaskStatus{UID: "uid-2", State: StateRunning},
		stopped: false,
		logDone: make(chan struct{}),
	}
	mgr.mu.Unlock()

	mgr.Stop("uid-2")
	mgr.Stop("uid-2") // 幂等，不 panic

	for _, s := range mgr.Statuses() {
		if s.State == StateRunning {
			t.Errorf("停止后 %s 仍为 running", s.UID)
		}
	}
}

func TestTaskManager_StopAll(t *testing.T) {
	mgr := NewTaskManager(nil)
	for _, uid := range []string{"a", "b", "c"} {
		u := uid
		mgr.mu.Lock()
		mgr.tasks[u] = &taskEntry{
			status:  TaskStatus{UID: u, State: StateRunning},
			logDone: make(chan struct{}),
		}
		mgr.mu.Unlock()
	}
	mgr.StopAll()
	for _, s := range mgr.Statuses() {
		if s.State == StateRunning {
			t.Errorf("StopAll 后 %s 仍为 running", s.UID)
		}
	}
}

func TestTaskManager_AutoStopIsScopedToCurrentEntry(t *testing.T) {
	var mu sync.Mutex
	var emitted []string
	mgr := NewTaskManager(func(uid, msg string) {
		mu.Lock()
		emitted = append(emitted, uid+":"+msg)
		mu.Unlock()
	})
	e := &taskEntry{
		status:  TaskStatus{UID: "timer-uid", State: StateRunning},
		logDone: make(chan struct{}),
	}
	e.timerGeneration = 7
	mgr.mu.Lock()
	mgr.tasks["timer-uid"] = e
	mgr.mu.Unlock()

	// A stale callback must not stop the task.
	mgr.autoStop("timer-uid", e, 6, 1)
	if !mgr.IsRunning("timer-uid") {
		t.Fatal("stale timer callback stopped the running task")
	}

	// The current callback stops only this account and emits a useful log.
	mgr.autoStop("timer-uid", e, 7, 1)
	if mgr.IsRunning("timer-uid") {
		t.Fatal("current timer callback did not stop the task")
	}
	mu.Lock()
	gotLogs := len(emitted)
	mu.Unlock()
	if gotLogs == 0 {
		t.Fatal("auto-stop did not emit a log")
	}
}

func TestTaskManager_SetRunDurationValidation(t *testing.T) {
	mgr := NewTaskManager(nil)
	if err := mgr.SetRunDuration("missing", -1); err == nil {
		t.Fatal("negative run duration should be rejected")
	}
	if err := mgr.SetRunDuration("missing", 0); err != nil {
		t.Fatalf("zero run duration should disable timer: %v", err)
	}
}

func TestDecodePassword(t *testing.T) {
	plain := "test-password-123"
	// base64("test-password-123") = "dGVzdC1wYXNzd29yZC0xMjM="
	enc := "b64:dGVzdC1wYXNzd29yZC0xMjM="
	if got := decodePassword(enc); got != plain {
		t.Errorf("want %q got %q", plain, got)
	}
	if got := decodePassword("plain"); got != "plain" {
		t.Error("非混淆密码应原样返回")
	}
}
