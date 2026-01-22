package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

import (
	"github.com/gorilla/mux"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/api"
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/core"
	"github.com/nanjiek/pixiu-rls/internal/core/strategy"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/rules"
	"github.com/nanjiek/pixiu-rls/internal/rules/source"
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

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	// 初始化Redis连接
	rdb := repo.NewRedis(cfg)
	defer rdb.Close()

	// Init rule cache
	ruleCache := rules.NewCache(cfg, rdb)
	if cfg.Nacos.Enabled() {
		nacosSource := source.NewNacosSource(cfg.Nacos)
		poller := rules.NewPoller(nacosSource, ruleCache, rules.PollerConfig{
			Interval:   time.Duration(cfg.Nacos.PollIntervalMs) * time.Millisecond,
			FailPolicy: cfg.Nacos.FailPolicy,
		})
		if err := poller.SyncOnce(rootCtx); err != nil {
			if strings.EqualFold(cfg.Nacos.FailPolicy, "fail-closed") {
				log.Fatalf("failed to load rules from nacos: %v", err)
			}
			log.Printf("nacos pull failed, using last-good rules: %v", err)
		}
		go poller.Start(rootCtx)
	} else {
		if err := ruleCache.Bootstrap(rootCtx); err != nil {
			log.Fatalf("failed to bootstrap rules: %v", err)
		}
		go ruleCache.StartWatcher(rootCtx)
	}

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
	cancelRoot()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
	log.Println("server exited properly")

}
