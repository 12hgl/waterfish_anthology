package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveExternalPath(t *testing.T) {
	bad := []string{"", "  ", "relative/a.md", "a.md", "/tmp/../etc/passwd\x00"}
	for _, p := range bad {
		if _, err := resolveExternalPath(p); err == nil {
			t.Errorf("应拒绝路径 %q", p)
		}
	}
	ok := []string{"/vol1/1000/a.md", "/tmp/wf/shared/notes/a.md"}
	for _, p := range ok {
		abs, err := resolveExternalPath(p)
		if err != nil || abs != filepath.Clean(p) {
			t.Errorf("应接受路径 %q，得到 %q / %v", p, abs, err)
		}
	}
	// 相对路径片段被 Clean 收拢
	if abs, err := resolveExternalPath("/tmp/wf/shared/../shared/x.md"); err != nil || abs != "/tmp/wf/shared/x.md" {
		t.Errorf("Clean 失败：%q %v", abs, err)
	}
}

func TestSplitFrontMatter(t *testing.T) {
	head, body, ok := splitFrontMatter("---\ntitle: 甲\ntags: [a, b]\n---\n\n正文")
	if !ok || !strings.Contains(head, "tags") || body != "正文" {
		t.Fatalf("解析失败：%q / %q / %v", head, body, ok)
	}
	if _, _, ok := splitFrontMatter("# 只有正文"); ok {
		t.Fatal("无 frontmatter 时不应报告 ok")
	}
	// 未闭合的 frontmatter 视为普通正文，避免吞掉用户内容
	_, body, ok = splitFrontMatter("---\ntitle: 甲\n正文")
	if ok || !strings.Contains(body, "正文") {
		t.Fatalf("未闭合 frontmatter 处理错误：%q %v", body, ok)
	}
}

func TestMergeFrontMatter(t *testing.T) {
	orig := "---\ntitle: 旧标题\ntags: [随笔, 技术]\ndate: 2026-01-01\n---\n\n旧正文\n"

	got, ok := mergeFrontMatter(orig, "新标题", "新正文")
	if !ok {
		t.Fatal("应识别到 frontmatter")
	}
	if !strings.Contains(got, "title: 新标题") {
		t.Error("标题未更新")
	}
	// 用户自定义元信息必须原样保留
	for _, want := range []string{"tags: [随笔, 技术]", "date: 2026-01-01"} {
		if !strings.Contains(got, want) {
			t.Errorf("丢失元信息 %q\n%s", want, got)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "新正文") {
		t.Error("正文未替换")
	}
	if strings.Contains(got, "旧正文") || strings.Contains(got, "旧标题") {
		t.Error("旧内容残留")
	}

	// 标题为空：保留原 title
	got, _ = mergeFrontMatter(orig, "", "改过的正文")
	if !strings.Contains(got, "title: 旧标题") {
		t.Error("空标题不应清掉原 title")
	}

	// 无 frontmatter：不擅自添加
	got, ok = mergeFrontMatter("# 标题\n正文", "新标题", "新正文")
	if ok || strings.Contains(got, "---") || got != "新正文" {
		t.Errorf("无 frontmatter 时应原样输出正文，得到 %q", got)
	}

	// 有 frontmatter 但没有 title 行：补一行
	got, _ = mergeFrontMatter("---\ntags: [a]\n---\n正文", "补标题", "正文2")
	if !strings.Contains(got, "title: 补标题") || !strings.Contains(got, "tags: [a]") {
		t.Errorf("应补充 title 且保留 tags：%q", got)
	}
}

func TestExternalExts(t *testing.T) {
	// 可编辑：纯文本类，写完直接回写原文件
	for _, ext := range []string{".md", ".markdown", ".mdown", ".mdx", ".txt", ".tex", ".MD"} {
		f, ok := formatOf("x" + strings.ToLower(ext))
		if !ok || f.Mode != modeEditable {
			t.Errorf("%s 应可编辑写回（得到 %q）", ext, f.Mode)
		}
	}
	// 只读预览：含版式或二进制容器，解析成 Markdown 后另存为文章
	for _, ext := range []string{".docx", ".odt", ".epub", ".pdf", ".rtf", ".html", ".htm"} {
		f, ok := formatOf("x" + ext)
		if !ok || f.Mode != modeReadonly {
			t.Errorf("%s 应为只读预览（得到 %q）", ext, f.Mode)
		}
	}
	// 闭源二进制与新格式外的一律拒绝
	for _, ext := range []string{".doc", ".pages"} {
		f, ok := formatOf("x" + ext)
		if !ok || f.Mode != modeBinary {
			t.Errorf("%s 应标记为不可解析", ext)
		}
	}
	for _, ext := range []string{".png", ".exe", ""} {
		if _, ok := formatOf("x" + ext); ok {
			t.Errorf("%s 不应被支持", ext)
		}
	}
}

// TestExternalFlow 用临时目录模拟「已授权目录」的完整读写链路。
func TestExternalFlow(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("---\ntitle: 甲\ntags: [x]\n---\n\n原始正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ua := &UserApp{App: &App{VarDir: root, AuthPaths: []string{root}}}
	ua.user = &User{Key: "tester", Name: "tester"}

	got, err := ua.openExternal(file)
	if err != nil {
		t.Fatalf("打开失败：%v", err)
	}
	if got.Title != "甲" || strings.TrimSpace(got.Content) != "原始正文" || !got.HasMeta {
		t.Fatalf("解析结果异常：%+v", got)
	}

	if _, err := ua.saveExternal(file, "乙", "改后正文"); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	raw, _ := os.ReadFile(file)
	text := string(raw)
	for _, want := range []string{"title: 乙", "tags: [x]", "改后正文"} {
		if !strings.Contains(text, want) {
			t.Errorf("写回内容缺少 %q：%s", want, text)
		}
	}

	// 未授权目录必须被拦下
	outside := filepath.Join(t.TempDir(), "b.md")
	_ = os.WriteFile(outside, []byte("x"), 0o644)
	if _, err := ua.openExternal(outside); err == nil {
		t.Error("未授权路径应被拒绝")
	}

	// 扫描 + 导入
	_, list, err := ua.scanExternalDir(dir)
	if err != nil || len(list) != 1 {
		t.Fatalf("扫描失败：%v %d", err, len(list))
	}
	imported, skipped := ua.importExternal([]string{file})
	if len(imported) != 1 || len(skipped) != 0 {
		t.Fatalf("导入失败：%v / %v", imported, skipped)
	}
	saved, err := ua.getArticle(imported[0].ID)
	if err != nil || !strings.Contains(saved.Content, "改后正文") {
		t.Fatalf("导入内容异常：%v %q", err, saved.Content)
	}
	if !strings.Contains(string(mustRead(t, ua.articlePathOrFail(imported[0].ID))), "tags: [x]") {
		t.Error("导入应保留原 frontmatter")
	}
}

func (u *UserApp) articlePathOrFail(id string) string {
	p, err := u.articlePath(id)
	if err != nil {
		panic(err)
	}
	return p
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
