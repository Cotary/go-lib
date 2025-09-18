package utils

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSingleRun_FirstRun(t *testing.T) {
	key := "firstRun"
	called := false

	info, err := SingleRun(key, NoWait, func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("function was not called")
	}
	if info.RunCount != 1 {
		t.Fatalf("expected RunCount=1, got %d", info.RunCount)
	}
}

func TestSingleRun_NoWaitAlreadyRunning(t *testing.T) {
	key := "noWait"
	started := make(chan struct{})
	block := make(chan struct{})

	// 第一个任务阻塞
	go func() {
		_, _ = SingleRun(key, NoWait, func() error {
			close(started)
			<-block
			return nil
		})
	}()

	<-started // 确保第一个任务已开始

	// 第二个任务应立即返回 ErrRunning
	info, err := SingleRun(key, NoWait, func() error {
		return nil
	})
	if !errors.Is(err, ErrRunning) {
		t.Fatalf("expected ErrRunning, got %v", err)
	}
	if info.RunCount != 1 {
		t.Fatalf("expected RunCount=1, got %d", info.RunCount)
	}

	close(block) // 释放第一个任务
}

func TestSingleRun_MustWait(t *testing.T) {
	key := "mustWait"
	started := make(chan struct{})
	block := make(chan struct{})
	done := make(chan struct{})

	// 第一个任务阻塞
	go func() {
		_, _ = SingleRun(key, MustWait, func() error {
			close(started)
			<-block
			return nil
		})
	}()

	<-started // 确保第一个任务已开始

	// 第二个任务会等待第一个任务完成
	go func() {
		info, err := SingleRun(key, MustWait, func() error {
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if info.RunCount != 2 {
			t.Errorf("expected RunCount=2, got %d", info.RunCount)
		}
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	close(block) // 释放第一个任务
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second run did not complete in time")
	}
}

func TestSingleRun_Timeout(t *testing.T) {
	key := "timeout"
	started := make(chan struct{})
	block := make(chan struct{})

	// 第一个任务阻塞
	go func() {
		_, _ = SingleRun(key, NoWait, func() error {
			close(started)
			<-block
			return nil
		})
	}()

	<-started // 确保第一个任务已开始

	// 第二个任务等待 100ms 后超时
	info, err := SingleRun(key, 3*time.Second, func() error {
		return nil
	})
	if !errors.Is(err, ErrRunning) {
		t.Fatalf("expected ErrRunning, got %v", err)
	}
	if info.RunCount != 1 {
		t.Fatalf("expected RunCount=1, got %d", info.RunCount)
	}

	close(block) // 释放第一个任务
}

func TestSingleRun_DifferentKeys(t *testing.T) {
	var mu sync.Mutex
	var executed []string

	run := func(key string) {
		_, _ = SingleRun(key, NoWait, func() error {
			mu.Lock()
			executed = append(executed, key)
			mu.Unlock()
			return nil
		})
	}

	run("key1")
	run("key2")

	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(executed))
	}
	if executed[0] == executed[1] {
		t.Fatal("expected different keys to run independently")
	}
}
