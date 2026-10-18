// mockcore 启动一个假的 botd，供本地开发、插件调试和端到端联调使用。
// 它不连接任何游戏服务器，也没有真实的账号逻辑。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"panko/hub/internal/mockcore"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:3039", "监听地址")
	chat := flag.Bool("chat", false, "每 5 秒推送一条模拟聊天事件")
	flag.Parse()

	srv, err := mockcore.Start(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动模拟核心失败: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	fmt.Printf("mockcore 监听 %s\n", srv.Addr())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	if *chat {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			n := 0
			for range ticker.C {
				n++
				srv.Publish("chat", map[string]any{
					"sender":  "MockPlayer",
					"message": fmt.Sprintf("模拟消息 %d", n),
				})
			}
		}()
	}

	<-stop
	fmt.Println("mockcore 退出")
}
