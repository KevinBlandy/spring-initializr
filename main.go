package main

import (
	"bytes"
	"context"
	"github.com/andybalholm/brotli"
	"golang.org/x/net/html"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

	reverseProxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		log.Printf("ErrorHandler Error: %s\n", err.Error())
	}
	reverseProxy.ModifyResponse = func(response *http.Response) (err error) {

		// 仅仅修改主页
		if response.Request.RequestURI != "/" {
			return nil
		}
		contentType := response.Header.Get("Content-Type")

		// 忽略非 html 响应
		if contentType == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "text/html") {
			return nil
		}

		//// 忽略空响应
		//if response.ContentLength < 1 {
		//	return nil
		//}

		contentEncoding := response.Header.Get("Content-Encoding")

		var payload []byte

		if contentEncoding == "br" {
			payload, err = io.ReadAll(brotli.NewReader(response.Body))
			if err != nil {
				return err
			}
		} else if contentEncoding == "gzip" {
			// TODO gzip 压缩
		} else {
			// 未知的压缩方式
			return nil
		}

		defer func() {
			if err != nil {
				log.Printf("ModifyResponse Error: %s\n", err.Error())
				response.Body = io.NopCloser(bytes.NewReader(payload))
			}
		}()

		// 解析html
		document, err := html.Parse(bytes.NewReader(payload))
		if err != nil {
			return nil
		}

		// TODO 解析HTML，在 </body> 前插入数据
		// TODO 修改length & encoding
		// TODO 压缩编码给客户端

		// 渲染到内存
		buf := &bytes.Buffer{}
		if err = html.Render(buf, document); err != nil {
			return err
		}

		response.Header.Del("Content-Encoding")
		response.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
		response.Body = io.NopCloser(buf)
		return nil
	}

	router := http.NewServeMux()
	router.HandleFunc("/about", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(writer, "https://start.springboot.io/about")
	})

	// Proxy
	router.HandleFunc("/", reverseProxy.ServeHTTP)

	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			request.Host = targetURL.Host
			request.Header.Set("X-User-Agent", "https://start.springboot.io/about")
			router.ServeHTTP(writer, request)
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
