package requestPool

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/utils"
	"net/http"
	"net/http/pprof"
	"sync"
	"testing"
	"time"
)

func StartHTTPDebuger() {
	pprofHandler := http.NewServeMux()
	pprofHandler.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	server := &http.Server{Addr: ":6789", Handler: pprofHandler}
	go server.ListenAndServe()
}

func TestRequestPool(t *testing.T) {

	//让pprof服务运行起来
	go StartHTTPDebuger()
	//detector := deadlock.NewDetector()
	//detector.Start()
	//RedirectStderr()
	// 初始化示例 RpcPoints
	rpcPoints := []PointConfig{
		{Url: "https://rpc.ankr.com/eth/5d7870ed5d1da8fcd8a8336eaa354d2612077fd31eed7f2570864215af234cfe"},
		{Url: "https://eth-mainnet.public.blastapi.io"},
		{Url: "https://mainnet.infura.io/v3/fc3fb27575534640a27b437a7e34a161"},
		{Url: "https://eth.llamarpc.com"},
		{Url: "https://rpc.mevblocker.io"},
		{Url: "https://eth.meowrpc.com"},
		{Url: "https://rpc.payload.de"},
		{Url: "https://eth.drpc.org"},
		{Url: "https://ethereum-rpc.publicnode.com"},
		{Url: "https://1rpc.io/eth"},
		{Url: "https://ddsfasefawerwerasdrawseraserawerdd.com/eth"},
	}

	// 创建 Points 实例
	pool, err := NewPool(rpcPoints)
	if err != nil {
		panic(err)
	}

	// 创建上下文
	ctx := context.Background()

	// 创建请求（以 eth_blockNumber 方法为例）
	requests := make(map[string]Request)
	for i := 0; i < 10; i++ {
		requests[utils.AnyToString(i)] = Request{
			Method: http.MethodPost,
			Body: map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "eth_blockNumber",
				"params":  []interface{}{},
			},
		}
	}

	var count = 0
	var success = 0
	var lock = new(sync.Mutex)
	var errlist []Request
	// 加入请求并等待结果
	startTime := time.Now().Unix()
	wg := &sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		coroutines.SafeGo(ctx, func(ctx context.Context) {
			defer wg.Done()
			results, err := pool.RequestMulti(ctx, requests)
			if err != nil {
				fmt.Println(err, 111111111111111)
			}
			//for method, result := range results {
			//	fmt.Printf("Method: %s, Result: %s\n", method, result.Result)
			//}

			lock.Lock()
			count += len(requests)
			for _, v := range results {
				if v.Result.String() != "" {
					success++
				} else {
					errlist = append(errlist, v)
				}
			}
			lock.Unlock()
		})

	}

	wg.Wait()
	end := time.Now().Unix()
	fmt.Println("time:", end-startTime, "all", count, "success", success)
	fmt.Println("errlist ", utils.Json(errlist))
	fmt.Println("over")
}
