package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/httpapi"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:25500", "HTTP 监听地址")
	readHeaderTimeout := flag.Duration("read-header-timeout", 5*time.Second, "HTTP ReadHeaderTimeout（请求头读取超时）")
	convertTimeout := flag.Duration("convert-timeout", 60*time.Second, "单次转换的总超时（包含远程拉取）")
	fetchTimeout := flag.Duration("fetch-timeout", 15*time.Second, "单次远程拉取的超时（每个 URL 一次请求）")
	shutdownTimeout := flag.Duration("shutdown-timeout", 10*time.Second, "收到退出信号后的优雅退出等待时间")
	flag.Parse()

	srv := &http.Server{
		Addr: *listen,
		Handler: httpapi.NewHandlerWithOptions(httpapi.Options{
			ConvertTimeout: *convertTimeout,
			FetchTimeout:   *fetchTimeout,
		}),
		ReadHeaderTimeout: *readHeaderTimeout,
	}

	log.Printf("listening on http://%s", *listen)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Printf("shutdown signal received")

		shCtx, cancel := context.WithTimeout(context.Background(), *shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			_ = srv.Close()
		}

		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}
