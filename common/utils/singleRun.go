package utils

import (
	"errors"
	"sync"
	"time"
)

var (
	runStatus = make(map[string]RunInfo)
	mu        sync.Mutex
)

type SingleRun struct {
	Key string
}

type RunInfo struct {
	IsRunning bool
	StartTime time.Time
	RunCount  int64
}

func NewSingleRun(key string) *SingleRun {
	return &SingleRun{Key: key}
}

func (t *SingleRun) setRunningStatus(status bool, startTime time.Time, runCount int64) {
	mu.Lock()
	defer mu.Unlock()
	runStatus[t.Key] = RunInfo{
		IsRunning: status,
		StartTime: startTime,
		RunCount:  runCount,
	}
}

func (t *SingleRun) getRunningInfo() (RunInfo, bool) {
	mu.Lock()
	defer mu.Unlock()
	info, ok := runStatus[t.Key]
	if !ok {
		return RunInfo{}, false
	}
	return info, true
}

var ErrProcessIsRunning = errors.New("process is running")

func (t *SingleRun) SingleRun(f func() error) (RunInfo, error) {
	info, running := t.getRunningInfo()
	if running && info.IsRunning {
		return info, ErrProcessIsRunning
	}

	startTime := time.Now()
	runCount := info.RunCount + 1
	t.setRunningStatus(true, startTime, runCount)
	defer t.setRunningStatus(false, startTime, runCount)

	err := f()
	info, _ = t.getRunningInfo()

	return info, err
}
