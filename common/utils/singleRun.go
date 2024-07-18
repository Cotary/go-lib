package utils

import (
	"github.com/pkg/errors"
	"sync"
)

// 配置只能单次运行的方法

var runStatus sync.Map

type SingleRun struct {
	Key string
}

func NewSingleRun(key string) *SingleRun {
	run := new(SingleRun)
	run.Key = key
	return run
}

func (t *SingleRun) markAsRunning() {
	runStatus.Store(t.Key, true)
}
func (t *SingleRun) markAsNotRunning() {
	runStatus.Store(t.Key, false)
}
func (t *SingleRun) isRunning() bool {
	running, _ := runStatus.Load(t.Key)
	if run, ok := running.(bool); ok {
		return run
	}
	return false
}
func (t *SingleRun) CheckRunning() bool {
	return t.isRunning()
}

var ErrProcessIsRunning = errors.New("process is running")

func (t *SingleRun) SingleRun(f func() error) error {
	if t.isRunning() {
		return ErrProcessIsRunning
	}
	t.markAsRunning()
	defer t.markAsNotRunning()
	return f()

}
