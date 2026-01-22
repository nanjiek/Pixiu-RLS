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
	"github.com/nanjiek/pixiu-rls/internal/limiter"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/rules"
	"github.com/nanjiek/pixiu-rls/internal/rules/source"
)

func main() {
	confPath := flag.String("c", "configs/rls.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*confPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	repoAny, err := repo.NewRedis(cfg, nil)
	if err != nil {
		log.Fatalf("failed to init redis: %v", err)
	}
	rdb, ok := repoAny.(*repo.RedisRepo)
	if !ok {
		log.Fatalf("unexpected repo type: %T", repoAny)
	}
	defer rdb.Close()

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

	tbLimiter := limiter.NewTokenBucket(rdb)
	slidingLimiter := limiter.NewSlidingWindow(rdb)
	leakyLimiter := limiter.NewLeakyBucket(rdb)
	limiterMux := limiter.NewMux("token_bucket", map[string]limiter.Limiter{
		"token_bucket":   tbLimiter,
		"sliding_window": slidingLimiter,
		"leaky_bucket":   leakyLimiter,
	})
	engine := core.NewEngine(rdb, limiterMux, cfg.Features.FailPolicy)

	httpServer := api.NewServer(cfg.Server, ruleCache, engine)
	r := mux.NewRouter()
	httpServer.RegisterRoutes(r)

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")
	cancelRoot()
	engine.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
	log.Println("server exited properly")
}
