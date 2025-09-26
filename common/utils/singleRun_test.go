package utils

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestSingleRun_NoWait 测试不等待场景
func TestSingleRun_NoWait(t *testing.T) {
	key := "test_no_wait"

	// 启动一个长时间运行的任务
	done := make(chan bool)
	go func() {
		_, err := DefaultManager.SingleRun(key, NoWait, func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		done <- true
	}()

	// 等待第一个任务开始
	time.Sleep(10 * time.Millisecond)

	// 尝试立即执行另一个任务（应该失败）
	_, err := DefaultManager.SingleRun(key, NoWait, func() error {
		return nil
	})

	if err != ErrRunning {
		t.Errorf("Expected ErrRunning, got %v", err)
	}

	// 等待第一个任务完成
	<-done
}

// TestSingleRun_Wait 测试等待场景
func TestSingleRun_Wait(t *testing.T) {
	key := "test_wait"
	// 启动一个长时间运行的任务
	start := time.Now()
	done := make(chan bool)
	go func() {
		_, err := DefaultManager.SingleRun(key, NoWait, func() error {
			time.Sleep(10 * time.Second)
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		done <- true
	}()

	// 等待第一个任务开始
	time.Sleep(10 * time.Millisecond)
	fmt.Println("first task started", time.Since(start).Seconds())
	_, err := DefaultManager.SingleRun(key, 5*time.Second, func() error {
		return nil
	})
	fmt.Println("first task end", time.Since(start).Seconds())

	if err != ErrRunning {
		t.Errorf("Expected ErrRunning, got %v", err)
	}
	fmt.Println("second task started", time.Since(start).Seconds())
	_, err = DefaultManager.SingleRun(key, 5*time.Second, func() error {
		return nil
	})
	fmt.Println("second task end", time.Since(start).Seconds())
	if err != nil {
		t.Errorf("Expected ErrRunning, got %v", err)
	}

	// 等待第一个任务完成
	<-done
}

// TestSingleRun_InfiniteWait 测试无限等待场景
func TestSingleRun_InfiniteWait(t *testing.T) {
	key := "test_infinite_wait"

	// 启动一个任务
	start := time.Now()
	done := make(chan bool)
	go func() {
		_, err := DefaultManager.SingleRun(key, NoWait, func() error {
			fmt.Println("execute task1")
			time.Sleep(5 * time.Second)
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		done <- true
	}()

	// 等待第一个任务开始
	time.Sleep(10 * time.Millisecond)

	// 无限等待第一个任务完成
	fmt.Println("first task started", time.Since(start).Seconds())
	_, err := DefaultManager.SingleRun(key, MustWait, func() error {
		fmt.Println("execute task2")
		return nil
	})
	fmt.Println("first task end", time.Since(start).Seconds())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	// 等待第一个任务完成
	<-done
}

// TestSingleRun_Count 测试并发场景
func TestSingleRun_Count_MustWait(t *testing.T) {
	key := "test_count_mustWait"
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = DefaultManager.SingleRun(key, MustWait, func() error {
				num += 1
				return nil
			})
		}(i)
	}

	wg.Wait()
	fmt.Println("num", num)
	// 由于SingleRun确保同一个key同时只有一个函数执行，所以应该只执行1次
	if num != numGoroutines {
		t.Errorf("Expected num to be %d, got %d", numGoroutines, num)
	}

}

// TestSingleRun_Count 测试并发场景
func TestSingleRun_Count_Wait(t *testing.T) {
	key := "test_count_wait"
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = DefaultManager.SingleRun(key, 5*time.Second, func() error {
				num += 1
				return nil
			})
		}(i)
	}

	wg.Wait()
	fmt.Println("num", num)
	// 由于SingleRun确保同一个key同时只有一个函数执行，所以应该只执行1次
	if num != numGoroutines {
		t.Errorf("Expected num to be %d, got %d", numGoroutines, num)
	}

}

func TestSingleRun_Count_NoWait(t *testing.T) {
	key := "test_count_nowait"
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = DefaultManager.SingleRun(key, NoWait, func() error {
				num += 1
				time.Sleep(5 * time.Second)
				return nil
			})
		}(i)
	}

	wg.Wait()
	fmt.Println("num", num)
	// 由于SingleRun确保同一个key同时只有一个函数执行，所以应该只执行1次
	if num != 1 {
		t.Errorf("Expected num to be %d, got %d", 1, num)
	}

}

// TestSingleRun_Concurrent 测试并发场景
func TestSingleRun_Concurrent(t *testing.T) {
	key := "test_concurrent"
	numGoroutines := 100

	var wg sync.WaitGroup
	results := make(chan error, numGoroutines)

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := DefaultManager.SingleRun(key, 200*time.Millisecond, func() error {
				time.Sleep(50 * time.Millisecond)
				return nil
			})
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// 统计结果
	successCount := 0
	errorCount := 0

	for err := range results {
		if err == nil {
			successCount++
		} else if err == ErrRunning {
			errorCount++
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	// 由于并发执行，可能有多个goroutine同时开始，所以成功数量可能大于1
	// 但错误数量应该等于总数减去成功数量
	if successCount < 1 {
		t.Errorf("Expected at least 1 success, got %d", successCount)
	}

	if successCount+errorCount != numGoroutines {
		t.Errorf("Expected total results to equal %d, got %d", numGoroutines, successCount+errorCount)
	}
	fmt.Println("success count", successCount, "error", errorCount)
}

// TestSingleRun_RunCount 测试运行计数
func TestSingleRun_RunCount(t *testing.T) {
	key := "test_run_count"

	// 第一次运行
	info1, err := DefaultManager.SingleRun(key, NoWait, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if info1.RunCount != 1 {
		t.Errorf("Expected RunCount 1, got %d", info1.RunCount)
	}

	// 第二次运行
	info2, err := DefaultManager.SingleRun(key, NoWait, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if info2.RunCount != 2 {
		t.Errorf("Expected RunCount 2, got %d", info2.RunCount)
	}
}

// TestSingleRun_ErrorHandling 测试错误处理
func TestSingleRun_ErrorHandling(t *testing.T) {
	key := "test_error_handling"
	expectedError := errors.New("test error")

	_, err := DefaultManager.SingleRun(key, NoWait, func() error {
		return expectedError
	})

	if err != expectedError {
		t.Errorf("Expected %v, got %v", expectedError, err)
	}
}
