package requestPool

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	DefaultConcurrency = 10
	MaxRetryAttempts   = 3
	MaxConcurrency     = 200
	CooldownPeriod     = 1 * time.Second
	SuccessThreshold   = 5
	FailureThreshold   = 3
	MaxFailureCount    = 5
)

type Request struct {
	Method    string
	Path      string
	Body      interface{}
	Params    map[string][]string
	Headers   map[string]string
	Result    *resty.Response
	Error     error
	RetryNum  int64
	requestID string
	groupID   string
	errorNum  int64
	groupNum  int
	errorUrls map[string]error
}

func (t Request) ResponseString() string {
	return string(t.Result.Body())
}

type routines struct {
	id        string
	closeChan chan struct{}
	pool      *Pool
	point     *Point
	client    *resty.Client
}

type Point struct {
	mu             *sync.Mutex
	id             string
	pool           *Pool
	Url            string
	Headers        map[string]string
	routines       map[string]routines
	requestChan    chan Request
	requestNum     int64
	requestTime    time.Duration
	avgTime        time.Duration
	successNum     int64
	errorNum       int64
	successStreak  int
	failureStreak  int
	lastAdjustTime time.Time
	failureCount   int
	backoffTime    time.Time
	needClose      bool
}

func (p *Point) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.needClose = true
	close(p.requestChan)
}

type ReTryFunc func(ctx context.Context, point *Point, req Request, t *resty.Response) error

type Pool struct {
	mu          *sync.Mutex
	poolData    any
	requestChan chan Request
	notifyChan  map[string]chan struct{}
	pointManage map[string]*Point //只读不需要加锁
	resultMap   map[string]map[string]Request
	retryFunc   []ReTryFunc
	pointSort   []*Point
}

func (p *Pool) SetPoolData(data any) {
	p.poolData = data
}
func (p *Pool) SetRetryFunc(fs []ReTryFunc) {
	p.retryFunc = fs
}
func (p *Pool) AddRetryFunc(f ReTryFunc) {
	p.retryFunc = append(p.retryFunc, f)
}

type PointConfig struct {
	Url     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func NewPool(points []PointConfig) (*Pool, error) {
	if len(points) == 0 {
		return nil, errors.New("Pool is empty")
	}
	instance := &Pool{
		mu:          &sync.Mutex{},
		pointManage: make(map[string]*Point),
		requestChan: make(chan Request, 10000),
		notifyChan:  make(map[string]chan struct{}),
		resultMap:   make(map[string]map[string]Request),
	}
	for _, v := range points {
		pid := uuid.NewString()
		instance.pointManage[pid] = &Point{
			id:          pid,
			mu:          &sync.Mutex{},
			pool:        instance,
			Url:         v.Url,
			Headers:     v.Headers,
			routines:    make(map[string]routines),
			requestChan: make(chan Request, 10000),
		}
	}

	instance.run()
	coroutines.SafeGo(coroutines.NewContext("healthy check"), func(ctx context.Context) {
		instance.CheckStatus(ctx)
	})
	return instance, nil
}

func (p *Pool) UpdatePoints(configs []PointConfig) {
	if len(configs) == 0 {
		return
	}
	//updateendpoints(config)
	//k换成随机数
	//判断那些和现在的一样，就不管，没有的话，就新增，多了，就删除
	//设置暂停标记，如果有标记，就不接收全局和局部的请求，处理完剩下的（或者直接给全局队列，这样好些，免得不稳定节点一直占用队列又不返回），直接删除这个节点
	//发现如果不需要了，就在处理的时候，自己把自己关了

	newConfigs := map[string]PointConfig{}
	for _, v := range configs {
		newConfigs[v.Url] = v
	}

	oldConfigs := map[string]*Point{}
	for _, v := range p.pointManage {
		oldConfigs[v.Url] = v
	}

	for _, v := range newConfigs {
		point, ok := oldConfigs[v.Url]
		if ok {
			//编辑headers
			point.mu.Lock()
			point.Headers = v.Headers
			point.needClose = false
			point.mu.Unlock()
		} else {
			//新增
			p.mu.Lock()
			pid := uuid.NewString()
			p.pointManage[pid] = &Point{
				id:          pid,
				mu:          &sync.Mutex{},
				pool:        p,
				Url:         v.Url,
				Headers:     v.Headers,
				routines:    make(map[string]routines),
				requestChan: make(chan Request, 10000),
			}
			p.mu.Unlock()
			for i := 0; i < DefaultConcurrency; i++ {
				p.pointManage[pid].AddGoroutine()
			}
		}
	}
	for _, point := range oldConfigs {
		_, ok := newConfigs[point.Url]
		if !ok {
			point.Close()
			p.mu.Lock()
			delete(p.pointManage, point.id)
			p.mu.Unlock()
		}
	}
}

func (p *Pool) CheckStatus(ctx context.Context) {
	sortTicker := time.NewTicker(3 * time.Second)
	printTicker := time.NewTicker(1 * time.Minute)
	defer sortTicker.Stop()
	defer printTicker.Stop()

	lastFailure := make(map[string]int)

	for {
		select {
		case <-sortTicker.C:
			p.mu.Lock()
			p.pointSort = make([]*Point, 0, len(p.pointManage))
			for _, v := range p.pointManage {
				v.mu.Lock()
				if !v.needClose {
					p.pointSort = append(p.pointSort, v)
				}
				v.mu.Unlock()
			}
			sort.Slice(p.pointSort, func(i, j int) bool {
				scoreI := calculateScore(p.pointSort[i])
				scoreJ := calculateScore(p.pointSort[j])
				return scoreI > scoreJ
			})
			p.mu.Unlock()

		case <-printTicker.C:
			p.mu.Lock()
			noSuccess := true
			pointInfo := ""
			for _, info := range p.pointManage {
				msg := fmt.Sprintf("now: %s, url: %s,\n"+
					" Concurrency: %d, RequestNum: %d, SuccessNum: %d, ErrorNum: %d, AvgTime: %s, "+
					" PointChanLen: %d, SuccessStreak: %d, FailureStreak: %d, "+
					" LastAdjustTime: %s, FailureCount: %d, BackoffTime: %s",
					time.Now().Format("2006-01-02 15:04:05"),
					info.Url,
					len(info.routines),
					info.requestNum,
					info.successNum,
					info.errorNum,
					info.avgTime.String(),
					len(info.requestChan),
					info.successStreak,
					info.failureStreak,
					info.lastAdjustTime.Format("2006-01-02 15:04:05"),
					info.failureCount,
					info.backoffTime.Format("2006-01-02 15:04:05"))
				pointInfo += msg + "\n"

				if info.failureStreak < DefaultConcurrency {
					noSuccess = false
				} else if prev, exists := lastFailure[info.Url]; exists && info.failureStreak == prev {
					noSuccess = false
				}

				lastFailure[info.Url] = info.failureStreak
			}

			if noSuccess {
				e.SendMessage(ctx, errors.New("all url no success: "+pointInfo))
			}
			fmt.Println(pointInfo)
			p.mu.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

func calculateScore(point *Point) float64 {
	point.mu.Lock()
	defer point.mu.Unlock()

	var routinesScore, successRateScore, failureStreakPenalty float64
	if len(point.routines) > DefaultConcurrency {
		routinesScore = float64(len(point.routines)) / float64(MaxConcurrency)
	}

	if point.requestNum > 0 {
		successRateScore = float64(point.successNum) / float64(point.requestNum)
	}

	//如果连续错误次数>开启的协程数，排名急速下降
	failureStreakPenalty = float64(point.failureStreak) / float64(len(point.routines))
	allScore := routinesScore + successRateScore - failureStreakPenalty

	return allScore
}

func (p *Pool) run() {
	for _, point := range p.pointManage {
		for i := 0; i < DefaultConcurrency; i++ {
			point.AddGoroutine()
		}
	}
}

func (p *Point) AddGoroutine() {
	coroutines.SafeGo(coroutines.NewContext("AddGoroutine"), func(ctx context.Context) {
		p.startWorker(ctx)
	})
}

func (p *Point) startWorker(ctx context.Context) {
	point := p
	point.mu.Lock()
	gID := fmt.Sprintf("%d%s", time.Now().UnixMilli(), uuid.NewString())
	closeChan := make(chan struct{})
	pointRoutines := routines{
		id:        gID,
		closeChan: closeChan,
		pool:      p.pool,
		point:     point,
		client:    resty.New(),
	}
	point.routines[gID] = pointRoutines
	point.mu.Unlock()

	defer func() {
		point.mu.Lock()
		delete(point.routines, gID)
		point.mu.Unlock()
	}()

LOOP:
	for {
		//如果需要关闭，就关闭这个协程，就不接收全部队列的数据了
		if point.needClose {
			for req := range point.requestChan {
				p.pool.requestChan <- req
			}
			return
		}

		select {
		case <-closeChan:
			return
		default:
		}

		select {
		case req := <-point.requestChan:
			p.handleRequestWithBackoff(ctx, pointRoutines, req)
			continue LOOP // 处理完直接进入下一轮循环
		default:
		}
		select {
		case <-closeChan:
			return
		case req := <-point.requestChan:
			p.handleRequestWithBackoff(ctx, pointRoutines, req)
		case req := <-p.pool.requestChan:
			p.handleRequestWithBackoff(ctx, pointRoutines, req)
		}
	}
}

func (p *Point) handleRequestWithBackoff(ctx context.Context, pointRoutines routines, req Request) {
	if time.Now().Before(p.backoffTime) {
		pointRoutines.redirectRequest(req)
		for time.Now().Before(p.backoffTime) {
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		pointRoutines.handleRequest(ctx, req)

	}
}

func (r routines) handleRequest(ctx context.Context, req Request) {
	r.pool.mu.Lock()
	if _, ok := r.pool.resultMap[req.groupID]; !ok {
		r.pool.mu.Unlock()
		return //这里肯定就关闭了
	}
	r.pool.mu.Unlock()
	runInfo, result, err := r.DoRequest(ctx, req)
	//如果是业务的错误，那么发送到其他节点试试，节点要满足
	if err != nil {
		r.handleErrorRequest(req, err)
	} else {
		r.handleSuccessRequest(req, runInfo, result)
	}
}

func (r routines) handleSuccessRequest(req Request, runInfo RequestRunInfo, result *resty.Response) {
	point := r.point
	req.Result = result
	req.Error = nil

	point.mu.Lock()
	point.requestNum++
	point.successNum++
	point.requestTime += runInfo.ExecTime
	point.avgTime = point.requestTime / time.Duration(point.requestNum)
	point.successStreak++
	point.failureStreak = 0
	point.mu.Unlock()

	r.saveAndNotify(req)
	r.adjustGoroutine(true)
}

func (r routines) handleErrorRequest(req Request, err error) {
	point := r.point
	req.errorNum++
	req.Error = err
	req.errorUrls[point.id] = err

	retry := true
	if req.RetryNum > 0 && req.errorNum > req.RetryNum {
		retry = false
	}

	if req.errorNum > 20 && req.errorNum%20 == 0 {
		e.SendMessage(coroutines.NewContext("Error Request"),
			errors.New(fmt.Sprintf("ErrorNumBigErr:%d,url:%s ,path:%s,body:%v,lastError:%v", req.errorNum, r.point.Url, req.Path, req.Body, req.Error)))
	}

	point.mu.Lock()
	point.failureCount++
	if point.failureCount >= MaxFailureCount {
		point.backoffTime = time.Now().Add(1 * time.Second)
		point.failureCount = 0
	}
	point.requestNum++
	point.errorNum++
	point.failureStreak++
	point.successStreak = 0
	point.mu.Unlock()
	if retry {
		r.redirectRequest(req)
	} else {
		r.saveAndNotify(req)
	}
	r.adjustGoroutine(false)
}

func (r routines) adjustGoroutine(success bool) {
	point := r.point

	point.mu.Lock()
	if time.Since(point.lastAdjustTime) < CooldownPeriod {
		point.mu.Unlock()
		return
	}

	if success {
		if point.successStreak >= SuccessThreshold && len(point.routines) < MaxConcurrency {
			point.AddGoroutine()
			point.successStreak = 0
			point.lastAdjustTime = time.Now()
		}
	} else {
		if point.failureStreak >= FailureThreshold && len(point.routines) > 1 {
			delete(point.routines, r.id)
			safeCloseChan(r.closeChan) //这里关闭了，但是当前协程还没结束，要下一轮才知道自己关闭了，所以在解锁之后，这个当前len还是没有减小的
			point.failureStreak = 0
			point.lastAdjustTime = time.Now()
		}
	}
	point.mu.Unlock()
}

func (r routines) redirectRequest(req Request) {
	curPoint := r.point
	r.pool.mu.Lock()
	for _, p := range r.pool.pointSort {
		if p.id != curPoint.id && req.errorUrls[p.id] == nil && len(p.requestChan) < len(p.routines) { //这里还是不能发给自己，就算是第一也有可能突然挂了，然后自己又转发给自己
			select {
			case p.requestChan <- req:
				r.pool.mu.Unlock()
				return
			default:
				// 如果通道已满，继续尝试下一个逻辑
			}
		}
	}

	//全部失败了
	r.pool.mu.Unlock()
	// 清空错误记录并重新尝试分配
	req.errorUrls = make(map[string]error)
	// 随机分配到一个其他 point
	//for pointIndex, otherPoint := range r.pool.pointManage {
	//	if pointIndex != curPoint.id && otherPoint.backoffTime.Before(time.Now()) {
	//		select {
	//		case otherPoint.requestChan <- req:
	//			return
	//		default:
	//			// 如果通道已满，继续尝试下一个 point
	//		}
	//	}
	//}

	// 如果所有 point 都无法处理请求，则重新放回自身通道
	curPoint.requestChan <- req
}

func (r routines) saveAndNotify(req Request) {
	r.pool.mu.Lock()
	if _, ok := r.pool.resultMap[req.groupID]; !ok {
		r.pool.mu.Unlock()
		return //这里肯定就关闭了
	}
	r.pool.resultMap[req.groupID][req.requestID] = req
	if notifyChan, ok := r.pool.notifyChan[req.groupID]; ok && len(r.pool.resultMap[req.groupID]) == req.groupNum {
		safeCloseChan(notifyChan)
	}
	r.pool.mu.Unlock()
}

// safeCloseChan 使用泛型安全地关闭任意类型的通道
func safeCloseChan[T any](ch chan T) {
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

type RequestRunInfo struct {
	ExecTime time.Duration
}

// DoRequest 执行请求
func (r routines) DoRequest(ctx context.Context, req Request) (runInfo RequestRunInfo, result *resty.Response, err error) {
	point := r.point

	method := req.Method
	params := req.Params
	reqHeaders := req.Headers
	headers := r.point.Headers
	for k, v := range reqHeaders {
		headers[k] = v
	}
	body := req.Body

	client := r.client

	// 动态设置超时时间
	minTimeout := 5 * time.Second
	client.SetTimeout(60 * time.Second)

	point.mu.Lock()
	url := point.Url + req.Path

	lastErrIsTimeout := false
	var netErr net.Error
	if errors.As(req.Error, &netErr) && netErr.Timeout() {
		lastErrIsTimeout = true
	}
	if !lastErrIsTimeout && point.avgTime > 0 && point.successNum > 0 {
		client.SetTimeout(point.avgTime + minTimeout)
	}
	point.mu.Unlock()

	httpReq := client.R()
	httpReq.SetQueryParamsFromValues(params)
	httpReq.SetHeaders(headers)
	if body != nil {
		httpReq.SetBody(body)
	}
	httpResponse, err := executeRequest(httpReq, method, url)
	if err != nil {
		return runInfo, result, errors.Wrap(err, fmt.Sprintf("url:%s ,body:%v", r.point.Url, req.Body))
	}
	httpResponse.RawResponse.Body.Close()
	err = CheckJson(ctx, httpResponse)
	if err != nil {
		return runInfo, result, errors.Wrap(err, fmt.Sprintf("url:%s ,body:%v,response:%s", r.point.Url, req.Body, httpResponse.String()))
	}

	for _, f := range r.pool.retryFunc {
		point.mu.Lock()
		err = f(ctx, point, req, httpResponse)
		point.mu.Unlock()
		if err != nil {
			return runInfo, result, e.Err(err)
		}
	}

	runInfo.ExecTime = httpResponse.Time()
	result = httpResponse
	return
}

// CheckJson 保证http请求正常，状态值正常，是个json，这里不正常就重试,业务的检测放在外面去做
func CheckJson(ctx context.Context, t *resty.Response) error {
	if !t.IsSuccess() {
		return errors.New(fmt.Sprintf("Response Status not success: %v", t.StatusCode()))
	}
	isJson := utils.IsJson(t.Body())
	if !isJson {
		return errors.New("Response is not json")
	}
	return nil
}

func (p *Pool) Request(ctx context.Context, req Request) (res Request, err error) {
	if p == nil {
		return res, errors.New("pool is nil")
	}
	requestID := uuid.NewString()
	requests := map[string]Request{
		requestID: req,
	}

	results, err := p.RequestMulti(ctx, requests)
	if err != nil {
		return res, err
	}

	result, exists := results[requestID]
	if !exists {
		return res, fmt.Errorf("request result not found for request ID: %s", requestID)
	}

	return result, result.Error
}
func (p *Pool) RequestMulti(ctx context.Context, requests map[string]Request) (map[string]Request, error) {
	if p == nil {
		return nil, errors.New("pool is nil")
	}
	if len(requests) == 0 {
		return nil, nil
	}
	p.mu.Lock()
	groupID := uuid.NewString()
	p.resultMap[groupID] = make(map[string]Request)
	notifyChan := make(chan struct{})
	p.notifyChan[groupID] = notifyChan
	msgLen := len(requests)
	rpcLen := len(p.pointManage)
	var msgList []Request
	for id, v := range requests {
		msg := v
		msg.requestID = id
		msg.groupID = groupID
		msg.groupNum = msgLen
		msg.errorUrls = make(map[string]error)
		if msg.RetryNum == 0 {
			msg.RetryNum = int64(rpcLen) * MaxRetryAttempts
		}
		msgList = append(msgList, msg)
	}
	p.mu.Unlock()

	//清理资源
	defer func() {

		p.mu.Lock()
		if notify, ok := p.notifyChan[groupID]; ok {
			safeCloseChan(notify)
			delete(p.notifyChan, groupID)
		}
		delete(p.resultMap, groupID)
		p.mu.Unlock()
	}()

	// 将请求放入全局通道
	for _, msg := range msgList {
		p.requestChan <- msg
	}

	select {
	case <-notifyChan:
		p.mu.Lock()
		response := p.resultMap[groupID]
		p.mu.Unlock()
		for _, v := range response {
			if v.Error != nil {
				return response, v.Error
			}
		}
		return response, nil
	case <-ctx.Done():
		return nil, e.Err(ctx.Err())
	}
}
func executeRequest(req *resty.Request, method, url string) (*resty.Response, error) {
	switch method {
	case http.MethodGet:
		return req.Get(url)
	case http.MethodPost:
		return req.Post(url)
	case http.MethodPut:
		return req.Put(url)
	case http.MethodDelete:
		return req.Delete(url)
	default:
		return req.Get(url)
	}
}
