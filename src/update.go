// 水鱼文集 —— 版本更新检测
// 通过读取仓库 latest 分支的 releases.js（raw 直连失败时走 gh-proxy 加速）
// 获取最新版本号、更新内容与安装包下载地址，与当前运行版本比较。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// releasesFileURL latest 分支上的版本信息文件（由 CI 在发布 Release 后自动更新）。
	releasesFileURL = "https://raw.githubusercontent.com/12hgl/waterfish_anthology/latest/releases.js"
	// ghProxyPrefix 国内网络直连 GitHub raw 失败时的加速前缀。
	ghProxyPrefix = "https://gh-proxy.com/"
	// updateClientTimeout 更新检测专用短超时：直连失败时尽快切换加速线路。
	updateClientTimeout = 8 * time.Second
)

// updateClient 外网短超时客户端（复用带备用 DNS 的 Transport）。
var updateClient = &http.Client{
	Timeout:   updateClientTimeout,
	Transport: externalTransport,
}

// ReleaseInfo releases.js 中 window.waterfishRelease 的载荷。
type ReleaseInfo struct {
	Version string `json:"version"` // 最新版本号，如 1.0.1
	Date    string `json:"date"`    // 发布日期
	Notes   string `json:"notes"`   // 更新内容（changelog）
	X86     string `json:"x86"`     // x86_64 安装包下载直链
	ARM     string `json:"arm"`     // aarch64 安装包下载直链
	Release string `json:"release"` // Release 页面地址
}

// UpdateCheckResult GET /api/update-check 的响应。
type UpdateCheckResult struct {
	Current   string `json:"current"`   // 当前运行版本
	Latest    string `json:"latest"`    // 仓库最新版本
	HasUpdate bool   `json:"hasUpdate"` // 是否有可用更新
	Date      string `json:"date"`
	Notes     string `json:"notes"`
	X86       string `json:"x86"`
	ARM       string `json:"arm"`
	Release   string `json:"release"`
	Error     string `json:"error,omitempty"` // 拉取/解析失败时的说明（此时其余字段为空）
}

// fetchWithProxy 拉取文本：先直连，失败走 gh-proxy 加速兜底。
func fetchWithProxy(url string) (string, error) {
	body, err := fetchRemoteText(url)
	if err == nil {
		return body, nil
	}
	proxyBody, proxyErr := fetchRemoteText(ghProxyPrefix + url)
	if proxyErr != nil {
		return "", fmt.Errorf("直连失败（%v），加速线路也失败（%v）", friendlyNetErr(err), friendlyNetErr(proxyErr))
	}
	return proxyBody, nil
}

// fetchRemoteText 短超时拉取远端文本（更新检测 / 适配器下载用）。
func fetchRemoteText(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "waterfish-anthology/"+Version)
	resp, err := updateClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// releasesRe 定位 window.waterfishRelease = { … }。
var releasesRe = regexp.MustCompile(`window\.waterfishRelease\s*=\s*`)

// parseReleaseJS 从 releases.js 内容中提取 JSON 对象（花括号配对，感知字符串）。
func parseReleaseJS(content string) (*ReleaseInfo, error) {
	loc := releasesRe.FindStringIndex(content)
	if loc == nil {
		return nil, fmt.Errorf("未找到版本信息")
	}
	start := strings.Index(content[loc[1]:], "{")
	if start < 0 {
		return nil, fmt.Errorf("版本信息格式不正确")
	}
	open := loc[1] + start
	depth, inStr, esc := 0, false, false
	end := -1
	for i := open; i < len(content); i++ {
		c := content[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("版本信息不完整")
	}
	var info ReleaseInfo
	if err := json.Unmarshal([]byte(content[open:end]), &info); err != nil {
		return nil, fmt.Errorf("版本信息解析失败: %v", err)
	}
	if strings.TrimSpace(info.Version) == "" {
		return nil, fmt.Errorf("版本号为空")
	}
	return &info, nil
}

// compareVersions 语义化版本比较：a > b 返回 1，相等 0，a < b 返回 -1。
// 预发布标签（如 -beta）按小于正式版本处理；无法解析时退化为字符串比较。
func compareVersions(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// splitVersion 拆版本号为数字段：1.2.10-beta → [1 2 10]，非数字段按 0。
func splitVersion(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // 忽略预发布/构建标签，正式版视为更新
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// checkUpdate 拉取远端版本信息并与当前版本比较。
func (a *App) checkUpdate() *UpdateCheckResult {
	res := &UpdateCheckResult{Current: Version}
	content, err := fetchWithProxy(releasesFileURL)
	if err != nil {
		res.Error = fmt.Sprintf("检查更新失败：%v", err)
		return res
	}
	info, err := parseReleaseJS(content)
	if err != nil {
		res.Error = fmt.Sprintf("检查更新失败：%v", err)
		return res
	}
	res.Latest = info.Version
	res.Date = info.Date
	res.Notes = info.Notes
	res.X86 = info.X86
	res.ARM = info.ARM
	res.Release = info.Release
	res.HasUpdate = compareVersions(info.Version, Version) > 0
	return res
}

// handleUpdateCheck GET /api/update-check。
func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET")
		return
	}
	writeJSON(w, http.StatusOK, a.checkUpdate())
}
