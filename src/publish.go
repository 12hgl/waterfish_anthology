// 水鱼文集 —— 发布适配器管理（实验性功能）
// 适配器是独立 JS 文件，托管在仓库 publishers/ 目录，不内置在应用内。
// 用户在「设置 → 实验性功能」中按需下载（直连失败走 gh-proxy 加速），
// 下载后配置密钥等信息，文章编辑页即可直接发布到对应建站系统。
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// publishersIndexURL 仓库适配器清单（CI 随 latest 分支同步）。
	publishersIndexURL = "https://raw.githubusercontent.com/12hgl/waterfish_anthology/latest/publishers/index.js"
	// publisherFileBase 适配器文件下载基址。
	publisherFileBase = "https://raw.githubusercontent.com/12hgl/waterfish_anthology/latest/publishers/"
	// maxPublisherBytes 适配器体积上限（256 KiB），防误下大文件。
	maxPublisherBytes = 256 << 10
)

// PublisherConfig 单个已安装发布适配器的配置（含用户填写的密钥等）。
type PublisherConfig struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Desc    string            `json:"desc"`
	Version string            `json:"version"`
	Config  map[string]string `json:"config"`
}

// InstalledPublisher 已安装适配器（附带文件是否存在标记）。
type InstalledPublisher struct {
	PublisherConfig
	Missing bool `json:"missing"` // 文件丢失（如数据目录被清理），需重新下载
}

// AvailablePublisher 仓库侧可下载的适配器元数据。
type AvailablePublisher struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	File    string `json:"file"`
	Version string `json:"version"`
}

// PublishersResponse GET /api/publishers 的响应。
type PublishersResponse struct {
	Installed []InstalledPublisher `json:"installed"`
	Available []AvailablePublisher `json:"available"`
	Error     string               `json:"error,omitempty"` // 清单拉取失败说明（installed 仍可用）
}

// sanitizePublisherID 清洗适配器 ID 为安全文件名（防路径穿越）。
func sanitizePublisherID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 64 {
		return "", fmt.Errorf("适配器 ID 不合法")
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return "", fmt.Errorf("适配器 ID 含非法字符")
		}
	}
	return id, nil
}

// publishersDir 适配器文件目录（跟随用户配置目录，与 config.json 同级）。
func (u *UserApp) publishersDir() string { return filepath.Join(u.userDir(), "publishers") }

func (u *UserApp) publisherPath(id string) string {
	return filepath.Join(u.publishersDir(), id+".js")
}

// Publishers 当前用户已安装的适配器配置列表。
func (u *UserApp) Publishers() []PublisherConfig {
	u.mu.RLock()
	defer u.mu.RUnlock()
	out := make([]PublisherConfig, len(u.cfg.Publishers))
	copy(out, u.cfg.Publishers)
	return out
}

// setPublisher 新增或更新适配器配置并持久化（保留原有密钥配置）。
func (u *UserApp) setPublisher(p PublisherConfig) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.cfg.Publishers {
		if u.cfg.Publishers[i].ID == p.ID {
			if p.Config == nil {
				p.Config = u.cfg.Publishers[i].Config // 未提交配置时保留原值
			}
			u.cfg.Publishers[i] = p
			return u.saveConfig()
		}
	}
	if p.Config == nil {
		p.Config = map[string]string{}
	}
	u.cfg.Publishers = append(u.cfg.Publishers, p)
	return u.saveConfig()
}

// setPublisherConfig 仅更新适配器的用户配置（密钥等）。
func (u *UserApp) setPublisherConfig(id string, cfg map[string]string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.cfg.Publishers {
		if u.cfg.Publishers[i].ID == id {
			u.cfg.Publishers[i].Config = cfg
			return u.saveConfig()
		}
	}
	return fmt.Errorf("适配器未安装")
}

// removePublisher 卸载适配器：删除文件与配置。
func (u *UserApp) removePublisher(id string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	idx := -1
	for i := range u.cfg.Publishers {
		if u.cfg.Publishers[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("适配器未安装")
	}
	if err := os.Remove(u.publisherPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	u.cfg.Publishers = append(u.cfg.Publishers[:idx], u.cfg.Publishers[idx+1:]...)
	return u.saveConfig()
}

// validateConfigMap 校验用户提交的适配器配置：键名安全、体积封顶。
func validateConfigMap(cfg map[string]string) (map[string]string, error) {
	if cfg == nil {
		return map[string]string{}, nil
	}
	total := 0
	for k, v := range cfg {
		if len(k) > 64 || len(v) > 4096 {
			return nil, fmt.Errorf("配置项 %q 过长", k)
		}
		total += len(k) + len(v)
	}
	if total > 16<<10 {
		return nil, fmt.Errorf("配置总量过大")
	}
	return cfg, nil
}

// extractJSArray 提取 `window.xxx = [ … ]` 中的数组文本（括号配对，感知字符串）。
func extractJSArray(content, varName string) (string, error) {
	idx := strings.Index(content, varName)
	if idx < 0 {
		return "", fmt.Errorf("未找到清单")
	}
	rest := content[idx+len(varName):]
	trimmed := strings.TrimLeft(rest, " \t")
	lead := len(rest) - len(trimmed)
	eq := strings.TrimLeft(trimmed, "= \t")
	lead += len(trimmed) - len(eq)
	open := strings.Index(eq, "[")
	if open < 0 {
		return "", fmt.Errorf("清单格式不正确")
	}
	start := idx + len(varName) + lead + open
	depth, inStr, esc := 0, false, false
	for i := start; i < len(content); i++ {
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
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return content[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("清单不完整")
}

// parsePublishersIndex 解析仓库适配器清单 index.js。
func parsePublishersIndex(content string) ([]AvailablePublisher, error) {
	arrText, err := extractJSArray(content, "window.waterfishPublishers")
	if err != nil {
		return nil, err
	}
	var list []AvailablePublisher
	if err := json.Unmarshal([]byte(arrText), &list); err != nil {
		return nil, fmt.Errorf("清单解析失败: %v", err)
	}
	return list, nil
}

// extractPublisherMeta 从适配器源码中提取 name/version。
// 适配器注册体不保证是严格 JSON（允许单引号、无引号键、尾逗号），
// 因此在注册赋值体（最后一个 window.WFPublishers… = {）之后的文本内宽松匹配。
var (
	metaNameRe    = regexp.MustCompile(`name\s*:\s*['"]([^'"]{1,64})['"]`)
	metaVersionRe = regexp.MustCompile(`version\s*:\s*['"]([0-9][^'"]{0,31})['"]`)
	// publisherRegRe 注册赋值体：window.WFPublishers.<id> = { 或 window.WFPublishers[id] = {
	publisherRegRe = regexp.MustCompile(`window\.WFPublishers(?:\.[A-Za-z0-9_-]+|\[[^\]]+\])?\s*=\s*\{`)
)

func extractPublisherMeta(content string) (name, version string) {
	locs := publisherRegRe.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return "", ""
	}
	scope := content[locs[len(locs)-1][1]:] // 元数据在注册体中，位于文件末尾
	if len(scope) > 4096 {
		scope = scope[:4096]
	}
	if m := metaNameRe.FindStringSubmatch(scope); m != nil {
		name = m[1]
	}
	if m := metaVersionRe.FindStringSubmatch(scope); m != nil {
		version = m[1]
	}
	return name, version
}

// handlePublishers GET /api/publishers（已安装 + 可下载清单）。
func (a *App) handlePublishers(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别用户")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp := PublishersResponse{
			Installed: []InstalledPublisher{},
			Available: []AvailablePublisher{},
		}
		for _, p := range ua.Publishers() {
			_, err := os.Stat(ua.publisherPath(p.ID))
			resp.Installed = append(resp.Installed, InstalledPublisher{
				PublisherConfig: p,
				Missing:         os.IsNotExist(err),
			})
		}
		content, err := fetchWithProxy(publishersIndexURL)
		if err != nil {
			resp.Error = fmt.Sprintf("获取适配器清单失败：%v", err)
		} else if list, perr := parsePublishersIndex(content); perr != nil {
			resp.Error = fmt.Sprintf("适配器清单无效：%v", perr)
		} else {
			resp.Available = list
		}
		writeJSON(w, http.StatusOK, resp)

	case http.MethodPost:
		// 安装适配器：下载仓库文件 → 校验 → 落盘 → 登记配置
		var body struct {
			ID   string `json:"id"`
			Name string `json:"name"` // 清单元数据（前端传来，省一次清单拉取）
			Desc string `json:"desc"`
		}
		if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.ID) == "" {
			writeErr(w, http.StatusBadRequest, "缺少适配器 ID")
			return
		}
		id, err := sanitizePublisherID(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content, err := fetchWithProxy(publisherFileBase + id + ".js")
		if err != nil {
			writeErr(w, http.StatusBadGateway, fmt.Sprintf("下载适配器失败：%v", err))
			return
		}
		if len(content) > maxPublisherBytes {
			writeErr(w, http.StatusBadGateway, "适配器文件过大")
			return
		}
		if !strings.Contains(content, "window.WFPublishers") || !strings.Contains(content, "publish") {
			writeErr(w, http.StatusBadGateway, "文件不是有效的发布适配器")
			return
		}
		// 元数据：优先用清单信息，其次解析适配器源码本身，最后退化为 ID
		name, version := extractPublisherMeta(content)
		if strings.TrimSpace(body.Name) != "" {
			name = strings.TrimSpace(body.Name)
		}
		if name == "" {
			name = id
		}
		if version == "" {
			version = "1.0.0"
		}
		if err := os.MkdirAll(ua.publishersDir(), 0o755); err != nil {
			writeErr(w, http.StatusInternalServerError, "无法创建适配器目录")
			return
		}
		if err := os.WriteFile(ua.publisherPath(id), []byte(content), 0o644); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存适配器失败")
			return
		}
		p := PublisherConfig{ID: id, Name: name, Desc: strings.TrimSpace(body.Desc), Version: version}
		if err := ua.setPublisher(p); err != nil {
			writeErr(w, http.StatusInternalServerError, "登记适配器失败")
			return
		}
		writeJSON(w, http.StatusOK, p)

	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// handlePublisherItem GET /api/publishers/{id}（元数据）/ {id}/source（源码，供前端 <script> 加载）
// PUT（保存配置）/ DELETE（卸载）。
func (a *App) handlePublisherItem(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别用户")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/publishers/")
	if r.Method == http.MethodGet && strings.HasSuffix(rest, "/source") {
		id, err := sanitizePublisherID(strings.TrimSuffix(rest, "/source"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		data, err := os.ReadFile(ua.publisherPath(id))
		if err != nil {
			writeErr(w, http.StatusNotFound, "适配器文件不存在，请重新下载")
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
		return
	}
	id, err := sanitizePublisherID(rest)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		for _, p := range ua.Publishers() {
			if p.ID == id {
				_, statErr := os.Stat(ua.publisherPath(id))
				writeJSON(w, http.StatusOK, InstalledPublisher{
					PublisherConfig: p,
					Missing:         os.IsNotExist(statErr),
				})
				return
			}
		}
		writeErr(w, http.StatusNotFound, "适配器未安装")

	case http.MethodPut:
		var body struct {
			Config map[string]string `json:"config"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		cfg, err := validateConfigMap(body.Config)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ua.setPublisherConfig(id, cfg); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if err := ua.removePublisher(id); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}
