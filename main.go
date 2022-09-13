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
	BaiduStatistics   = `
var _hmt = _hmt || [];
(function() {
  var hm = document.createElement("script");
  hm.src = "https://hm.baidu.com/hm.js?01a8b83d4f38d7c890e8dbcaa8e661d3";
  var s = document.getElementsByTagName("script")[0]; 
  s.parentNode.insertBefore(hm, s);
})();

`
	About = `
To Pivotal

Hello, due to the firewall blocking in China, most Chinese users are not able to use the official Spring Initializr service, so I created this proxy service, specifically for spring users in mainland China. Please don't block me, thank you very much.

Email: admin@springboot.io

`
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

		// ---------------- 解析html ----------------
		document, err := html.Parse(bytes.NewReader(payload))
		if err != nil {
			return nil
		}

		// 删除body前面的3个script标签，官方用于统计的，我这里用不着
		bodyNode := GetNode(document, func(node *html.Node) bool {
			return node.Type == html.ElementNode && node.Data == "body"
		})

		var scriptNodes []*html.Node
		for node := bodyNode.LastChild; node != nil; node = node.PrevSibling {
			if node.Type == html.ElementNode && node.Data == "script" {
				scriptNodes = append(scriptNodes, node)
			}
		}
		for _, v := range scriptNodes {
			bodyNode.RemoveChild(v)
		}

		// head标签
		headNode := GetNode(document, func(node *html.Node) bool {
			return node.Type == html.ElementNode && node.Data == "head"
		})

		// 修改 description
		descriptionNode := GetNode(headNode, func(node *html.Node) bool {
			if node.Type == html.ElementNode && node.Data == "meta" {
				for _, v := range node.Attr {
					if strings.EqualFold(v.Key, "name") && strings.EqualFold(v.Val, "description") {
						return true
					}
				}
			}
			return false
		})
		if descriptionNode != nil {
			for i := range descriptionNode.Attr {
				if descriptionNode.Attr[i].Key == "content" {
					descriptionNode.Attr[i].Val = "快速生成你的Spring Boot应用"
				}
			}
		}

		// 插入Keywords
		headNode.InsertBefore(&html.Node{
			Type: html.ElementNode,
			Data: "meta",
			Attr: []html.Attribute{{
				Key: "content",
				Val: "spring boot, Spring Initializr, spring boot 脚手架",
			}, {
				Key: "name",
				Val: "keywords",
			}},
		}, descriptionNode)

		// 自定义脚本
		headNode.AppendChild(&html.Node{
			Type:     html.ElementNode,
			DataAtom: 0,
			Data:     "script",
			Attr: []html.Attribute{{
				Key: "src",
				Val: "/localhost/index.js",
			}},
		})

		// 插入百度统计代码
		headNode.AppendChild(&html.Node{
			FirstChild: &html.Node{
				Type: html.TextNode,
				Data: BaiduStatistics,
			},
			Type: html.ElementNode,
			Data: "script",
		})

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
		_, _ = io.WriteString(writer, About)
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

func GetNode(node *html.Node, test func(*html.Node) bool) *html.Node {
	if test(node) {
		return node
	}
	if node.FirstChild != nil {
		if ret := GetNode(node.FirstChild, test); ret != nil {
			return ret
		}
	}
	if node.NextSibling != nil {
		if ret := GetNode(node.NextSibling, test); ret != nil {
			return ret
		}
	}
	return nil
}
