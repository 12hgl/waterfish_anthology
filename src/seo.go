package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	seoAPIEndpoint = "https://uapis.cn/api/seo-check"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	mobileUA       = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
)

// SEOIssue 单条 SEO 问题。
type SEOIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

// SEOEngine 单个搜索引擎的收录结果。
type SEOEngine struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Status  string `json:"status"` // indexed / not_indexed / unknown / error
	Count   int    `json:"count"`
	Message string `json:"message"`
}

// SEORecord 一次 SEO 检测记录。
type SEORecord struct {
	ID       string          `json:"id"`
	URL      string          `json:"url"`
	Host     string          `json:"host"`
	Created  string          `json:"created"`
	Updated  string          `json:"updated"`
	Score    int             `json:"score"`
	LoadTime int             `json:"loadTime"`
	Issues   []SEOIssue      `json:"issues"`
	Details  json.RawMessage `json:"details,omitempty"`
	Engines  []SEOEngine     `json:"engines"`
	Error    string          `json:"error,omitempty"`
}

// seoClient 走带 DNS 备用线路的外网客户端（见 net.go）。
var seoClient = externalClient

// ---------- 站点检测 ----------

func sanitizeURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("请输入网址")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("网址格式不正确")
	}
	if u.Hostname() == "" || !strings.Contains(u.Hostname(), ".") {
		return "", "", fmt.Errorf("网址格式不正确")
	}
	return u.String(), u.Hostname(), nil
}

func checkSite(target string) (*SEORecord, error) {
	var metrics struct {
		Score    int             `json:"score"`
		LoadTime int             `json:"loadTime"`
		Issues   []SEOIssue      `json:"issues"`
		Details  json.RawMessage `json:"details"`
		Error    string          `json:"error"`
	}
	body, _ := json.Marshal(map[string]string{"url": target})
	req, err := http.NewRequest(http.MethodPost, seoAPIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := seoClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SEO 接口请求失败：%w", friendlyNetErr(err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	if err := json.Unmarshal(raw, &metrics); err != nil {
		return nil, fmt.Errorf("SEO 接口返回异常（HTTP %d）", resp.StatusCode)
	}
	if metrics.Error != "" {
		return nil, fmt.Errorf("%s", metrics.Error)
	}
	return &SEORecord{
		Score:    metrics.Score,
		LoadTime: metrics.LoadTime,
		Issues:   metrics.Issues,
		Details:  metrics.Details,
	}, nil
}

// ---------- 收录检测 ----------

func fetchText(rawURL string) (string, error) {
	return fetchTextWith(rawURL, userAgent)
}

func fetchTextWith(rawURL, ua string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := seoClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var (
	reTag      = regexp.MustCompile(`<[^>]*>`)
	reSpaces   = regexp.MustCompile(`\s+`)
	numPattern = `([0-9][0-9,\.]*)`
)

func plain(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return reSpaces.ReplaceAllString(s, " ")
}

func parseCount(s string, patterns ...string) int {
	for _, p := range patterns {
		if m := regexp.MustCompile(p).FindStringSubmatch(s); len(m) > 1 {
			n, err := strconv.Atoi(strings.NewReplacer(",", "", ".", "").Replace(m[1]))
			if err == nil {
				return n
			}
		}
	}
	return -1
}

// blocked 判断是否命中搜索引擎的风控验证页。
// 关键词只收强信号：真实结果页的 JS 里常出现 verify 等字样，纳入会误杀正常页面。
func blocked(text string) bool {
	for _, k := range []string{"安全验证", "请输入验证码", "网络不给力", "访问过于频繁", "异常访问", "访问异常出错", "captcha", "请开启JavaScript"} {
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}

// checkBing 通过必应 RSS 检测收录（RSS 稳定且免反爬）。
func checkBing(host string) SEOEngine {
	e := SEOEngine{Key: "bing", Name: "必应 Bing"}
	q := url.QueryEscape("site:" + host)

	// 优先尝试 HTML 拿准确条数
	if html, err := fetchText("https://cn.bing.com/search?q=" + q + "&setlang=zh-CN"); err == nil && !blocked(html) {
		txt := plain(html)
		if n := parseCount(txt, `约\s*`+numPattern+`\s*条结果`, numPattern+`\s*条结果`, `About\s*`+numPattern+`\s*results`); n >= 0 {
			e.Status, e.Count = "indexed", n
			e.Message = fmt.Sprintf("已收录，约 %d 条", n)
			return e
		}
		if strings.Contains(txt, "没有与此相关的结果") || strings.Contains(txt, "There are no results") {
			e.Status, e.Count = "not_indexed", 0
			e.Message = "site: 查询无结果"
			return e
		}
	}

	xml, err := fetchText("https://cn.bing.com/search?q=" + q + "&format=rss")
	if err != nil {
		e.Status = "error"
		e.Message = "访问失败：" + err.Error()
		return e
	}
	n := strings.Count(xml, "<item>")
	if n == 0 {
		if strings.Contains(xml, "没有与此相关的结果") || strings.Contains(plain(xml), "没有找到") {
			e.Status, e.Count = "not_indexed", 0
			e.Message = "site: 查询无结果"
		} else {
			e.Status = "unknown"
			e.Message = "结果无法解析"
		}
		return e
	}
	e.Status, e.Count = "indexed", n
	e.Message = fmt.Sprintf("已收录（RSS 返回 %d 条，实际数量以站点后台为准）", n)
	return e
}

func checkBaidu(host string) SEOEngine {
	e := SEOEngine{Key: "baidu", Name: "百度"}
	html, err := fetchText("https://www.baidu.com/s?ie=utf-8&wd=" + url.QueryEscape("site:"+host))
	if err != nil {
		e.Status = "error"
		e.Message = "访问失败：" + err.Error()
		return e
	}
	if blocked(html) {
		e.Status = "unknown"
		e.Message = "百度对服务器 IP 风控严格，无法自动查询，可手动搜索 site:" + host + " 查看"
		return e
	}
	txt := plain(html)
	if n := parseCount(txt, `找到相关结果约?\s*`+numPattern+`\s*个`, `该网站共有\s*`+numPattern+`\s*个网页被收录`, numPattern+`\s*个网页被收录`); n >= 0 {
		if n == 0 {
			e.Status, e.Count = "not_indexed", 0
			e.Message = "site: 查询无结果"
			return e
		}
		e.Status, e.Count = "indexed", n
		e.Message = fmt.Sprintf("已收录，约 %d 条", n)
		return e
	}
	e.Status = "unknown"
	e.Message = "未能解析出收录条数（百度页面结构可能已调整）"
	return e
}

// check360 通过 360 移动版检测收录（PC 版对服务器 IP 风控严格，移动版可用）。
func check360(host string) SEOEngine {
	e := SEOEngine{Key: "so360", Name: "360 搜索"}
	q := url.QueryEscape("site:" + host)
	html, err := fetchTextWith("https://m.so.com/s?q="+q, mobileUA)
	if err != nil {
		e.Status = "error"
		e.Message = "访问失败：" + err.Error()
		return e
	}
	txt := plain(html)
	if n := parseCount(txt, `找到相关结果\s*`+numPattern+`\s*个`); n >= 0 {
		if n == 0 {
			e.Status, e.Count = "not_indexed", 0
			e.Message = "site: 查询无结果"
			return e
		}
		e.Status, e.Count = "indexed", n
		e.Message = fmt.Sprintf("已收录，约 %d 条", n)
		return e
	}
	if strings.Contains(txt, "很抱歉") || strings.Contains(txt, "没有找到") {
		e.Status, e.Count = "not_indexed", 0
		e.Message = "site: 查询无结果"
		return e
	}
	if blocked(html) {
		e.Status = "unknown"
		e.Message = "被 360 风控拦截，本次结果未知"
		return e
	}
	e.Status = "unknown"
	e.Message = "未能解析出收录条数（360 页面结构可能已调整）"
	return e
}

// checkSogou 通过搜狗检测收录（搜狗对连续请求限流，命中后退避重试一次）。
func checkSogou(host string) SEOEngine {
	e := SEOEngine{Key: "sogou", Name: "搜狗"}
	q := url.QueryEscape("site:" + host)
	html, err := fetchText("https://www.sogou.com/web?query=" + q)
	if err == nil && blocked(html) {
		time.Sleep(6 * time.Second)
		html, err = fetchText("https://www.sogou.com/web?query=" + q)
	}
	if err != nil {
		e.Status = "error"
		e.Message = "访问失败：" + err.Error()
		return e
	}
	txt := plain(html)
	if n := parseCount(txt, `找到约\s*`+numPattern+`\s*条`, numPattern+`\s*条相关结果`); n >= 0 {
		if n == 0 {
			e.Status, e.Count = "not_indexed", 0
			e.Message = "site: 查询无结果"
			return e
		}
		e.Status, e.Count = "indexed", n
		e.Message = fmt.Sprintf("已收录，约 %d 条", n)
		return e
	}
	if blocked(html) {
		e.Status = "unknown"
		e.Message = "被搜狗风控拦截，本次结果未知"
		return e
	}
	e.Status = "unknown"
	e.Message = "未能解析出收录条数（搜狗页面结构可能已调整）"
	return e
}

func runChecks(target, host string) *SEORecord {
	rec := &SEORecord{
		URL:     target,
		Host:    host,
		Created: nowStamp(),
		Updated: nowStamp(),
	}

	var wg sync.WaitGroup
	engines := make([]SEOEngine, 4)
	wg.Add(4)
	go func() { defer wg.Done(); engines[0] = checkBing(host) }()
	go func() { defer wg.Done(); engines[1] = checkBaidu(host) }()
	go func() { defer wg.Done(); engines[2] = check360(host) }()
	go func() { defer wg.Done(); engines[3] = checkSogou(host) }()
	wg.Wait()
	rec.Engines = engines

	if site, err := checkSite(target); err != nil {
		rec.Error = err.Error()
	} else {
		rec.Score = site.Score
		rec.LoadTime = site.LoadTime
		rec.Issues = site.Issues
		rec.Details = site.Details
	}
	if rec.Issues == nil {
		rec.Issues = []SEOIssue{}
	}
	return rec
}

// ---------- 持久化 ----------

func (u *UserApp) seoPath(id string) (string, error) {
	name, err := safeName(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(u.SEODir(), strings.TrimSuffix(name, ".json")+".json"), nil
}

func (u *UserApp) saveSEORecord(rec *SEORecord) error {
	if err := os.MkdirAll(u.SEODir(), 0o755); err != nil {
		return err
	}
	if rec.ID == "" {
		base := time.Now().Format("20060102-150405")
		id := base
		for i := 2; ; i++ {
			if _, err := os.Stat(filepath.Join(u.SEODir(), id+".json")); os.IsNotExist(err) {
				break
			}
			id = fmt.Sprintf("%s-%d", base, i)
		}
		rec.ID = id
	}
	p, err := u.seoPath(rec.ID)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(p); err == nil {
		var prev SEORecord
		if json.Unmarshal(old, &prev) == nil && prev.Created != "" {
			rec.Created = prev.Created
		}
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func (u *UserApp) loadSEORecord(id string) (*SEORecord, error) {
	p, err := u.seoPath(id)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("记录不存在")
	}
	var rec SEORecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("记录损坏")
	}
	return &rec, nil
}

func (u *UserApp) listSEORecords() []SEORecord {
	entries, err := os.ReadDir(u.SEODir())
	if err != nil {
		return []SEORecord{}
	}
	out := []SEORecord{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(u.SEODir(), e.Name()))
		if err != nil {
			continue
		}
		var rec SEORecord
		if json.Unmarshal(raw, &rec) != nil {
			continue
		}
		rec.Details = nil // 列表不返回明细，减小体积
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

// ---------- HTTP ----------

func (a *App) handleSEOList(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, ua.listSEORecords())
	case http.MethodPost:
		var in struct {
			URL string `json:"url"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		target, host, err := sanitizeURL(in.URL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		rec := runChecks(target, host)
		if err := ua.saveSEORecord(rec); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

func (a *App) handleSEO(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/seo/"), "/")
	if rest == "" {
		writeErr(w, http.StatusBadRequest, "缺少记录 ID")
		return
	}
	id, action := rest, ""
	if i := strings.LastIndex(rest, "/"); i >= 0 {
		id, action = rest[:i], rest[i+1:]
	}

	switch {
	case action == "recheck" && r.Method == http.MethodPost:
		prev, err := ua.loadSEORecord(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		rec := runChecks(prev.URL, prev.Host)
		rec.ID = id
		if err := ua.saveSEORecord(rec); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case action != "":
		writeErr(w, http.StatusNotFound, "未知操作")

	case r.Method == http.MethodGet:
		rec, err := ua.loadSEORecord(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case r.Method == http.MethodDelete:
		p, err := ua.seoPath(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := os.Remove(p); err != nil {
			writeErr(w, http.StatusInternalServerError, "删除失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}
