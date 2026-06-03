//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"myproject/config"
	"myproject/dao"
	"myproject/model"
	"myproject/server/api/cmd"
	"myproject/server/api/router"

	lib "go-lib"
	"go-lib/common/coroutines"
	log2 "go-lib/log"
	"go-lib/provider/HTTPServer/gin/handler"

	"github.com/gin-gonic/gin"
)

// init 统一初始化入口，各服务遵循相同模板
func init() {
	// 1. 初始化服务名和环境
	lib.Init(config.Config.ServerName, config.Config.ENV)

	// 2. 初始化全局日志
	lib.InitLog(log2.NewLogger(config.Config.Logging))

	// 3. 启动崩溃捕获（可选，推荐）
	flush := lib.BootstrapCrashCapture()

	// 4. 初始化告警 Sender（按需，此处省略具体实现）
	// lib.InitGlobalSender(sender)

	// 5. 补报历史 crash dump
	flush(coroutines.NewContext("crash-report"))

	// 6. 初始化数据源
	dao.InitRedis()
	model.Init()
}

func main() {
	// 创建 Gin 引擎并挂载标准中间件链
	r := gin.New()
	r.Use(gin.CustomRecovery(handler.RecoveryHandler())) // panic 恢复 + 告警
	r.Use(handler.CorsMiddleware())                      // CORS 跨域
	r.Use(handler.RequestIDMiddleware())                 // 注入 RequestID 到 context
	r.Use(handler.RequestLogMiddleware())                // 请求/响应日志

	// 注册路由
	router.RegisterRouter(r)

	// 启动定时任务
	cmd.StartScheduler()

	// 启动 HTTP 服务
	srv := &http.Server{
		Addr:    config.Config.ServerPort,
		Handler: r,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if err := srv.Shutdown(context.Background()); err != nil {
		log2.WithContext(context.Background()).Error("server shutdown error: " + err.Error())
	}
}
