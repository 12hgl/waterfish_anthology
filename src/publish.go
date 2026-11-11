// 水鱼文集 —— 发布适配器管理
//
// 适配器是独立 JS 文件，托管在仓库 publishers/ 目录，不内置在应用内。
// 安装 / 卸载是系统级操作（仅管理员）：文件落在 ${TRIM_PKGVAR}/publishers，
// 元数据登记在同目录的 publishers.json，装一次全体账号可用。
// 各账号的密钥配置按适配器 ID 存在自己的 config.json 里，互相隔离。
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
	// publishersIndexURL 仓库适配器清单（跟随 main 分支：适配器独立于安装包更新）。
	publishersIndexURL = "https://raw.githubusercontent.com/12hgl/waterfish_anthology/main/publishers/index.js"
	// publisherFileBase 适配器文件下载基址。
	publisherFileBase = "https://raw.githubusercontent.com/12hgl/waterfish_anthology/main/publishers/"
	// maxPublisherBytes 适配器体积上限（256 KiB），防误下大文件。
	maxPublisherBytes = 256 << 10
)

// PublisherConfig 单个发布适配器的用户级配置（密钥等）。
// Name / Desc / Version 为 1.2.x 的遗留字段，读取旧配置时兼容，不再写入。
type PublisherConfig struct {
	ID     string            `json:"id"`
	Config map[string]string `json:"config,omitempty"`

	Name    string `json:"name,omitempty"`
	Desc    string `json:"desc,omitempty"`
	Version string `json:"version,omitempty"`
}

// PublisherMeta 适配器的系统级元数据（登记在 publishers.json）。
type PublisherMeta struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Version string `json:"version"`
}

// InstalledPublisher 已安装适配器 + 当前用户的配置。
type InstalledPublisher struct {
	PublisherMeta
	Config  map[string]string `json:"config"`
	Missing bool              `json:"missing"` // 文件丢失（如数据目录被清理），需管理员重新下载
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
	Enabled   bool                 `json:"enabled"` // 当前用户的发布开关
	IsAdmin   bool                 `json:"isAdmin"` // 是否可安装 / 卸载适配器
	Error     string               `json:"error,omitempty"`
}

// isAdmin 当前用户是否具备系统级设置权限（网关管理员或本机直连）。
func (u *UserApp) isAdmin() bool {
	return u.User().IsAdmin || u.User().Source == "local"
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

// ---------- 系统级适配器存储 ----------

func (a *App) publishersDir() string { return filepath.Join(a.VarDir, "publishers") }

func (a *App) publishersRegistryPath() string { return filepath.Join(a.VarDir, "publishers.json") }

func (a *App) publisherPath(id string) string {
	return filepath.Join(a.publishersDir(), id+".js")
}

// PublisherRegistry 已安装适配器清单（系统级，全体账号共享）。
func (a *App) PublisherRegistry() []PublisherMeta {
	data, err := os.ReadFile(a.publishersRegistryPath())
	if err != nil {
		return []PublisherMeta{}
	}
	var list []PublisherMeta
	if json.Unmarshal(data, &list) != nil {
		return []PublisherMeta{}
	}
	return list
}

func (a *App) savePublisherRegistry(list []PublisherMeta) error {
	if err := os.MkdirAll(a.VarDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.publishersRegistryPath(), data, 0o644)
}

// setPublisherMeta 新增或覆盖登记项。
func (a *App) setPublisherMeta(m PublisherMeta) error {
	list := a.PublisherRegistry()
	for i := range list {
		if list[i].ID == m.ID {
			list[i] = m
			return a.savePublisherRegistry(list)
		}
	}
	list = append(list, m)
	return a.savePublisherRegistry(list)
}

// removePublisherMeta 删除登记项。
func (a *App) removePublisherMeta(id string) error {
	list := a.PublisherRegistry()
	out := make([]PublisherMeta, 0, len(list))
	for _, m := range list {
		if m.ID != id {
			out = append(out, m)
		}
	}
	return a.savePublisherRegistry(out)
}

// adoptLegacyPublishers 1.2.x 把适配器装在各用户目录下；升级后统一迁到系统目录，
// 保证「装一次全体可用」且旧账号的适配器不会凭空消失。
func (a *App) adoptLegacyPublishers() {
	matches, err := filepath.Glob(filepath.Join(a.VarDir, "users", "*", "publishers", "*.js"))
	if err != nil || len(matches) == 0 {
		return
	}
	if err := os.MkdirAll(a.publishersDir(), 0o755); err != nil {
		return
	}
	for _, src := range matches {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		id, err := sanitizePublisherID(strings.TrimSuffix(filepath.Base(src), ".js"))
		if err != nil {
			continue
		}
		dst := a.publisherPath(id)
		if _, err := os.Stat(dst); err == nil {
			continue // 系统目录已有同名适配器，保留较新的那份
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			continue
		}
		name, version := extractPublisherMeta(string(data))
		if name == "" {
			name = id
		}
		if version == "" {
			version = "1.0.0"
		}
		_ = a.setPublisherMeta(PublisherMeta{ID: id, Name: name, Version: version})
	}
}

// ---------- 用户级配置 ----------

// PublishEnabled 当前用户是否启用发布功能（未设置时默认开启）。
func (u *UserApp) PublishEnabled() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cfg.PublishEnabled == nil || *u.cfg.PublishEnabled
}

func (u *UserApp) setPublishEnabled(v bool) error {
	u.mu.Lock()
	u.cfg.PublishEnabled = &v
	err := u.saveConfig()
	u.mu.Unlock()
	return err
}

// publisherCfg 取当前用户保存的适配器配置。
func (u *UserApp) publisherCfg(id string) map[string]string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	for _, p := range u.cfg.Publishers {
		if p.ID == id {
			out := make(map[string]string, len(p.Config))
			for k, v := range p.Config {
				out[k] = v
			}
			return out
		}
	}
	return map[string]string{}
}

// setPublisherConfig 保存当前用户的适配器配置（按适配器 ID 覆盖）。
func (u *UserApp) setPublisherConfig(id string, cfg map[string]string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.cfg.Publishers {
		if u.cfg.Publishers[i].ID == id {
			u.cfg.Publishers[i] = PublisherConfig{ID: id, Config: cfg}
			return u.saveConfig()
		}
	}
	u.cfg.Publishers = append(u.cfg.Publishers, PublisherConfig{ID: id, Config: cfg})
	return u.saveConfig()
}

// dropPublisherConfig 删除当前用户的适配器配置（适配器被卸载后清理）。
func (u *UserApp) dropPublisherConfig(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]PublisherConfig, 0, len(u.cfg.Publishers))
	changed := false
	for _, p := range u.cfg.Publishers {
		if p.ID == id {
			changed = true
			continue
		}
		out = append(out, p)
	}
	if changed {
		u.cfg.Publishers = out
		_ = u.saveConfig()
	}
}

// uninstallPublisher 系统级卸载：删文件、删登记、清各账号的配置。
func (a *App) uninstallPublisher(id string) error {
	if err := os.Remove(a.publisherPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := a.removePublisherMeta(id); err != nil {
		return err
	}
	a.usersMu.RLock()
	users := make([]*UserApp, 0, len(a.users))
	for _, ua := range a.users {
		users = append(users, ua)
	}
	a.usersMu.RUnlock()
	for _, ua := range users {
		ua.dropPublisherConfig(id)
	}
	return nil
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

// installedPublishers 汇总已安装适配器（系统登记 + 当前用户配置 + 文件存在性）。
func (a *App) installedPublishers(ua *UserApp) []InstalledPublisher {
	reg := a.PublisherRegistry()
	out := make([]InstalledPublisher, 0, len(reg))
	for _, m := range reg {
		_, err := os.Stat(a.publisherPath(m.ID))
		out = append(out, InstalledPublisher{
			PublisherMeta: m,
			Config:        ua.publisherCfg(m.ID),
			Missing:       os.IsNotExist(err),
		})
	}
	return out
}

// handlePublishers GET /api/publishers（已安装 + 可下载清单，后者仅管理员可见）
// POST（安装，仅管理员）。
func (a *App) handlePublishers(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别用户")
		return
	}
	admin := ua.isAdmin()
	switch r.Method {
	case http.MethodGet:
		resp := PublishersResponse{
			Installed: a.installedPublishers(ua),
			Available: []AvailablePublisher{},
			Enabled:   ua.PublishEnabled(),
			IsAdmin:   admin,
		}
		// 非管理员不需要（也不应触发）GitHub 请求：他们无法安装
		if admin {
			content, err := fetchWithProxy(publishersIndexURL)
			if err != nil {
				if strings.Contains(err.Error(), "HTTP 404") {
					resp.Error = "仓库尚未发布适配器清单（main 分支上还没有 publishers/index.js）"
				} else {
					resp.Error = fmt.Sprintf("获取适配器清单失败：%v", err)
				}
			} else if list, perr := parsePublishersIndex(content); perr != nil {
				resp.Error = fmt.Sprintf("适配器清单无效：%v", perr)
			} else {
				resp.Available = list
			}
		}
		writeJSON(w, http.StatusOK, resp)

	case http.MethodPost:
		if !admin {
			writeErr(w, http.StatusForbidden, "仅管理员可安装发布适配器")
			return
		}
		// 安装适配器：下载仓库文件 → 校验 → 落盘 → 登记
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
			if strings.Contains(err.Error(), "HTTP 404") {
				writeErr(w, http.StatusNotFound, "仓库适配器清单中没有该项（可能尚未同步，或 ID 不正确）")
			} else {
				writeErr(w, http.StatusBadGateway, fmt.Sprintf("下载适配器失败：%v", err))
			}
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
		a.publishersMu.Lock()
		defer a.publishersMu.Unlock()
		if err := os.MkdirAll(a.publishersDir(), 0o755); err != nil {
			writeErr(w, http.StatusInternalServerError, "无法创建适配器目录")
			return
		}
		if err := os.WriteFile(a.publisherPath(id), []byte(content), 0o644); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存适配器失败")
			return
		}
		meta := PublisherMeta{ID: id, Name: name, Desc: strings.TrimSpace(body.Desc), Version: version}
		if err := a.setPublisherMeta(meta); err != nil {
			writeErr(w, http.StatusInternalServerError, "登记适配器失败")
			return
		}
		writeJSON(w, http.StatusOK, meta)

	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// handlePublisherItem GET /api/publishers/{id}（元数据）/ {id}/source（源码，供前端 <script> 加载）
// PUT（保存当前用户的配置）/ DELETE（卸载，仅管理员）。
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
		data, err := os.ReadFile(a.publisherPath(id))
		if err != nil {
			writeErr(w, http.StatusNotFound, "适配器文件不存在，请管理员重新下载")
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
		for _, p := range a.installedPublishers(ua) {
			if p.ID == id {
				writeJSON(w, http.StatusOK, p)
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
		found := false
		for _, p := range a.installedPublishers(ua) {
			if p.ID == id && !p.Missing {
				found = true
				break
			}
		}
		if !found {
			writeErr(w, http.StatusBadRequest, "适配器未安装")
			return
		}
		if err := ua.setPublisherConfig(id, cfg); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存配置失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if !ua.isAdmin() {
			writeErr(w, http.StatusForbidden, "仅管理员可卸载发布适配器")
			return
		}
		a.publishersMu.Lock()
		err := a.uninstallPublisher(id)
		a.publishersMu.Unlock()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}
