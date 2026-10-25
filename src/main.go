// 水鱼文集 —— 飞牛 fnOS 原生应用
// 单二进制服务：文章库 / 图片库 / SEO 检测 / 设置
// 双通道监听：TCP 端口（局域网直连）+ Unix socket（飞牛统一网关，携带登录用户身份）
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	app := NewApp()

	mux := http.NewServeMux()
	app.Routes(mux)
	handler := app.Middleware(mux)

	srv := &http.Server{
		Addr:              ":" + app.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("水鱼文集 %s 已启动，监听 :%s", app.Version, app.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 飞牛统一网关：在 ${TRIM_APPDEST} 创建 Unix socket，网关校验登录态后转发到此
	sockPath := filepath.Join(app.DestDir, gatewaySocket)
	_ = os.Remove(sockPath)
	sockLn, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Printf("警告：Unix socket 监听失败（网关入口不可用，直连端口仍可访问）: %v", err)
	} else {
		// 网关进程需要连接权限
		_ = os.Chmod(sockPath, 0o666)
		srvSock := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			log.Printf("网关 socket 就绪: %s", sockPath)
			if err := srvSock.Serve(sockLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("网关 socket 服务停止: %v", err)
			}
		}()
		defer func() {
			_ = srvSock.Close()
			_ = os.Remove(sockPath)
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Print("水鱼文集已停止")
}

// Routes 注册全部路由。
func (a *App) Routes(mux *http.ServeMux) {
	// 前端静态资源
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", noCache(http.FileServer(http.FS(sub)))))
	mux.HandleFunc("/", a.handleIndex(sub))

	// 本地图片直链（供 Markdown 引用 photo/xxx.png 使用）
	mux.HandleFunc("/photo/", a.handleLocalImage)

	// 状态与设置
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/settings/storage", a.handleSetStorage)
	mux.HandleFunc("/api/theme.js", a.handleThemeJS)
	mux.HandleFunc("/api/browse", a.handleBrowse)

	// 文章库
	mux.HandleFunc("/api/articles", a.handleArticles)
	mux.HandleFunc("/api/articles/", a.handleArticle)

	// 图片库
	mux.HandleFunc("/api/images", a.handleImages)
	mux.HandleFunc("/api/uploads", a.handleUpload)
	mux.HandleFunc("/api/images/", a.handleImage)

	// SEO
	mux.HandleFunc("/api/seo", a.handleSEOList)
	mux.HandleFunc("/api/seo/", a.handleSEO)
}

func (a *App) handleIndex(sub fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "前端资源缺失", http.StatusInternalServerError)
			return
		}
		// 网关模式下页面挂在 /app/waterfish_anthology/ 前缀下，
		// 注入 <base> 让相对路径的资源与接口请求都能正确解析
		base := prefixFromCtx(r) + "/"
		html := strings.Replace(string(data), "<head>", "<head>\n<base href=\""+base+"\">", 1)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(html))
	}
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}
