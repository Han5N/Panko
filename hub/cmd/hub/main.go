// Panko hub 是本地控制面：承载 API、WebUI 和可选的调试日志流。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"panko/hub/internal/apiserver"
	"panko/hub/internal/config"
	"panko/hub/internal/coreclient"
	"panko/hub/internal/debugserver"
	"panko/hub/internal/events"
	"panko/hub/internal/plugin"
)

// version 在构建时通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	configPath := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	if err := run(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "panko hub 退出: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	baseDir := filepath.Dir(configPath)
	if !filepath.IsAbs(cfg.WebDir) {
		cfg.WebDir = filepath.Join(baseDir, cfg.WebDir)
	}
	if !filepath.IsAbs(cfg.PluginsDir) {
		cfg.PluginsDir = filepath.Join(baseDir, cfg.PluginsDir)
	}

	logHub := debugserver.NewLogHub()
	logger := log.New(io.MultiWriter(os.Stdout, logHub), "", log.LstdFlags)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bus := events.New(2048)
	client := coreclient.New(cfg.Core.Addr, bus, logger)

	host := plugin.NewHost(ctx, cfg.PluginsDir, client, bus, logger)
	if err := host.Load(); err != nil {
		return err
	}
	host.StartAuto()

	go client.Run(ctx)
	go host.RunFanout(ctx)

	api := apiserver.New(version, client, bus, host)

	logger.Printf("Panko hub %s 启动", version)
	logger.Printf("API 服务:   http://%s", cfg.API.Addr)
	logger.Printf("WebUI 服务: http://%s", cfg.WebUI.Addr)
	logger.Printf("核心接口:   tcp://%s", cfg.Core.Addr)
	logger.Printf("插件目录:   %s", cfg.PluginsDir)
	if cfg.Debug {
		logger.Printf("调试服务:   http://%s/logs（SSE）", cfg.DebugServer.Addr)
	}

	// 关闭服务时取消该 context，让 SSE 等长连接立即退出，而不是等 Shutdown 超时。
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	apiHandler := api.Handler()
	webMux := http.NewServeMux()
	webMux.Handle("/api/", http.StripPrefix("/api", apiHandler))
	webMux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))

	servers := []*http.Server{
		{
			Addr:              cfg.API.Addr,
			Handler:           apiHandler,
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return baseCtx },
		},
		{
			Addr:              cfg.WebUI.Addr,
			Handler:           webMux,
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return baseCtx },
		},
	}

	if cfg.Debug {
		servers = append(servers, &http.Server{
			Addr:              cfg.DebugServer.Addr,
			Handler:           debugserver.Handler(logHub, version),
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return baseCtx },
		})
	}

	errCh := make(chan error, len(servers))
	for _, server := range servers {
		go func(s *http.Server) {
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("监听 %s 失败: %w", s.Addr, err)
			}
		}(server)
	}

	var runErr error
	select {
	case <-ctx.Done():
		logger.Printf("收到退出信号，正在关闭服务")
	case runErr = <-errCh:
		logger.Printf("服务异常: %v", runErr)
	}

	cancelBase()
	host.StopAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("关闭 %s 失败: %v", server.Addr, err)
		}
	}

	logger.Printf("已退出")
	return runErr
}
