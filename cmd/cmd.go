package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	utils2 "github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/robfig/cron/v3"
	"time"
)

func Start(handlers []Handler) *cron.Cron {
	// 任务计划
	c := cron.New(cron.WithSeconds())
	cmdHandlers := handlers
	for i, v := range cmdHandlers {
		newHandler := v.New()
		_, err := c.AddFunc(newHandler.Spec(), func() {
			cmdHandle(i, newHandler)
		})
		if err != nil {
			panic(err)
		}
	}
	c.Start()
	fmt.Println("cron start")

	return c
}

func cmdHandle(index int, handle Handler) {
	ctx := coroutines.NewContext("CRON")
	coroutines.SafeFunc(ctx, func(ctx context.Context) {
		funcName := fmt.Sprintf("%d-%s", index, coroutines.GetStructName(handle))
		singleRun := utils2.NewSingleRun(funcName)
		runInfo, err := singleRun.SingleRun(func() error {
			return handle.Do(ctx)
		})
		if err != nil {
			if errors.Is(err, utils2.ErrProcessIsRunning) {
				executionTime := time.Since(runInfo.StartTime)
				if executionTime < handle.MaxExecuteTime() {
					// 执行时间小于最大执行时间，不报错
					return
				}
				err = e.Err(err, fmt.Sprintf(
					"funcName: %s is running\nstartTime: %s\nnowTime: %s",
					funcName,
					utils2.NewTime(runInfo.StartTime).Format(time.DateTime),
					utils2.NewLocal().Format(time.DateTime),
				))

			}
			e.SendMessage(ctx, err)
		}
	})
}

type Handler interface {
	New() Handler
	Spec() string
	MaxExecuteTime() time.Duration
	Do(ctx context.Context) error
}
