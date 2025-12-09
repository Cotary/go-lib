package utils

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestSingleRun_NoWait 测试不等待场景
func TestSingleRun_NoWait(t *testing.T) {
	key := "test_no_wait"
	manager := NewManager()
	ctx := context.Background()

	// 启动一个长时间运行的任务
	done := make(chan bool)
	go func() {
		_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()

	// 启动一个长时间运行的任务
	start := time.Now()
	done := make(chan bool)
	go func() {
		_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	_, err := manager.SingleRun(ctx, key, 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	fmt.Println("first task end", time.Since(start).Seconds())

	if err != ErrRunning {
		t.Errorf("Expected ErrRunning, got %v", err)
	}
	fmt.Println("second task started", time.Since(start).Seconds())
	_, err = manager.SingleRun(ctx, key, 5*time.Second, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()

	// 启动一个任务
	start := time.Now()
	done := make(chan bool)
	go func() {
		_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	_, err := manager.SingleRun(ctx, key, MustWait, func(ctx context.Context) error {
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

// TestSingleRun_Count_MustWait 测试并发场景
func TestSingleRun_Count_MustWait(t *testing.T) {
	key := "test_count_mustWait"
	manager := NewManager()
	ctx := context.Background()
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = manager.SingleRun(ctx, key, MustWait, func(ctx context.Context) error {
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

// TestSingleRun_Count_Wait 测试并发场景
func TestSingleRun_Count_Wait(t *testing.T) {
	key := "test_count_wait"
	manager := NewManager()
	ctx := context.Background()
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = manager.SingleRun(ctx, key, 5*time.Second, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()
	num := 0
	numGoroutines := 1000

	var wg sync.WaitGroup

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()
	numGoroutines := 100

	var wg sync.WaitGroup
	results := make(chan error, numGoroutines)

	// 启动多个goroutine同时尝试执行
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := manager.SingleRun(ctx, key, 200*time.Millisecond, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()

	// 第一次运行
	info1, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	info2, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
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
	manager := NewManager()
	ctx := context.Background()
	expectedError := errors.New("test error")

	_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		return expectedError
	})

	if err != expectedError {
		t.Errorf("Expected %v, got %v", expectedError, err)
	}
}

// ======================== 嵌套锁测试 ========================

// TestSingleRun_NestedSameKey 测试嵌套调用同一个 key（核心测试用例）
func TestSingleRun_NestedSameKey(t *testing.T) {
	key := "test_nested_same_key"
	manager := NewManager()
	callOrder := []string{}

	ctx := context.Background()
	_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		callOrder = append(callOrder, "outer_start")

		// 嵌套调用同一个 key，应该直接执行，不会死锁
		_, innerErr := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
			callOrder = append(callOrder, "inner")
			return nil
		})

		if innerErr != nil {
			t.Errorf("Nested call should succeed, got error: %v", innerErr)
		}

		callOrder = append(callOrder, "outer_end")
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 验证调用顺序
	expected := []string{"outer_start", "inner", "outer_end"}
	if len(callOrder) != len(expected) {
		t.Errorf("Expected call order %v, got %v", expected, callOrder)
	}
	for i, v := range expected {
		if callOrder[i] != v {
			t.Errorf("Expected call order %v, got %v", expected, callOrder)
			break
		}
	}
	fmt.Println("Call order:", callOrder)
}

// TestSingleRun_DeepNested 测试多层嵌套
func TestSingleRun_DeepNested(t *testing.T) {
	key := "test_deep_nested"
	manager := NewManager()
	depth := 0
	maxDepth := 5

	ctx := context.Background()
	var nestedCall func(ctx context.Context) error
	nestedCall = func(ctx context.Context) error {
		depth++
		currentDepth := depth
		fmt.Printf("Entering depth %d\n", currentDepth)

		if currentDepth < maxDepth {
			_, err := manager.SingleRun(ctx, key, NoWait, nestedCall)
			if err != nil {
				return err
			}
		}

		fmt.Printf("Exiting depth %d\n", currentDepth)
		return nil
	}

	_, err := manager.SingleRun(ctx, key, NoWait, nestedCall)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if depth != maxDepth {
		t.Errorf("Expected depth %d, got %d", maxDepth, depth)
	}
}

// TestSingleRun_NestedDifferentKeys 测试嵌套调用不同的 key
func TestSingleRun_NestedDifferentKeys(t *testing.T) {
	key1 := "test_nested_key1"
	key2 := "test_nested_key2"
	manager := NewManager()
	callOrder := []string{}

	ctx := context.Background()
	_, err := manager.SingleRun(ctx, key1, NoWait, func(ctx context.Context) error {
		callOrder = append(callOrder, "key1_start")

		// 嵌套调用不同的 key，也应该正常执行
		_, innerErr := manager.SingleRun(ctx, key2, NoWait, func(ctx context.Context) error {
			callOrder = append(callOrder, "key2")
			return nil
		})

		if innerErr != nil {
			t.Errorf("Nested call with different key should succeed, got error: %v", innerErr)
		}

		callOrder = append(callOrder, "key1_end")
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := []string{"key1_start", "key2", "key1_end"}
	if len(callOrder) != len(expected) {
		t.Errorf("Expected call order %v, got %v", expected, callOrder)
	}
	fmt.Println("Call order:", callOrder)
}

// TestSingleRun_NilContext 测试 nil context
func TestSingleRun_NilContext(t *testing.T) {
	key := "test_nil_context"
	manager := NewManager()

	//nolint:staticcheck // 故意测试 nil context 的处理
	_, err := manager.SingleRun(nil, key, NoWait, func(ctx context.Context) error {
		if ctx == nil {
			t.Error("Context should not be nil inside function")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestSingleRun_NestedWithWait 测试嵌套调用时 waitTime 参数被忽略
func TestSingleRun_NestedWithWait(t *testing.T) {
	key := "test_nested_wait"
	manager := NewManager()

	ctx := context.Background()
	start := time.Now()

	_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		// 嵌套调用，即使设置了等待时间，也应该立即执行（因为是嵌套调用）
		_, innerErr := manager.SingleRun(ctx, key, 5*time.Second, func(ctx context.Context) error {
			return nil
		})
		return innerErr
	})

	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 嵌套调用应该立即执行，不需要等待 5 秒
	if elapsed > 1*time.Second {
		t.Errorf("Nested call took too long: %v", elapsed)
	}
	fmt.Println("Elapsed:", elapsed)
}

// TestSingleRun_ErrorPropagation 测试嵌套调用的错误传播
func TestSingleRun_ErrorPropagation(t *testing.T) {
	key := "test_error_propagation"
	manager := NewManager()
	expectedError := errors.New("inner error")

	ctx := context.Background()
	_, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		_, innerErr := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
			return expectedError
		})
		return innerErr
	})

	if err != expectedError {
		t.Errorf("Expected %v, got %v", expectedError, err)
	}
}

// TestSingleRun_NestedRunCount 测试嵌套调用时 RunCount 的行为
func TestSingleRun_NestedRunCount(t *testing.T) {
	key := "test_nested_run_count"
	manager := NewManager()

	ctx := context.Background()

	// 第一次调用
	info1, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if info1.RunCount != 1 {
		t.Errorf("Expected RunCount 1, got %d", info1.RunCount)
	}

	// 第二次调用（带嵌套）
	info2, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		// 嵌套调用不应该增加 RunCount
		_, _ = manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
			return nil
		})
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if info2.RunCount != 2 {
		t.Errorf("Expected RunCount 2, got %d (nested calls should not increase RunCount)", info2.RunCount)
	}

	// 第三次调用
	info3, err := manager.SingleRun(ctx, key, NoWait, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if info3.RunCount != 3 {
		t.Errorf("Expected RunCount 3, got %d", info3.RunCount)
	}
}

// TestSingleRun_MixedNestedKeys 测试混合嵌套多个 key
func TestSingleRun_MixedNestedKeys(t *testing.T) {
	manager := NewManager()
	callOrder := []string{}

	ctx := context.Background()
	_, err := manager.SingleRun(ctx, "A", NoWait, func(ctx context.Context) error {
		callOrder = append(callOrder, "A_start")

		_, _ = manager.SingleRun(ctx, "B", NoWait, func(ctx context.Context) error {
			callOrder = append(callOrder, "B_start")

			// 嵌套调用 A（应该直接执行，因为 A 已在调用链中）
			_, _ = manager.SingleRun(ctx, "A", NoWait, func(ctx context.Context) error {
				callOrder = append(callOrder, "A_nested")
				return nil
			})

			callOrder = append(callOrder, "B_end")
			return nil
		})

		callOrder = append(callOrder, "A_end")
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := []string{"A_start", "B_start", "A_nested", "B_end", "A_end"}
	if len(callOrder) != len(expected) {
		t.Errorf("Expected call order %v, got %v", expected, callOrder)
	}
	for i, v := range expected {
		if i >= len(callOrder) || callOrder[i] != v {
			t.Errorf("Expected call order %v, got %v", expected, callOrder)
			break
		}
	}
	fmt.Println("Call order:", callOrder)
}
