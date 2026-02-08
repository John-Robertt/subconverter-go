package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
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
	healthcheck := flag.Bool("healthcheck", false, "执行健康检查并退出（0=healthy，1=unhealthy）")
	healthcheckURL := flag.String("healthcheck-url", "", "健康检查 URL（默认由 -listen 推导为 http://127.0.0.1:<port>/healthz）")
	healthcheckTimeout := flag.Duration("healthcheck-timeout", 2*time.Second, "健康检查超时（建议小于容器 healthcheck 的 timeout）")
	flag.Parse()

	if *healthcheck {
		raw := strings.TrimSpace(*healthcheckURL)
		if raw == "" {
			u, err := deriveHealthzURL(*listen)
			if err != nil {
				log.Printf("healthcheck: invalid -listen=%q: %v", *listen, err)
				os.Exit(1)
			}
			raw = u
		}
		if err := runHealthcheck(raw, *healthcheckTimeout); err != nil {
			log.Printf("healthcheck failed: %v", err)
			os.Exit(1)
		}
		return
	}

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

func deriveHealthzURL(listenAddr string) (string, error) {
	addr := strings.TrimSpace(listenAddr)
	if addr == "" {
		return "", errors.New("empty listen addr")
	}

	// Allow passing a full URL for local dev convenience.
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err == nil && u != nil && u.Host != "" {
			u2 := *u
			u2.Path = "/healthz"
			u2.RawQuery = ""
			u2.Fragment = ""
			return u2.String(), nil
		}
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Support passing a bare port like "25500" in ad-hoc environments.
		if strings.Contains(addr, ":") {
			return "", err
		}
		host = "127.0.0.1"
		port = addr
	}

	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if port == "" {
		return "", errors.New("missing port")
	}

	// 0.0.0.0/:: are listen-only; healthcheck must use a routable local address.
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	// Bracket IPv6 in URL form.
	if strings.Contains(host, ":") && !(strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]")) {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port + "/healthz", nil
}

func runHealthcheck(rawURL string, timeout time.Duration) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("empty healthcheck url")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("unexpected status: " + resp.Status)
	}
	return nil
}
