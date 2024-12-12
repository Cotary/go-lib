package requestPool

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/httpServer"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/pkg/errors"
	"net/http"
	"sync"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	e "github.com/Cotary/go-lib/err"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// todo 怎么动态调节这个batch
const (
	BatchNum = 100
)

// todo 清理数据，ctx 超时
type BatchRequest struct {
	Request
	requestID    string
	groupID      string
	ResponseBody string
}

func (b BatchRequest) String() string {
	return b.ResponseBody
}

type BatchPool struct {
	mu                sync.Mutex
	pool              *Pool
	requestChan       chan BatchRequest
	requestSingleChan chan BatchRequest
	notifyChan        map[string]chan struct{}
	resultCount       map[string]int
	resultMap         map[string]map[string]BatchRequest
}

func NewBatchRequest(pool *Pool) *BatchPool {
	pool.AddRetryFunc(BatchRetryCheck)
	instance := &BatchPool{
		pool:              pool,
		requestChan:       make(chan BatchRequest, 10000),
		requestSingleChan: make(chan BatchRequest, 10000),
		notifyChan:        make(map[string]chan struct{}),
		resultCount:       make(map[string]int),
		resultMap:         make(map[string]map[string]BatchRequest),
	}
	go instance.run()
	return instance
}

func (p *BatchPool) run() {
	for i := 0; i < 50; i++ {
		coroutines.SafeGo(coroutines.NewContext("BatchWorker"), func(ctx context.Context) {
			p.batchWorker(ctx)
		})
	}
	for i := 0; i < 100; i++ {
		coroutines.SafeGo(coroutines.NewContext("SingleWorker"), func(ctx context.Context) {
			p.singleWorker(ctx)
		})
	}
}

func (p *BatchPool) singleWorker(ctx context.Context) {
	for {
		select {
		case req := <-p.requestSingleChan:
			result, _ := p.pool.Request(ctx, req.Request)
			p.notifyCompletion(req, result.Result.String())
		}
	}
}

func (p *BatchPool) batchWorker(ctx context.Context) {
	for {
		requests := p.takeBatchFromChannel(p.requestChan, BatchNum)
		if len(requests) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		fmt.Println("batch", len(requests))

		var mergeBody []interface{}
		for _, req := range requests {
			mergeBody = append(mergeBody, req.Request.Body)
		}

		req := Request{
			Method:   http.MethodPost,
			Body:     mergeBody,
			RetryNum: -1,
		}

		//todo 设置 3 分钟，如果3分钟还没有成功，那么应该是内容太大了,减小批量请求数
		//timeoutCtx, cancel := context.WithTimeout(ctx, 3*60*time.Second)
		//defer cancel()

		result, err := p.pool.Request(ctx, req)
		if err != nil {
			e.SendMessage(ctx, err)
			fmt.Println("Request error:", err)
			continue
		}

		gj := gjson.Parse(result.Result.String()) // TODO: 有的节点不支持批量，有的节点支持

		if gj.IsArray() {
			fmt.Println("IsArray")
			for key, item := range gj.Array() {
				if item.Get("error").Exists() && !item.Get("result").Exists() {
					fmt.Println("error", item.String())
					p.requestSingleChan <- requests[key]
				} else {
					fmt.Println("success", item.String())
					p.notifyCompletion(requests[key], item.String())
				}
			}
		} else {
			// 不支持批量，看看打到别的节点，如果节点都不支持，就单个请求
			fmt.Println("not IsArray", len(result.errorUrls))
			fmt.Println("llllll", result.Result.Request.URL, gj.String())
			for _, item := range requests {
				p.requestSingleChan <- item
			}

		}
	}
}

func (p *BatchPool) takeBatchFromChannel(ch chan BatchRequest, batchSize int) []BatchRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	var batch []BatchRequest
	for i := 0; i < batchSize; i++ {
		select {
		case req := <-ch:
			batch = append(batch, req)
		default:
			return batch
		}
	}
	return batch
}

func (p *BatchPool) BatchRequestMulti(ctx context.Context, requests map[string]Request) (map[string]BatchRequest, error) {
	groupID := uuid.NewString()

	p.mu.Lock()
	notifyChan := make(chan struct{})
	p.notifyChan[groupID] = notifyChan
	p.resultMap[groupID] = make(map[string]BatchRequest)
	p.resultCount[groupID] = len(requests)
	p.mu.Unlock()

	for id, req := range requests {
		batchReq := BatchRequest{
			requestID: id,
			groupID:   groupID,
			Request:   req,
		}
		p.requestChan <- batchReq
	}

	// 清理资源
	defer func() {
		p.mu.Lock()
		//close(p.notifyChan[groupID])
		if notify, ok := p.notifyChan[groupID]; ok {
			SafeCloseChan(notify)
			delete(p.notifyChan, groupID)
		}
		delete(p.resultMap, groupID)
		delete(p.resultCount, groupID)
		p.mu.Unlock()
	}()

	select {
	case <-notifyChan:
		p.mu.Lock()
		results := make(map[string]BatchRequest)
		for id, batchReq := range p.resultMap[groupID] {
			results[id] = batchReq
		}
		p.mu.Unlock()
		return results, nil
	case <-ctx.Done():
		return nil, e.Err(ctx.Err())
	}
}

func (p *BatchPool) notifyCompletion(req BatchRequest, responseBody string) {
	groupID := req.groupID
	requestID := req.requestID

	p.mu.Lock()
	req.ResponseBody = responseBody
	p.resultMap[groupID][requestID] = req
	if len(p.resultMap[groupID]) == p.resultCount[groupID] {
		if notifyChan, ok := p.notifyChan[groupID]; ok {
			close(notifyChan)
		}
	}
	p.mu.Unlock()
}

// SafeCloseChan 使用泛型安全地关闭任意类型的通道
func SafeCloseChan[T any](ch chan T) {
	select {
	case _, open := <-ch:
		if !open {
			return
		}
		close(ch)
	default:
		close(ch)
	}
}

func BatchRetryCheck(ctx context.Context, point *Point, req Request, t *httpServer.RestyResult) error {
	reqStr, err := utils.ToString(req.Body)
	if err != nil {
		return nil //如果转换失败，那么就不重试
	}
	//resStr, err := utils.ToString(t.Response.Body())
	//fmt.Println("aaaaaa", resStr)
	//if err != nil {
	//	return nil //如果转换失败，那么就不重试
	//}
	resStr := string(t.Response.Body())

	reqBody := gjson.Parse(reqStr)
	if reqBody.IsArray() {
		resBody := gjson.Parse(resStr)
		if !resBody.IsArray() {
			if len(req.errorUrls) < len(point.pool.pointManage) { //都重试一次
				fmt.Println("batch  ddd", len(req.errorUrls), len(point.pool.pointManage), t.Request.URL, resStr)
				return errors.New("point not support batch")
			}
		}
	}
	return nil
}
