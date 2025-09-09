package utils

import (
	"errors"
	"sync"
	"time"
)

var (
	runStatus = make(map[string]RunInfo)
	mu        sync.Mutex
	cond      = sync.NewCond(&mu)
)

type RunInfo struct {
	IsRunning bool
	StartTime time.Time
	RunCount  int64
}

var ErrRunning = errors.New("process is running")

const MustWait = -1

// waitTime < 0: 一直等
// waitTime = 0: 不等
// waitTime > 0: 等待指定时间
func SingleRun(key string, waitTime time.Duration, f func() error) (RunInfo, error) {
	mu.Lock()
	for {
		info := runStatus[key]
		if !info.IsRunning {
			// 可以开始执行
			startTime := time.Now()
			runCount := info.RunCount + 1
			runStatus[key] = RunInfo{
				IsRunning: true,
				StartTime: startTime,
				RunCount:  runCount,
			}
			mu.Unlock()

			// 执行任务
			err := f()

			// 更新状态并通知等待者
			mu.Lock()
			runStatus[key] = RunInfo{
				IsRunning: false,
				StartTime: startTime,
				RunCount:  runCount,
			}
			cond.Broadcast()
			mu.Unlock()

			return runStatus[key], err
		}

		// 已在运行
		if waitTime == 0 {
			mu.Unlock()
			return info, ErrRunning
		}

		if waitTime > 0 {
			// 有超时时间
			timeout := time.After(waitTime)
			done := make(chan struct{})
			go func() {
				cond.Wait()
				close(done)
			}()
			mu.Unlock()

			select {
			case <-done:
				// 被唤醒，重新尝试
				mu.Lock()
				waitTime = 0 // 避免重复等待
				continue
			case <-timeout:
				return info, ErrRunning
			}
		}

		// 无限等待
		if waitTime < 0 {
			cond.Wait()
			continue
		}
	}
}
