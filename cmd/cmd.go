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
	//任务计划
	c := cron.New(cron.WithSeconds())
	cmdHandlers := handlers
	for _, v := range cmdHandlers {
		newHandler := v.New()
		_, err := c.AddFunc(newHandler.Spec(), func() {
			cmdHandle(newHandler)
		})
		if err != nil {
			panic(err)
		}
	}
	c.Start()
	fmt.Println("cron start")

	return c
}

func cmdHandle(handle Handler) {
	ctx := coroutines.NewContext("CRON")
	coroutines.SafeFunc(ctx, func(ctx context.Context) {
		funcName := coroutines.GetStructName(handle)
		running := utils2.NewSingleRun(funcName)
		runInfo, err := running.SingleRun(func() error {
			return handle.Do(ctx)
		})
		if err != nil {
			if errors.Is(err, utils2.ErrProcessIsRunning) {
				err = e.Err(err, fmt.Sprintf("funcName:"+funcName+" is running"+
					",startTime:"+utils2.NewTime(runInfo.StartTime).Format(time.DateTime)+
					",nowTime:"+utils2.NewLocal().Format(time.DateTime)))
			}
			e.SendMessage(ctx, err)
		}
	})
}

type Handler interface {
	New() Handler
	Spec() string
	Do(ctx context.Context) error
}
