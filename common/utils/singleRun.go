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

var ErrProcessIsRunning = errors.New("process is running")

func (t *SingleRun) SingleRun(f func() error) (RunInfo, error) {
	mu.Lock()
	info := runStatus[t.Key]
	if info.IsRunning {
		mu.Unlock()
		return info, ErrProcessIsRunning
	}

	startTime := time.Now()
	runCount := info.RunCount + 1
	runStatus[t.Key] = RunInfo{
		IsRunning: true,
		StartTime: startTime,
		RunCount:  runCount,
	}
	mu.Unlock()

	err := f()

	runStatus[t.Key] = RunInfo{
		IsRunning: false,
		StartTime: startTime,
		RunCount:  runCount,
	}

	return runStatus[t.Key], err
}
