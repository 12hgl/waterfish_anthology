package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testApp 构造隔离的应用实例（配置 / 数据都落在临时目录）。
func testApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TRIM_PKGETC", filepath.Join(dir, "etc"))
	t.Setenv("TRIM_PKGVAR", filepath.Join(dir, "var"))
	t.Setenv("TRIM_APPDEST", dir)
	return NewApp()
}

func asUser(t *testing.T, a *App, key string, admin bool) *UserApp {
	t.Helper()
	return a.forUser(&User{Key: key, Name: key, Source: "gateway", IsAdmin: admin})
}

// call 以指定用户身份直接调用 handler。
func call(a *App, ua *UserApp, method, target, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req = req.WithContext(context.WithValue(req.Context(), ctxUserKey, ua))
	rr := httptest.NewRecorder()
	switch {
	case strings.HasPrefix(target, "/api/publishers/"):
		a.handlePublisherItem(rr, req)
	default:
		a.handlePublishers(rr, req)
	}
	return rr
}

// seedPublisher 模拟管理员已安装的适配器（文件 + 系统登记）。
func seedPublisher(t *testing.T, a *App, id string) {
	t.Helper()
	if err := os.MkdirAll(a.publishersDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	js := "window.WFPublishers=window.WFPublishers||{};\nwindow.WFPublishers." + id + " = { name: 'Demo', version: '1.0.0', publish: function(){} };\n"
	if err := os.WriteFile(a.publisherPath(id), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.setPublisherMeta(PublisherMeta{ID: id, Name: "Demo", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
}

func TestPublishDisabledByDefaultThenToggle(t *testing.T) {
	a := testApp(t)
	ua := asUser(t, a, "alice", false)
	if !ua.PublishEnabled() {
		t.Fatal("发布功能应默认开启")
	}
	if err := ua.setPublishEnabled(false); err != nil {
		t.Fatal(err)
	}
	if ua.PublishEnabled() {
		t.Fatal("关闭后应为关闭状态")
	}
	// 重新加载配置后仍保持关闭
	ua2 := asUser(t, a, "bob", false)
	if !ua2.PublishEnabled() {
		t.Fatal("其他账号不应受影响")
	}
}

func TestPublisherInstallIsAdminOnly(t *testing.T) {
	a := testApp(t)
	user := asUser(t, a, "alice", false)
	admin := asUser(t, a, "root", true)

	if rr := call(a, user, http.MethodPost, "/api/publishers", `{"id":"demo"}`); rr.Code != http.StatusForbidden {
		t.Fatalf("非管理员安装应 403，实际 %d", rr.Code)
	}
	seedPublisher(t, a, "demo")

	if rr := call(a, user, http.MethodDelete, "/api/publishers/demo", ""); rr.Code != http.StatusForbidden {
		t.Fatalf("非管理员卸载应 403，实际 %d", rr.Code)
	}
	if _, err := os.Stat(a.publisherPath("demo")); err != nil {
		t.Fatal("非管理员不应删掉适配器文件")
	}

	// 管理员可以卸载，文件与登记一起清理
	if rr := call(a, admin, http.MethodDelete, "/api/publishers/demo", ""); rr.Code != http.StatusOK {
		t.Fatalf("管理员卸载应成功，实际 %d %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(a.publisherPath("demo")); !os.IsNotExist(err) {
		t.Fatal("卸载后适配器文件应被删除")
	}
	if len(a.PublisherRegistry()) != 0 {
		t.Fatal("卸载后登记应清空")
	}
}

func TestPublisherConfigIsolatedPerUser(t *testing.T) {
	a := testApp(t)
	seedPublisher(t, a, "demo")
	alice := asUser(t, a, "alice", false)
	bob := asUser(t, a, "bob", false)

	if rr := call(a, alice, http.MethodPut, "/api/publishers/demo", `{"config":{"token":"alice-token"}}`); rr.Code != http.StatusOK {
		t.Fatalf("保存配置应成功，实际 %d %s", rr.Code, rr.Body.String())
	}
	if got := alice.publisherCfg("demo")["token"]; got != "alice-token" {
		t.Fatalf("alice 的配置未保存：%q", got)
	}
	if got := bob.publisherCfg("demo")["token"]; got != "" {
		t.Fatalf("bob 不应看到 alice 的密钥：%q", got)
	}

	// 未安装的适配器不能写配置
	if rr := call(a, alice, http.MethodPut, "/api/publishers/nope", `{"config":{"token":"x"}}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("未安装适配器写配置应 400，实际 %d", rr.Code)
	}
}

func TestPublisherListHidesAvailableFromNonAdmin(t *testing.T) {
	a := testApp(t)
	seedPublisher(t, a, "demo")
	user := asUser(t, a, "alice", false)

	rr := call(a, user, http.MethodGet, "/api/publishers", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"demo"`) {
		t.Fatalf("列表应包含已安装适配器：%s", body)
	}
	if !strings.Contains(body, `"isAdmin":false`) {
		t.Fatalf("非管理员标记错误：%s", body)
	}
	if !strings.Contains(body, `"available":[]`) {
		t.Fatalf("非管理员不应拿到可下载清单：%s", body)
	}
}
