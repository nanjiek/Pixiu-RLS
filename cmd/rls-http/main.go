package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/nanjiek/pixiu-rls/internal/api"
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/core"
	"github.com/nanjiek/pixiu-rls/internal/core/strategy" // 仅入口依赖strategy包
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/rules"
)

func main() {
	// 解析命令行参数
	confPath := flag.String("c", "configs/rls.yaml", "path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*confPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 初始化Redis连接
	rdb := repo.NewRedis(cfg)
	defer rdb.Close()

	// 初始化规则缓存
	ruleCache := rules.NewCache(cfg, rdb)
	if err := ruleCache.Bootstrap(context.Background()); err != nil {
		log.Fatalf("failed to bootstrap rules: %v", err)
	}
	// 启动规则更新监听器
	go ruleCache.StartWatcher(context.Background())

	// -------------------------- 关键：依赖注入策略实例 --------------------------
	// 1. 创建具体策略实例
	// 原来：
	// sliding := strategy.NewSliding(rdb)
	// token   := strategy.NewToken(rdb)
	// leaky   := strategy.NewLeaky(rdb)

	// 替换为（用熔断装饰器包装）
	sliding := strategy.WithBreaker(rdb, strategy.NewSliding(rdb), "sliding_window")
	token := strategy.WithBreaker(rdb, strategy.NewToken(rdb), "token_bucket")
	leaky := strategy.WithBreaker(rdb, strategy.NewLeaky(rdb), "leaky_bucket")

	// 2. 构建策略映射（算法名→策略实例）
	strategies := map[string]core.Strategy{
		"sliding_window": sliding, // 滑动窗口算法
		"token_bucket":   token,   // 令牌桶算法
		"leaky_bucket":   leaky,   // 漏桶算法
	}

	// 3. 初始化核心引擎（注入策略）
	engine := core.NewEngine(rdb, strategies)

	// 初始化HTTP服务
	// 初始化HTTP服务（只负责注册路由）
	httpServer := api.NewServer(cfg.Server, ruleCache, engine)

	// 用你自己的 router
	r := mux.NewRouter()
	httpServer.RegisterRoutes(r)

	// 原生 http.Server，方便优雅退出
	srv := &http.Server{
		Addr:    cfg.Server.HTTPAddr,
		Handler: r,
	}

	go func() {
		log.Printf("server is running on %s (PID: %d)", cfg.Server.HTTPAddr, os.Getpid())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
	log.Println("server exited properly")

}
