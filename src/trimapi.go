package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// 飞牛开放平台「后端 API」：应用服务端通过系统提供的 Unix Socket 调用，
// 用系统注入的 TRIM_API_TOKEN 认证。token 每次从环境变量读取，不落盘、不下发前端。
//
// 参考：https://developer.fnnas.com/api/calling
const (
	trimAPISocketDefault = "/var/run/trim_open_gateway_apiscope.socket"
	trimAPIEndpoint      = "http://localhost/api/v1/trimapp"
)

// errTrimAPIUnavailable 表示当前环境没有开放平台后端 API（例如局域网直连调试、
// 或系统版本过低）。调用方应降级为本地文件系统探测，而不是直接报错。
var errTrimAPIUnavailable = errors.New("当前环境不支持飞牛开放接口")

func trimAPISocket() string {
	if v := strings.TrimSpace(os.Getenv("WF_TRIM_API_SOCKET")); v != "" {
		return v
	}
	return trimAPISocketDefault
}

var trimAPIClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			d.Timeout = 3 * time.Second
			return d.DialContext(ctx, "unix", trimAPISocket())
		},
		MaxIdleConns:    4,
		IdleConnTimeout: 30 * time.Second,
	},
}

type trimAPIResp struct {
	ReqID string          `json:"reqId"`
	Code  int             `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
}

// trimAPIAvailable 判断后端 API 是否可用（socket 存在 + token 已注入）。
func trimAPIAvailable() bool {
	if strings.TrimSpace(os.Getenv("TRIM_API_TOKEN")) == "" {
		return false
	}
	if _, err := os.Stat(trimAPISocket()); err != nil {
		return false
	}
	return true
}

// trimCall 调用一个后端 API 能力。data 为 nil 时传空对象。
func trimCall(appName, req string, data any) (*trimAPIResp, error) {
	token := strings.TrimSpace(os.Getenv("TRIM_API_TOKEN"))
	if token == "" {
		return nil, errTrimAPIUnavailable
	}
	if data == nil {
		data = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{
		"reqId":   strconv.FormatInt(time.Now().UnixNano(), 10),
		"req":     req,
		"appName": appName,
		"data":    data,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, trimAPIEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := trimAPIClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w：%v", errTrimAPIUnavailable, err)
	}
	defer resp.Body.Close()

	var out trimAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("开放接口返回格式异常：%v", err)
	}
	if out.Code != 0 {
		msg := strings.TrimSpace(out.Msg)
		if msg == "" {
			msg = "未知错误"
		}
		return &out, fmt.Errorf("%s", msg)
	}
	return &out, nil
}

// ---------- 具体能力 ----------

// aclEntry 单路径权限结果。
type aclEntry struct {
	Path      string `json:"path"`
	Readable  bool   `json:"readable"`
	Writable  bool   `json:"writable"`
	Deletable bool   `json:"deletable"`
}

// checkUserACL 检查某个用户对路径的读/写/删权限。
// 路径不存在或应用无权读取时，系统会返回三项均为 false。
func checkUserACL(appName string, uid int, paths []string) ([]aclEntry, error) {
	if uid <= 0 || len(paths) == 0 {
		return nil, errors.New("缺少 uid 或路径")
	}
	var data any = map[string]any{"uid": uid, "path": paths[0]}
	if len(paths) > 1 {
		data = map[string]any{"uid": uid, "path": paths}
	}
	resp, err := trimCall(appName, "trim.file.checkUserACL", data)
	if err != nil {
		return nil, err
	}
	var out []aclEntry
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, fmt.Errorf("权限结果解析失败：%v", err)
	}
	return out, nil
}

// convertPaths 把 /vol1/... 内部路径转成用户可读的语义路径。
func convertPaths(appName string, paths []string, language string) (map[string]string, error) {
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	if language == "" {
		language = "zh-CN"
	}
	var data any = map[string]any{"path": paths[0], "language": language}
	if len(paths) > 1 {
		data = map[string]any{"path": paths, "language": language}
	}
	resp, err := trimCall(appName, "trim.file.convertPath", data)
	if err != nil {
		return nil, err
	}
	var out struct {
		Status int `json:"status"`
		Result []struct {
			Path         string `json:"path"`
			SemanticPath string `json:"semanticPath"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, err
	}
	m := make(map[string]string, len(out.Result))
	for _, r := range out.Result {
		m[r.Path] = r.SemanticPath
	}
	return m, nil
}

// getUserAccessibleFolders 查询当前用户已授权给应用的目录。
func getUserAccessibleFolders(appName string, uid int) ([]string, error) {
	if uid <= 0 {
		return nil, errors.New("缺少 uid")
	}
	resp, err := trimCall(appName, "trim.file.getUserAccessibleFolders", map[string]any{"uid": uid})
	if err != nil {
		return nil, err
	}
	var out struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, err
	}
	return out.Paths, nil
}

// friendlyTrimErr 把开放接口错误转成面向用户的提示。
func friendlyTrimErr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errTrimAPIUnavailable) {
		return "当前环境未提供飞牛开放接口（请从飞牛桌面打开应用后重试）"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "仅管理员"):
		return "该操作仅管理员可执行"
	case strings.Contains(msg, "access_denied"), strings.Contains(msg, "denied"):
		return "授权被拒绝"
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no such file"):
		return "路径不存在或已失效"
	}
	return msg
}
