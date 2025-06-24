package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	utils2 "github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Handler 定义不变
type Handler interface {
	Spec() string // cron 表达式
	MaxExecuteTime() time.Duration
	Do(ctx context.Context) error
}

// Scheduler 负责调度生命周期管理
type Scheduler struct {
	c       *cron.Cron
	entries map[string]cron.EntryID
	mu      sync.Mutex
}

// NewScheduler 创建一个支持秒级的调度器
func NewScheduler() *Scheduler {
	return &Scheduler{
		c:       cron.New(cron.WithSeconds()),
		entries: make(map[string]cron.EntryID),
	}
}

// Start 启动调度
func (s *Scheduler) Start() {
	s.c.Start()
	fmt.Println("cron scheduler started")
}

// Stop 停止调度
func (s *Scheduler) Stop() context.Context {
	// 返回一个上下文，用户可等待所有任务优雅退出
	return s.c.Stop()

	//异步做信号监听优雅关闭：<-sched.Stop().Done()
}

// AddJob 如果同名 id 已存在，则直接返回不做任何操作
func (s *Scheduler) AddJob(id string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exist := s.entries[id]; exist {
		// 同名任务已存在，直接跳过
		return nil
	}
	entryID, err := s.c.AddFunc(h.Spec(), func() { cmdHandle(id, h) })
	if err != nil {
		return fmt.Errorf("AddJob failed: %w", err)
	}
	s.entries[id] = entryID
	return nil
}

// ForceAddJob 按业务 id 添加一个任务；如果已存在，先移除再新增
func (s *Scheduler) ForceAddJob(id string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已经存在同名任务，先删掉
	if entry, ok := s.entries[id]; ok {
		s.c.Remove(entry)
	}
	// 注册到 cron
	entryID, err := s.c.AddFunc(h.Spec(), func() {
		cmdHandle(id, h)
	})
	if err != nil {
		return fmt.Errorf("ForceAddJob failed: %w", err)
	}
	s.entries[id] = entryID
	return nil
}

// RemoveJob 删除一个任务
func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("no job with id %q", id)
	}
	s.c.Remove(entry)
	delete(s.entries, id)
	return nil
}

// ListJobs 返回当前所有任务 id 和下次执行时间
func (s *Scheduler) ListJobs() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]time.Time, len(s.entries))
	for id, entryID := range s.entries {
		if e := s.c.Entry(entryID); e.Valid() {
			out[id] = e.Next
		}
	}
	return out
}

// cmdHandle 按原逻辑执行并做幂等 / 告警
func cmdHandle(id string, handle Handler) {
	ctx := coroutines.NewContext("CRON:" + id)
	coroutines.SafeFunc(ctx, func(ctx context.Context) {
		funcName := fmt.Sprintf("%s-%T", id, handle)
		singleRun := utils2.NewSingleRun(funcName)
		runInfo, err := singleRun.SingleRun(func() error {
			return handle.Do(ctx)
		})
		if err != nil {
			if errors.Is(err, utils2.ErrProcessIsRunning) {
				// 当前还在运行，则判断是否超时
				if time.Since(runInfo.StartTime) < handle.MaxExecuteTime() {
					return
				}
				err = e.Err(err, fmt.Sprintf(
					"funcName: %s is still running\nstart: %s\nnow:   %s",
					funcName,
					utils2.NewTime(runInfo.StartTime).Format(time.DateTime),
					utils2.NewLocal().Format(time.DateTime),
				))
			}
			e.SendMessage(ctx, err)
		}
	})
}
