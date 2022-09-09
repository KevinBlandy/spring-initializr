package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
)

func init() {
	log.Default().SetOutput(os.Stdout)
	log.Default().SetFlags(log.LstdFlags | log.Lshortfile)
}

const (
	// SpringInitializer Spring 官方的地址
	SpringInitializer = "https://start.spring.io/"
)

func main() {

	// 代理的地址
	targetURL, err := url.Parse(SpringInitializer)
	if err != nil {
		log.Fatalf("SpringInitializer 地址解析异常: %s\n", err.Error())
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)

	// 异常处理
	reverseProxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		log.Printf("ErrorHandler Error: %s\n", err.Error())
	}
	// 修改响应
	reverseProxy.ModifyResponse = func(response *http.Response) error {
		// 仅仅修改主页
		if response.Request.RequestURI != "/" {
			return nil
		}
		// TODO 解压BR数据
		// TODO 解析HTML，在 </body> 前插入数据
		// TODO 修改length & encoding
		// TODO 压缩编码给客户端
		payload, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		response.Body = io.NopCloser(bytes.NewReader(payload))
		return nil
	}

	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			request.Host = "start.spring.io"
			reverseProxy.ServeHTTP(writer, request)
		}),
	}

	go func() {
		log.Println("Server Start")
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer cancel()
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("Server Shutdown Error: %s\n", err.Error())
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server ListenAndServe Error: %s\n", err.Error())
	}

	log.Println("Bye")
}
