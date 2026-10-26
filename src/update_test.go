package main

import (
	"os"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.0", "1.0.0", 0},
		{"v1.2.0", "1.1.9", 1},
		{"1.10.0", "1.9.0", 1},        // 数值比较，非字符串
		{"1.0.1-beta", "1.0.0", 1},    // 预发布标签忽略
		{"1.0", "1.0.0", 0},           // 段数不齐补 0
		{"1.0.1", "1.0.1+build", 0},   // 构建标签忽略
		{"2.0.0", "10.0.0", -1},       // 两位数
		{"1.0.0", "not-a-version", 1}, // 无法解析的远端版本按 0 计，不会误报更新
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseReleaseJS(t *testing.T) {
	content := `/* 注释 */
window.waterfishRelease = {
  "version": "1.0.1",
  "date": "2026-10-25",
  "notes": "第一行 {花括号} 与 \"引号\"\n第二行",
  "x86": "https://example.com/x86.fpk",
  "arm": "https://example.com/arm.fpk",
  "release": "https://example.com/release"
};
`
	info, err := parseReleaseJS(content)
	if err != nil {
		t.Fatalf("parseReleaseJS 失败: %v", err)
	}
	if info.Version != "1.0.1" || info.Date != "2026-10-25" {
		t.Errorf("版本/日期解析错误: %+v", info)
	}
	if info.Notes == "" || info.X86 == "" || info.ARM == "" || info.Release == "" {
		t.Errorf("字段缺失: %+v", info)
	}
	if want := "第一行 {花括号} 与 \"引号\"\n第二行"; info.Notes != want {
		t.Errorf("notes 解析错误: %q", info.Notes)
	}

	// 无标记
	if _, err := parseReleaseJS("nothing here"); err == nil {
		t.Error("应报错：未找到版本信息")
	}
	// 不完整
	if _, err := parseReleaseJS("window.waterfishRelease = {\"version\":\"1.0.0\""); err == nil {
		t.Error("应报错：版本信息不完整")
	}
	// 空版本号
	if _, err := parseReleaseJS(`window.waterfishRelease = {"version":""};`); err == nil {
		t.Error("应报错：版本号为空")
	}
}

func TestParsePublishersIndex(t *testing.T) {
	content := `window.waterfishPublishers = [
  {"id":"notionnext","name":"NotionNext","desc":"发布到 NotionNext","file":"notionnext.js","version":"1.0.0"},
  {"id":"other","name":"Other","desc":"","file":"other.js","version":"2.0.0"}
];`
	list, err := parsePublishersIndex(content)
	if err != nil {
		t.Fatalf("parsePublishersIndex 失败: %v", err)
	}
	if len(list) != 2 || list[0].ID != "notionnext" || list[1].File != "other.js" {
		t.Fatalf("清单解析错误: %+v", list)
	}
	if _, err := parsePublishersIndex("window.waterfishPublishers = 123"); err == nil {
		t.Error("应报错：清单格式不正确")
	}
	if _, err := parsePublishersIndex("no marker"); err == nil {
		t.Error("应报错：未找到清单")
	}
}

func TestExtractPublisherMeta(t *testing.T) {
	content := `window.WFPublishers.notionnext = {
  id: 'notionnext',
  name: 'NotionNext',
  version: '1.0.0',
  configFields: [],
  publish: async function () {},
};`
	name, version := extractPublisherMeta(content)
	if name != "NotionNext" || version != "1.0.0" {
		t.Errorf("元数据解析错误: name=%q version=%q", name, version)
	}
	// 注册体含嵌套花括号（configFields 数组中的对象）时仍能定位到最外层
	nested := `window.WFPublishers.x = { name: "X", version: "2.1.0",
  configFields: [{ key: "token", label: "令牌" }] };`
	name, version = extractPublisherMeta(nested)
	if name != "X" || version != "2.1.0" {
		t.Errorf("嵌套元数据解析错误: name=%q version=%q", name, version)
	}
}

// TestRealPublisherFiles 用仓库里的真实适配器文件验证解析链路
// （CI 之外本地跑：go test 的工作目录是 src/）。
func TestRealPublisherFiles(t *testing.T) {
	idxData, err := os.ReadFile("../publishers/index.js")
	if err != nil {
		t.Skip("仓库文件不在预期位置")
	}
	list, err := parsePublishersIndex(string(idxData))
	if err != nil {
		t.Fatalf("index.js 解析失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != "notionnext" || list[0].File != "notionnext.js" {
		t.Fatalf("index.js 内容异常: %+v", list)
	}

	adapterData, err := os.ReadFile("../publishers/notionnext.js")
	if err != nil {
		t.Skip("适配器文件不在预期位置")
	}
	content := string(adapterData)
	if !containsAll(content, "window.WFPublishers", "publish") {
		t.Fatal("适配器缺少契约标记")
	}
	name, version := extractPublisherMeta(content)
	if name != "NotionNext" || version != "1.0.0" {
		t.Errorf("适配器元数据解析错误: name=%q version=%q", name, version)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestSanitizePublisherID(t *testing.T) {
	ok := []string{"notionnext", "my-adapter", "a_b.c"}
	for _, id := range ok {
		if got, err := sanitizePublisherID(id); err != nil || got != id {
			t.Errorf("sanitizePublisherID(%q) = %q, %v", id, got, err)
		}
	}
	bad := []string{"", "../etc/passwd", "a/b", "a b", "适配器", "x" + string(make([]byte, 0)) + "toolong" + string(make([]byte, 80))}
	for _, id := range bad {
		if _, err := sanitizePublisherID(id); err == nil {
			t.Errorf("sanitizePublisherID(%q) 应报错", id)
		}
	}
}
