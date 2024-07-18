package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/robfig/cron/v3"
	utils2 "go-lib/common/utils"
	e "go-lib/err"
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
	ctx := utils2.NewContext("CRON")
	utils2.SafeFunc(ctx, func() {
		funcName := utils2.GetStructName(handle)
		running := utils2.NewSingleRun(funcName)
		err := running.SingleRun(func() error {
			return handle.Do(ctx)
		})
		if err != nil {
			if errors.Is(err, utils2.ErrProcessIsRunning) {
				err = e.Err(err, "funcName:"+funcName)
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
