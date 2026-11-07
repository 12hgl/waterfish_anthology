package main

// 格式解析回归：HTML / RTF / docx / odt / epub / PDF。
// 样本在测试内用 archive/zip 现场构造，不依赖外部文件。

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZip(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("创建样本失败: %v", err)
	}
	zw := zip.NewWriter(f)
	for n, content := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatalf("写入条目失败: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("写入内容失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	f.Close()
	return p
}

func TestHTMLToMarkdown(t *testing.T) {
	src := `<html><head><title>X</title><style>p{color:red}</style></head><body>
<h1>标题一</h1>
<p>这是<strong>加粗</strong>与<em>斜体</em>，还有<code>code</code>。</p>
<ul><li>第一项</li><li>第二项</li></ul>
<blockquote>引用一句话</blockquote>
<pre><code>line1
line2</code></pre>
<table><tr><th>姓名</th><th>年龄</th></tr><tr><td>甲</td><td>18</td></tr></table>
<p><a href="https://example.com">链接</a> <img src="a.png" alt="图"></p>
</body></html>`
	md := htmlToMarkdown(src)
	for _, want := range []string{"# 标题一", "**加粗**", "*斜体*", "`code`", "- 第一项",
		"> 引用一句话", "line1", "| 姓名 | 年龄 |", "[链接](https://example.com)", "![图](a.png)"} {
		if !strings.Contains(md, want) {
			t.Errorf("HTML 转换缺少 %q\n---\n%s", want, md)
		}
	}
	if strings.Contains(md, "color:red") {
		t.Error("HTML 转换未剔除 style 内容")
	}
}

func TestRTFToMarkdown(t *testing.T) {
	// \u20013? = 中，\u25991? = 文
	src := `{\rtf1\ansi\ansicpg936{\fonttbl{\f0 SimSun;}}
{\*\generator Riched20;}
\pard 这是\u20013?\u25991?测试\par 第二\u27573?\par}`
	md, _ := rtfToMarkdown(src)
	if !strings.Contains(md, "这是中文测试") {
		t.Errorf("RTF \\u 转义解析失败: %q", md)
	}
	if !strings.Contains(md, "第二段") {
		t.Errorf("RTF 段落分隔失败: %q", md)
	}
	if strings.Contains(md, "SimSun") || strings.Contains(md, "generator") {
		t.Errorf("RTF 未跳过字体表与生成器信息: %q", md)
	}
}

func TestDocxConvert(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>文档标题</w:t></w:r></w:p>
<w:p><w:r><w:t>普通段落</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>加粗部分</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/></w:numPr></w:pPr><w:r><w:t>列表项</w:t></w:r></w:p>
<w:tbl>
<w:tr><w:tc><w:p><w:r><w:t>列A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>列B</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc></w:tr>
</w:tbl>
</w:body></w:document>`
	p := writeZip(t, "t.docx", map[string]string{"word/document.xml": doc})
	res, err := docxConvert(p)
	if err != nil {
		t.Fatalf("docx 解析失败: %v", err)
	}
	for _, want := range []string{"# 文档标题", "普通段落", "**加粗部分**", "- 列表项", "| 列A | 列B |"} {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("docx 转换缺少 %q\n---\n%s", want, res.Markdown)
		}
	}
	if res.Title != "文档标题" {
		t.Errorf("docx 标题识别错误: %q", res.Title)
	}
}

func TestODTConvert(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"><office:body><office:text>
<text:h text:outline-level="1">ODT 标题</text:h>
<text:p>第一段<text:span>内容</text:span></text:p>
<text:list><text:list-item><text:p>列表一</text:p></text:list-item></text:list>
<table:table>
<table:table-row><table:table-cell><text:p>A</text:p></table:table-cell><table:table-cell><text:p>B</text:p></table:table-cell></table:table-row>
</table:table>
</office:text></office:body></office:document-content>`
	p := writeZip(t, "t.odt", map[string]string{"content.xml": content})
	res, err := odtConvert(p)
	if err != nil {
		t.Fatalf("odt 解析失败: %v", err)
	}
	for _, want := range []string{"# ODT 标题", "第一段内容", "- 列表一", "| A | B |"} {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("odt 转换缺少 %q\n---\n%s", want, res.Markdown)
		}
	}
}

func TestEPUBConvert(t *testing.T) {
	container := `<?xml version="1.0"?><container><rootfiles>
<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	opf := `<?xml version="1.0"?><package xmlns:dc="http://purl.org/dc/elements/1.1/">
<metadata><dc:title>电子书标题</dc:title></metadata>
<manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="c1"/></spine></package>`
	ch1 := `<html><body><h1>第一章</h1><p>正文<strong>重点</strong></p></body></html>`
	p := writeZip(t, "t.epub", map[string]string{
		"META-INF/container.xml": container,
		"OEBPS/content.opf":      opf,
		"OEBPS/c1.xhtml":         ch1,
	})
	res, err := epubConvert(p)
	if err != nil {
		t.Fatalf("epub 解析失败: %v", err)
	}
	if !strings.Contains(res.Markdown, "# 第一章") || !strings.Contains(res.Markdown, "**重点**") {
		t.Errorf("epub 正文解析异常:\n%s", res.Markdown)
	}
	if res.Title != "电子书标题" {
		t.Errorf("epub 书名解析异常: %q", res.Title)
	}
}

func TestUnsupportedFormats(t *testing.T) {
	for _, ext := range []string{".doc", ".pages"} {
		f, ok := formatOf("x" + ext)
		if !ok || f.Mode != modeBinary {
			t.Fatalf("%s 应标记为不可解析格式", ext)
		}
		if _, err := convertFile("x"+ext, f); err == nil {
			t.Errorf("%s 应返回明确的不可解析提示", ext)
		}
	}
	for _, ext := range []string{".md", ".txt", ".tex"} {
		f, ok := formatOf("x" + ext)
		if !ok || f.Mode != modeEditable {
			t.Errorf("%s 应可编辑写回", ext)
		}
	}
}

func TestSupportedFormatList(t *testing.T) {
	list := supportedFormats()
	if len(list) < 12 {
		t.Fatalf("支持格式数量异常: %d", len(list))
	}
	seen := map[string]bool{}
	for _, f := range list {
		if seen[f.Ext] {
			t.Errorf("格式重复: %s", f.Ext)
		}
		seen[f.Ext] = true
	}
	for _, need := range []string{".md", ".txt", ".docx", ".pdf", ".epub", ".rtf", ".html", ".tex", ".odt", ".pages", ".doc", ".markdown"} {
		if !seen[need] {
			t.Errorf("缺少格式: %s", need)
		}
	}
}

func TestPDFExtract(t *testing.T) {
	// 构造最小 PDF：未压缩内容流 + 标准字体，验证内容流解析与文字抽取
	stream := "BT /F1 12 Tf 72 720 Td (Hello PDF) Tj 0 -20 Td (Second line) Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	for i, o := range objs {
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	b.WriteString("trailer\n<< /Root 1 0 R >>\n%%EOF\n")

	text, err := pdfExtractText([]byte(b.String()))
	if err != nil {
		t.Fatalf("PDF 提取失败: %v", err)
	}
	for _, want := range []string{"Hello PDF", "Second line"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF 文本缺少 %q：%q", want, text)
		}
	}
	if strings.Count(text, "\n") < 1 {
		t.Errorf("PDF 换行未还原：%q", text)
	}
}
