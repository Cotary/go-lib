// Package cmd 提供基于 gocron/v2 的定时任务调度能力。
//
// 使用方式：
//
//  1. 实现 Handler 接口
//
//     type MyJob struct{}
//
//     func (j MyJob) Spec() string              { return "0 */5 * * * *" }  // 每 5 分钟（支持 6 位秒级 cron）
//     func (j MyJob) MaxExecuteTime() time.Duration { return 2 * time.Minute }  // 超过此时长会触发告警
//     func (j MyJob) Do(ctx context.Context) error  { /* 业务逻辑 */ return nil }
//
//  2. 创建调度器并注册任务
//
//     sched, err := cmd.NewScheduler()
//     sched.AddJob("sync-orders", MyJob{})       // 同名 id 幂等，不会重复注册
//     sched.ForceAddJob("sync-orders", MyJobV2{}) // 强制替换已有同名任务
//     sched.Start()
//
//  3. 优雅关闭（Shutdown 会阻塞直到所有运行中的任务完成）
//
//     if err := sched.Stop(); err != nil { ... }
//
// 内置特性：
//   - SingletonMode：同一任务上一次未完成时，本次调度自动跳过，不会并发执行
//   - 超时告警：任务执行耗时超过 MaxExecuteTime 时自动发送告警
//   - Panic Recovery：任务内 panic 会被捕获并通过 SendErrMessage 告警，不影响调度器
//   - 错误上报：任务返回 error 时自动通过 AfterJobRunsWithError 事件上报
package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/notify"
)

// Handler 定义一个定时任务需要实现的接口。
//   - Spec: 返回 cron 表达式，支持标准 5 位和秒级 6 位格式（如 "*/5 * * * * *"）
//   - MaxExecuteTime: 任务最大预期执行时间，超过后会触发告警（不会取消任务）
//   - Do: 任务执行逻辑，ctx 携带 RequestID 等链路信息
type Handler interface {
	Spec() string
	MaxExecuteTime() time.Duration
	Do(ctx context.Context) error
}

// Scheduler 基于 gocron/v2 的定时任务调度器，支持按业务 id 管理任务的增删查。
type Scheduler struct {
	s       gocron.Scheduler
	entries map[string]gocron.Job
	mu      sync.RWMutex
}

// NewScheduler 创建调度器实例。创建后需调用 Start 启动、Stop 关闭。
func NewScheduler() (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("NewScheduler: %w", err)
	}
	return &Scheduler{
		s:       s,
		entries: make(map[string]gocron.Job),
	}, nil
}

// Start 启动调度器，开始按计划执行已注册的任务。
func (s *Scheduler) Start() {
	s.s.Start()
}

// Stop 停止调度器并阻塞等待所有运行中的任务完成后返回。
func (s *Scheduler) Stop() error {
	return s.s.Shutdown()
}

// AddJob 如果同名 id 已存在，则直接返回不做任何操作
func (s *Scheduler) AddJob(id string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exist := s.entries[id]; exist {
		return nil
	}
	return s.addJobLocked(id, h)
}

// ForceAddJob 按业务 id 添加一个任务；如果已存在，先注册新任务再移除旧任务，避免中间态丢失
func (s *Scheduler) ForceAddJob(id string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldJob, hasOld := s.entries[id]

	if err := s.addJobLocked(id, h); err != nil {
		return err
	}

	if hasOld {
		_ = s.s.RemoveJob(oldJob.ID())
	}
	return nil
}

func (s *Scheduler) addJobLocked(id string, h Handler) error {
	j, err := s.s.NewJob(
		gocron.CronJob(h.Spec(), true),
		gocron.NewTask(wrapTask(id, h)),
		gocron.WithName(id),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
		gocron.WithEventListeners(
			gocron.AfterJobRunsWithError(func(jobID uuid.UUID, jobName string, jobErr error) {
				ctx := coroutines.NewContext("CRON:" + jobName)
				notify.SendErrMessage(ctx, jobErr)
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("AddJob %q failed: %w", id, err)
	}
	s.entries[id] = j
	return nil
}

// RemoveJob 删除一个任务
func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("no job with id %q", id)
	}
	if err := s.s.RemoveJob(j.ID()); err != nil {
		return fmt.Errorf("RemoveJob %q: %w", id, err)
	}
	delete(s.entries, id)
	return nil
}

// ListJobs 返回当前所有任务 id 和下次执行时间
func (s *Scheduler) ListJobs() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]time.Time, len(s.entries))
	for id, j := range s.entries {
		nextRun, _ := j.NextRun()
		if !nextRun.IsZero() {
			out[id] = nextRun
		}
	}
	return out
}

func wrapTask(id string, h Handler) func() {
	return func() {
		ctx := coroutines.NewContext("CRON:" + id)
		coroutines.SafeFunc(ctx, func(ctx context.Context) {
			start := time.Now()
			err := h.Do(ctx)
			elapsed := time.Since(start)

			if err != nil {
				notify.SendErrMessage(ctx, err)
			}
			if elapsed > h.MaxExecuteTime() {
				notify.SendErrMessage(ctx, fmt.Errorf(
					"job %s exceeded max execute time: elapsed %s, max %s",
					id, elapsed, h.MaxExecuteTime(),
				))
			}
		})
	}
}
