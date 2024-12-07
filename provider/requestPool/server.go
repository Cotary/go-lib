package requestPool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/httpServer"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// todo 当前情况只考虑了节点当前的情况，比如在使用过程中，节点从第一变成了最后等情况
const (
	DefaultConcurrency = 50
	MaxRetryAttempts   = 3
	MaxConcurrency     = 200
	CooldownPeriod     = 1 * time.Second
	SuccessThreshold   = 5
	FailureThreshold   = 3
	MaxFailureCount    = 5
)

type Request struct {
	//todo 增加优先级概率，如果是高优先级的，就先发到高优先级的节点
	mu        *sync.Mutex
	Method    string
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
	errorUrls map[int]error
}

type routines struct {
	id        int
	closeChan chan struct{}
	pool      *Pool
	point     *Point
}

type Point struct {
	mu             *sync.Mutex
	id             int
	pool           *Pool
	Url            string
	Headers        map[string]string
	routines       map[int]routines
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
}

type ReTryFunc func(ctx context.Context, point *Point, req Request, t *httpServer.RestyResult) error

type Pool struct {
	mu          *sync.Mutex
	requestChan chan Request
	notifyChan  map[string]chan struct{}
	pointManage map[int]*Point //只读不需要加锁
	resultMap   map[string]map[string]Request
	retryFunc   []ReTryFunc
	pointSort   []*Point
}

func (p *Pool) SetRetryFunc(fs []ReTryFunc) {
	p.retryFunc = fs
}
func (p *Pool) AddRetryFunc(f ReTryFunc) {
	p.retryFunc = append(p.retryFunc, f)
}

type PointConfig struct {
	Url     string
	Headers map[string]string
}

func NewPool(points []PointConfig) (*Pool, error) {
	if len(points) == 0 {
		return nil, errors.New("Pool is empty")
	}
	instance := &Pool{
		mu:          new(sync.Mutex),
		pointManage: make(map[int]*Point),
		requestChan: make(chan Request, 10000),
		notifyChan:  make(map[string]chan struct{}),
		resultMap:   make(map[string]map[string]Request),
	}
	for k, v := range points {
		instance.pointManage[k] = &Point{
			id:          k,
			mu:          new(sync.Mutex),
			pool:        instance,
			Url:         v.Url,
			Headers:     v.Headers,
			routines:    make(map[int]routines),
			requestChan: make(chan Request, 10000),
		}
	}

	instance.run()
	coroutines.SafeGo(coroutines.NewContext("healthy check"), func(ctx context.Context) {
		instance.CheckStatus(ctx)
	})
	return instance, nil
}

func (p *Pool) CheckStatus(ctx context.Context) {
	for {
		time.Sleep(3 * time.Second)
		p.mu.Lock()

		pointInfo := ""

		for _, info := range p.pointManage {
			msg := fmt.Sprintf("now: %s,"+""+
				"url: %s,"+
				" Concurrency: %d,"+
				" RequestNum: %d,"+
				" SuccessNum: %d,"+
				" ErrorNum: %d,"+
				" AvgTime: %s,"+
				" PointChanLen: %d,"+
				" SuccessStreak: %d,"+
				" FailureStreak: %d,"+
				" LastAdjustTime: %s,"+
				" FailureCount: %d,"+
				" BackoffTime: %s",
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

		}

		//这里打分
		p.pointSort = make([]*Point, 0, len(p.pointManage))
		for _, v := range p.pointManage {
			p.pointSort = append(p.pointSort, v)
		}
		sort.Slice(p.pointSort, func(i, j int) bool {
			return len(p.pointSort[i].routines) > len(p.pointSort[j].routines)
		})
		fmt.Println(pointInfo)
		p.mu.Unlock()
	}
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
	gID := len(point.routines)
	closeChan := make(chan struct{})
	pointRoutines := routines{
		id:        gID,
		closeChan: closeChan,
		pool:      p.pool,
		point:     point,
	}
	point.routines[gID] = pointRoutines
	point.mu.Unlock()

	defer func() {
		point.mu.Lock()
		delete(point.routines, gID)
		point.mu.Unlock()
	}()

	for {
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
	fmt.Println(r.point.Url, runInfo.ExecTime.String(), err)
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
	retry := req.RetryNum > 0 && req.errorNum <= req.RetryNum

	if req.errorNum > 20 && req.errorNum%100 == 0 {
		e.SendMessage(coroutines.NewContext("Error Request"),
			errors.New(fmt.Sprintf("ErrorNumBigErr:%d,method:%s,params:%v,lastError:%v", req.errorNum, req.Method, req.Params, req.Error)))
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
			coroutines.SafeCloseChan(r.closeChan)
			point.failureStreak = 0
			point.lastAdjustTime = time.Now()
		}
	}
	point.mu.Unlock()
}

func (r routines) redirectRequest2(req Request) {
	point := r.point
	// 找出协程数最大的 point
	var maxRoutinesPoint *Point
	for _, otherPoint := range r.pool.pointManage {
		if otherPoint.id != point.id {
			if maxRoutinesPoint == nil || len(otherPoint.routines) > len(maxRoutinesPoint.routines) {
				maxRoutinesPoint = otherPoint
			}
		}
	}

	// 将请求分配到协程数最大的 point
	if maxRoutinesPoint != nil && len(maxRoutinesPoint.routines) > DefaultConcurrency {
		select {
		case maxRoutinesPoint.requestChan <- req:
			return
		default:
			// 如果通道已满，继续尝试下一个逻辑
		}
	}

	// 尝试将请求发送到其他没有错误记录的 point
	for pointIndex, otherPoint := range r.pool.pointManage {
		if pointIndex != point.id && req.errorUrls[pointIndex] == nil {
			select {
			case otherPoint.requestChan <- req:
				return
			default:
				// 如果通道已满，继续尝试下一个 point
			}
		}
	}

	// 清空错误记录并重新尝试分配
	req.errorUrls = make(map[int]error)

	// 随机分配到一个其他 point
	for pointIndex, otherPoint := range r.pool.pointManage {
		if pointIndex != point.id {
			select {
			case otherPoint.requestChan <- req:
				return
			default:
				// 如果通道已满，继续尝试下一个 point
			}
		}
	}

	// 如果所有 point 都无法处理请求，则重新放回自身通道
	point.requestChan <- req
}

func (r routines) redirectRequest(req Request) {

	r.pool.mu.Lock()
	for _, point := range r.pool.pointSort {
		select {
		case point.requestChan <- req:
			r.pool.mu.Unlock()
			return
		default:
			// 如果通道已满，继续尝试下一个逻辑
		}
	}
	r.pool.mu.Unlock()

	point := r.point
	// 尝试将请求发送到其他没有错误记录的 point
	for pointIndex, otherPoint := range r.pool.pointManage {
		if pointIndex != point.id && req.errorUrls[pointIndex] == nil {
			select {
			case otherPoint.requestChan <- req:
				return
			default:
				// 如果通道已满，继续尝试下一个 point
			}
		}
	}

	// 清空错误记录并重新尝试分配
	req.errorUrls = make(map[int]error)

	// 随机分配到一个其他 point
	for pointIndex, otherPoint := range r.pool.pointManage {
		if pointIndex != point.id {
			select {
			case otherPoint.requestChan <- req:
				return
			default:
				// 如果通道已满，继续尝试下一个 point
			}
		}
	}

	// 如果所有 point 都无法处理请求，则重新放回自身通道
	point.requestChan <- req
}

func (r routines) saveAndNotify(req Request) {
	r.pool.mu.Lock()
	if _, ok := r.pool.resultMap[req.groupID]; !ok {
		r.pool.mu.Unlock()
		return //这里肯定就关闭了
	}
	r.pool.resultMap[req.groupID][req.requestID] = req
	if notifyChan, ok := r.pool.notifyChan[req.groupID]; ok && len(r.pool.resultMap[req.groupID]) == req.groupNum {
		coroutines.SafeCloseChan(notifyChan)
	}
	r.pool.mu.Unlock()
}

type RequestRunInfo struct {
	ExecTime time.Duration
}

// DoRequest TODO 之后支持不同的client request
func (r routines) DoRequest(ctx context.Context, req Request) (runInfo RequestRunInfo, result *resty.Response, err error) {
	point := r.point

	method := req.Method
	params := req.Params
	headers := req.Headers
	body := req.Body

	client := httpServer.NewHttpClient()
	// 动态设置超时时间

	minTimeout := 3 * time.Second
	client.SetTimeout(5 * time.Second)

	point.mu.Lock()
	url := point.Url
	if point.avgTime > 0 && point.successNum > 0 {
		client.SetTimeout(point.avgTime + minTimeout)
	}
	point.mu.Unlock()

	client.NoKeepLog()
	realStartTime := time.Now()
	httpResponse := client.HttpRequest(ctx, method, url, params, body, headers)
	if httpResponse.Error != nil {
		return runInfo, result, e.Err(httpResponse.Error)
	}
	realEndTime := time.Now()

	err = CheckJson(ctx, httpResponse) //todo 这里灵活检测
	if err != nil {
		return runInfo, result, e.Err(err)
	}

	for _, f := range r.pool.retryFunc {
		point.mu.Lock()
		err = f(ctx, point, req, httpResponse)
		point.mu.Unlock()
		if err != nil {
			return runInfo, result, e.Err(err)
		}
	}

	runInfo.ExecTime = httpResponse.Response.Time()
	fmt.Println("dddd", realEndTime.Sub(realStartTime).String(), runInfo.ExecTime.String())
	if realEndTime.Sub(realStartTime) > 5*time.Second {
		fmt.Println("eeee", realEndTime.Sub(realStartTime).String(), runInfo.ExecTime.String())

	}
	result = httpResponse.Response
	return
}

// CheckJson 保证http请求正常，状态值正常，是个json，这里不正常就重试,业务的检测放在外面去做
func CheckJson(ctx context.Context, t *httpServer.RestyResult) error {
	if t.Error != nil {
		return e.Err(t.Error)
	}
	if !t.IsSuccess() {
		return errors.New(fmt.Sprintf("Response Status not success: %v", t.Response.StatusCode()))
	}
	isJson := utils.IsJson(t.String())
	if !isJson {
		return errors.New("Response is not json: " + t.String())
	}
	return nil
}

func (p *Pool) RequestMulti(ctx context.Context, requests map[string]Request) (map[string]Request, error) {
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
		msg.errorUrls = make(map[int]error)
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
			coroutines.SafeCloseChan(notify)
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
