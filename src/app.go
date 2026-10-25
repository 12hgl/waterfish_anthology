package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Version 由构建脚本通过 -ldflags 注入。
var Version = "1.0.0"

const (
	dirPassage = "passage"
	dirPhoto   = "photo"
	dirSEO     = "seo"
	dirMeta    = ".waterfish"

	maxUploadBytes = 64 << 20 // 64 MiB

	// gatewayPrefix 飞牛统一网关访问前缀（与 app/ui/config 的 gatewayPrefix 一致）。
	gatewayPrefix = "/app/waterfish_anthology"
	// gatewaySocket 网关转发的 Unix socket 文件名（创建于 ${TRIM_APPDEST}）。
	gatewaySocket = "app.sock"
)

// CloudConfig Cloudinary 图床配置。
type CloudConfig struct {
	Enabled      bool   `json:"enabled"`
	CloudName    string `json:"cloudName"`
	UploadPreset string `json:"uploadPreset"`
}

// CustomTheme 用户上传的自定义主题（JS 脚本，按上传文件名命名）。
type CustomTheme struct {
	Name string `json:"name"` // 主题名（取自文件名，重名加 (2) 后缀）
	JS   string `json:"js"`   // 脚本内容
}

// AppearanceConfig 外观配置：主题打包选择（内置主题 / 自定义 JS 主题）+ 深浅色模式。
// 外观统一由主题打包下发，不再单独调节配色 / 字体 / 字号 / 背景。
type AppearanceConfig struct {
	Theme  string        `json:"theme"`  // 当前主题：内置主题 id 或自定义主题名
	Mode   string        `json:"mode"`   // auto | light | dark
	Themes []CustomTheme `json:"themes"` // 自定义主题列表

	// 更早版本的旧字段，仅用于加载时迁移，之后不再写回
	LegacyPreset     string `json:"preset,omitempty"`
	LegacyAccent     string `json:"accent,omitempty"`
	LegacyFontFamily string `json:"fontFamily,omitempty"`
	LegacyFontScale  int    `json:"fontScale,omitempty"`
	LegacyBgStyle    string `json:"bgStyle,omitempty"`
	LegacyCustomJS   string `json:"customJS,omitempty"`
	LegacyCustomName string `json:"customName,omitempty"`
}

// Config 单用户配置，持久化在 ${TRIM_PKGVAR}/users/<用户>/config.json。
type Config struct {
	StorageRoot string           `json:"storageRoot"`
	Cloudinary  CloudConfig      `json:"cloudinary"`
	Appearance  AppearanceConfig `json:"appearance"`
}

// App 应用上下文（系统级共享状态；每用户数据见 UserApp）。
type App struct {
	EtcDir  string // ${TRIM_PKGETC}
	VarDir  string // ${TRIM_PKGVAR}
	DestDir string // ${TRIM_APPDEST}
	Port    string
	Version string

	// AuthPaths 用户在「应用设置 → 授权目录」中授予访问权限的路径。
	AuthPaths []string

	usersMu sync.RWMutex
	users   map[string]*UserApp
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func NewApp() *App {
	a := &App{
		EtcDir:    env("TRIM_PKGETC", "./.runtime/etc"),
		VarDir:    env("TRIM_PKGVAR", "./.runtime/var"),
		DestDir:   env("TRIM_APPDEST", "."),
		Port:      env("TRIM_SERVICE_PORT", "8631"),
		AuthPaths: splitPaths(env("TRIM_DATA_ACCESSIBLE_PATHS", "")),
		Version:   Version,
		users:     map[string]*UserApp{},
	}

	for _, d := range []string{a.EtcDir, a.VarDir, filepath.Join(a.VarDir, "users")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("创建目录失败 %s: %v", d, err)
		}
	}
	return a
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ":") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

// ---------- HTTP 辅助 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	return dec.Decode(v)
}

// Middleware 统一处理：剥离飞牛网关前缀 → 解析登录用户 → 安全响应头。
func (a *App) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := ""
		if p := r.URL.Path; strings.HasPrefix(p, gatewayPrefix) {
			prefix = gatewayPrefix
			r.URL.Path = strings.TrimPrefix(p, gatewayPrefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}
		if r.URL.RawPath != "" && strings.HasPrefix(r.URL.RawPath, gatewayPrefix) {
			r.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, gatewayPrefix)
		}

		user := a.resolveUser(r)
		ua := a.forUser(user)
		ctx := context.WithValue(r.Context(), ctxUserKey, ua)
		ctx = context.WithValue(ctx, ctxPrefixKey, prefix)

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------- 路径安全 ----------

// safeName 清洗用户提供的文件名，防止路径穿越。
func safeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return "", errors.New("非法文件名")
	}
	return name, nil
}

// writable 通过写入探针判断目录是否可写。
func writable(dir string) bool {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".wf-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
