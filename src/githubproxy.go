package main

// GitHub 加速源（管理员设置，系统级）：应用访问 GitHub 的场景有两处 ——
// 「检查更新」读取 latest 分支 releases.js、「下载发布适配器」读取 main 分支脚本。
// 默认关闭（直连），开启后可选择一个预设加速线路，或由管理员自行添加地址。
//
// 加速前缀的拼法是 <前缀><原始 URL>，因此只接受前缀型代理。

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GitHubProxy 加速源配置，持久化在 ${TRIM_PKGETC}/github-proxy.json。
type GitHubProxy struct {
	Enabled bool     `json:"enabled"`
	Active  string   `json:"active"`           // 当前使用的加速前缀，空表示直连
	Custom  []string `json:"custom,omitempty"` // 管理员自行添加的加速地址
}

// ProxyPreset 内置加速线路（均为前缀型：<前缀><原始地址>）。
type ProxyPreset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Desc string `json:"desc"`
}

var proxyPresets = []ProxyPreset{
	{Name: "gh-proxy.com", URL: "https://gh-proxy.com/", Desc: "通用加速，稳定性好"},
	{Name: "ghproxy.net", URL: "https://ghproxy.net/", Desc: "备用线路"},
	{Name: "ghfast.top", URL: "https://ghfast.top/", Desc: "备用线路"},
}

var (
	proxyMu    sync.RWMutex
	proxyCfg   = GitHubProxy{}
	proxyPathS = ""
)

const maxProxyCustom = 8

// loadGitHubProxy 启动时加载系统级配置。
func loadGitHubProxy(etcDir string) {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	proxyPathS = filepath.Join(etcDir, "github-proxy.json")
	proxyCfg = GitHubProxy{}
	raw, err := os.ReadFile(proxyPathS)
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, &proxyCfg)
	proxyCfg.Active = normalizeProxyURL(proxyCfg.Active)
}

// githubProxy 返回当前配置副本。
func githubProxy() GitHubProxy {
	proxyMu.RLock()
	defer proxyMu.RUnlock()
	out := proxyCfg
	out.Custom = append([]string{}, proxyCfg.Custom...)
	return out
}

// setGitHubProxy 保存配置（仅管理员可调用，见 handleSettings）。
func setGitHubProxy(next GitHubProxy) error {
	next.Active = normalizeProxyURL(next.Active)
	if next.Active != "" && !validProxyURL(next.Active) {
		return errors.New("加速地址需以 http:// 或 https:// 开头")
	}
	clean := make([]string, 0, len(next.Custom))
	for _, c := range next.Custom {
		c = normalizeProxyURL(c)
		if c == "" {
			continue
		}
		if !validProxyURL(c) {
			return errors.New("自定义加速地址需以 http:// 或 https:// 开头")
		}
		if len(clean) >= maxProxyCustom {
			break
		}
		dup := false
		for _, x := range clean {
			if x == c {
				dup = true
				break
			}
		}
		if !dup {
			clean = append(clean, c)
		}
	}
	next.Custom = clean

	proxyMu.Lock()
	defer proxyMu.Unlock()
	proxyCfg = next
	if proxyPathS == "" {
		return errors.New("配置目录未初始化")
	}
	if err := os.MkdirAll(filepath.Dir(proxyPathS), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(proxyPathS, data, 0o644)
}

// activeProxyPrefix 当前生效的加速前缀；未开启或未选择时返回空。
func activeProxyPrefix() string {
	proxyMu.RLock()
	defer proxyMu.RUnlock()
	if !proxyCfg.Enabled {
		return ""
	}
	return proxyCfg.Active
}

// normalizeProxyURL 统一成「以 / 结尾」的前缀形式。
func normalizeProxyURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func validProxyURL(s string) bool {
	return (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")) && len(s) <= 200
}

// handleGitHubProxy GET/PUT /api/settings/github。
func (a *App) handleGitHubProxy(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg := githubProxy()
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":  cfg.Enabled,
			"active":   cfg.Active,
			"custom":   cfg.Custom,
			"presets":  proxyPresets,
			"isAdmin":  ua.user.IsAdmin || ua.user.Source == "local",
			"defaults": defaultProxyURL(),
		})
	case http.MethodPut:
		// 加速线路影响整机对外访问，仅管理员（或局域网直连）可改
		if !ua.user.IsAdmin && ua.user.Source != "local" {
			writeErr(w, http.StatusForbidden, "仅管理员可修改 GitHub 加速源")
			return
		}
		var in GitHubProxy
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if err := setGitHubProxy(in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := githubProxy()
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": cfg.Enabled,
			"active":  cfg.Active,
			"custom":  cfg.Custom,
		})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// defaultProxyURL 建议的加速地址（列表第一条），供前端在开启时预选。
func defaultProxyURL() string {
	if len(proxyPresets) == 0 {
		return ""
	}
	return proxyPresets[0].URL
}
