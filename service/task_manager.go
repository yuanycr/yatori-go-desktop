package service

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

func sysLog(format string, args ...interface{}) string {
	return fmt.Sprintf("[%s] [系统] %s",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))
}

type TaskState string

const (
	StateRunning TaskState = "running"
	StateStopped TaskState = "stopped"
	StateFailed  TaskState = "failed"
)

type TaskStatus struct {
	UID       string    `json:"uid"`
	Account   string    `json:"account"`
	Platform  string    `json:"platform"`
	State     TaskState `json:"state"`
	StartTime string    `json:"startTime,omitempty"`
	LastLog   string    `json:"lastLog,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type taskEntry struct {
	mu              sync.Mutex
	status          TaskStatus
	cmd             *exec.Cmd
	stopped         bool
	logDone         chan struct{}
	timer           *time.Timer
	timerGeneration uint64
}

type LogEmitter func(uid, msg string)

type TaskManager struct {
	mu         sync.RWMutex
	tasks      map[string]*taskEntry
	emitter    LogEmitter
	maxWorkers int
}

const maxConcurrentAccountTasks = 5

func NewTaskManager(emitter LogEmitter) *TaskManager {
	return &TaskManager{tasks: make(map[string]*taskEntry), emitter: emitter, maxWorkers: maxConcurrentAccountTasks}
}

func (m *TaskManager) SetConfigPath(_ string) {}

// SetMaxWorkers remains for compatibility with older callers. Desktop task
// concurrency is deliberately fixed at five accounts.
func (m *TaskManager) SetMaxWorkers(_ int) {
	m.mu.Lock()
	m.maxWorkers = maxConcurrentAccountTasks
	m.mu.Unlock()
}

func (m *TaskManager) HasRunningTasks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runningCount() > 0
}

func (m *TaskManager) runningCount() int {
	count := 0
	for _, e := range m.tasks {
		e.mu.Lock()
		if e.status.State == StateRunning {
			count++
		}
		e.mu.Unlock()
	}
	return count
}

func (m *TaskManager) Start(uid string) error {
	po, err := getAccountPO(uid)
	if err != nil {
		return fmt.Errorf("账号不存在: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.tasks[uid]; ok {
		e.mu.Lock()
		state := e.status.State
		e.mu.Unlock()
		if state == StateRunning {
			return fmt.Errorf("账号 %s 已在运行中", uid)
		}
	}

	running := m.runningCount()
	if running >= m.maxWorkers {
		return fmt.Errorf("当前运行任务数已达上限：%d", m.maxWorkers)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取可执行路径: %w", err)
	}

	cmd := exec.Command(exe, "--worker", "--uid", uid)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法创建 stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 worker 失败: %w", err)
	}

	e := &taskEntry{
		status: TaskStatus{
			UID:       uid,
			Account:   po.Account,
			Platform:  po.AccountType,
			State:     StateRunning,
			StartTime: time.Now().Format("2006-01-02 15:04:05"),
		},
		cmd:     cmd,
		stopped: false,
		logDone: make(chan struct{}),
	}
	m.tasks[uid] = e
	if po.RunDurationMinutes > 0 {
		m.scheduleRunDurationLocked(uid, e, po.RunDurationMinutes)
	}

	if m.emitter != nil {
		m.emitter(uid, sysLog("worker 并发：当前 %d/%d", running+1, m.maxWorkers))
		m.emitter(uid, sysLog("worker 子进程已启动 pid=%d", cmd.Process.Pid))
	}

	go m.pipeLog(uid, e, stdout)
	go func() {
		_ = cmd.Wait()
		m.mu.RLock()
		entry, ok := m.tasks[uid]
		m.mu.RUnlock()
		// A stopped task may be started again before the old worker exits. Only
		// update the entry belonging to this worker; never mark the new run as
		// stopped or cancel its timer.
		if ok && entry == e {
			entry.mu.Lock()
			if entry.status.State == StateRunning {
				entry.status.State = StateStopped
			}
			entry.timerGeneration++
			if entry.timer != nil {
				entry.timer.Stop()
				entry.timer = nil
			}
			entry.mu.Unlock()
		}
	}()

	return nil
}

func (m *TaskManager) Stop(uid string) {
	m.mu.RLock()
	e, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok {
		return
	}
	m.stopEntry(uid, e)
}

// stopEntry stops e only if it is still the current task for uid. The identity
// check prevents a timer/old worker from stopping a newer run after a restart.
func (m *TaskManager) stopEntry(uid string, e *taskEntry) {
	m.stopEntryAtGeneration(uid, e, 0, false)
}

// stopEntryAtGeneration is used by timer callbacks. The generation check is
// performed while holding e.mu, so a concurrent duration update cannot race
// between callback validation and killing the task.
func (m *TaskManager) stopEntryAtGeneration(uid string, e *taskEntry, generation uint64, checkGeneration bool) {
	m.mu.RLock()
	current, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok || current != e {
		return
	}

	e.mu.Lock()
	if checkGeneration && e.timerGeneration != generation {
		e.mu.Unlock()
		return
	}
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	e.timerGeneration++
	timer := e.timer
	e.timer = nil
	pid := 0
	if e.cmd != nil && e.cmd.Process != nil {
		pid = e.cmd.Process.Pid
	}
	e.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}

	if e.logDone != nil {
		select {
		case <-e.logDone:
		default:
			close(e.logDone)
		}
	}

	if pid > 0 {
		if out, err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput(); err != nil {
			if m.emitter != nil {
				m.emitter(uid, sysLog("taskkill pid=%d 失败: %v output=%s", pid, err, string(out)))
			}
		}
		if e.cmd.Process != nil {
			_ = e.cmd.Process.Kill()
		}
	}

	e.mu.Lock()
	e.status.State = StateStopped
	e.mu.Unlock()

	if m.emitter != nil {
		m.emitter(uid, sysLog("已强制停止任务 pid=%d", pid))
	}
}

// SetRunDuration updates the automatic stop timer for an already running
// account. A value of zero disables the timer. It is intentionally scoped to
// the account's current task and does not affect any other account.
func (m *TaskManager) SetRunDuration(uid string, minutes int) error {
	if minutes < 0 {
		return fmt.Errorf("运行时长不能为负数")
	}
	m.mu.RLock()
	e, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	e.mu.Lock()
	e.timerGeneration++
	gen := e.timerGeneration
	old := e.timer
	e.timer = nil
	running := e.status.State == StateRunning && !e.stopped
	if running && minutes > 0 {
		e.timer = time.AfterFunc(time.Duration(minutes)*time.Minute, func() {
			m.autoStop(uid, e, gen, minutes)
		})
	}
	e.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	return nil
}

// scheduleRunDurationLocked schedules the timer for a freshly-created entry.
// m.mu must be held by the caller; e.mu is acquired here.
func (m *TaskManager) scheduleRunDurationLocked(uid string, e *taskEntry, minutes int) {
	if minutes <= 0 {
		return
	}
	e.mu.Lock()
	e.timerGeneration++
	gen := e.timerGeneration
	e.timer = time.AfterFunc(time.Duration(minutes)*time.Minute, func() {
		m.autoStop(uid, e, gen, minutes)
	})
	e.mu.Unlock()
}

func (m *TaskManager) autoStop(uid string, e *taskEntry, generation uint64, minutes int) {
	m.mu.RLock()
	current, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok || current != e {
		return
	}
	e.mu.Lock()
	valid := e.timerGeneration == generation && e.status.State == StateRunning && !e.stopped
	e.mu.Unlock()
	if !valid {
		return
	}
	if m.emitter != nil {
		m.emitter(uid, sysLog("账号运行时长已到，自动停止任务（%d 分钟）", minutes))
	}
	m.stopEntryAtGeneration(uid, e, generation, true)
}

func (m *TaskManager) StopAll() {
	m.mu.RLock()
	uids := make([]string, 0, len(m.tasks))
	for uid := range m.tasks {
		uids = append(uids, uid)
	}
	m.mu.RUnlock()
	for _, uid := range uids {
		m.Stop(uid)
	}
}

func (m *TaskManager) pipeLog(uid string, e *taskEntry, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-e.logDone:
			for scanner.Scan() {
			}
			return
		default:
		}
		e.mu.Lock()
		stopped := e.stopped
		e.mu.Unlock()
		if stopped {
			for scanner.Scan() {
			}
			return
		}
		line := NormalizeLogText(scanner.Text())
		if m.emitter != nil {
			m.emitter(uid, line)
		}
		m.setLog(uid, line)
	}
}

func (m *TaskManager) Statuses() []TaskStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]TaskStatus, 0, len(m.tasks))
	for _, e := range m.tasks {
		e.mu.Lock()
		out = append(out, e.status)
		e.mu.Unlock()
	}
	return out
}

func (m *TaskManager) IsRunning(uid string) bool {
	m.mu.RLock()
	e, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status.State == StateRunning
}

func (m *TaskManager) setLog(uid, msg string) {
	m.mu.RLock()
	e, ok := m.tasks[uid]
	m.mu.RUnlock()
	if !ok {
		return
	}
	e.mu.Lock()
	e.status.LastLog = msg
	e.mu.Unlock()
}

func DecodePassword(enc string) string { return decodePassword(enc) }
func decodePassword(enc string) string {
	const prefix = "b64:"
	if len(enc) <= len(prefix) {
		return enc
	}
	b, err := base64.StdEncoding.DecodeString(enc[len(prefix):])
	if err != nil {
		return enc
	}
	return string(b)
}
