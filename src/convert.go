package main

// 文件格式支持：把「打开方式」交给本应用的文档按可编辑程度分三档，
//   editable —— 纯文本，编辑器里改完直接写回原文件
//   readonly —— 含版式或二进制容器：只读预览（渲染正文），可另存为文集文章
//   binary   —— 闭源二进制（.doc / .pages），无法解析正文，只提示导出
//
// 解析全部基于标准库（archive/zip + encoding/xml + compress/zlib），
// 不引入第三方依赖，保证 NAS 端离线可构建。

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	modeEditable = "editable"
	modeReadonly = "readonly"
	modeBinary   = "binary"

	maxConvertSource = 64 << 20 // 单个文档读取上限 64 MiB
	maxConvertOutput = 8 << 20  // 解压 / 转换后的正文上限 8 MiB（防 zip 炸弹）
)

// fileFormat 扩展名对应的解析方式。
type fileFormat struct {
	Kind  string `json:"kind"`
	Mode  string `json:"mode"`
	Label string `json:"label"`
}

var formatTable = map[string]fileFormat{
	".md":       {"markdown", modeEditable, "Markdown"},
	".markdown": {"markdown", modeEditable, "Markdown"},
	".mdown":    {"markdown", modeEditable, "Markdown"},
	".mdx":      {"markdown", modeEditable, "MDX"},
	".txt":      {"text", modeEditable, "纯文本"},
	".tex":      {"latex", modeEditable, "LaTeX"},
	".html":     {"html", modeReadonly, "网页"},
	".htm":      {"html", modeReadonly, "网页"},
	".rtf":      {"rtf", modeReadonly, "RTF 富文本"},
	".docx":     {"docx", modeReadonly, "Word 文档"},
	".doc":      {"doc", modeBinary, "Word 97 文档"},
	".odt":      {"odt", modeReadonly, "OpenDocument 文档"},
	".epub":     {"epub", modeReadonly, "EPUB 电子书"},
	".pdf":      {"pdf", modeReadonly, "PDF"},
	".pages":    {"pages", modeBinary, "Pages 文档"},
}

// formatOf 查扩展名对应的解析方式。
func formatOf(path string) (fileFormat, bool) {
	f, ok := formatTable[strings.ToLower(filepath.Ext(path))]
	return f, ok
}

// SupportedFormat 前端展示与 manifest 文件关联用的格式清单。
type SupportedFormat struct {
	Ext   string `json:"ext"`
	Kind  string `json:"kind"`
	Mode  string `json:"mode"`
	Label string `json:"label"`
}

// supportedFormats 全部支持格式（可编辑优先，其次只读，最后闭源）。
func supportedFormats() []SupportedFormat {
	exts := make([]string, 0, len(formatTable))
	for e := range formatTable {
		exts = append(exts, e)
	}
	rank := map[string]int{modeEditable: 0, modeReadonly: 1, modeBinary: 2}
	sort.Slice(exts, func(i, j int) bool {
		a, b := formatTable[exts[i]], formatTable[exts[j]]
		if rank[a.Mode] != rank[b.Mode] {
			return rank[a.Mode] < rank[b.Mode]
		}
		return exts[i] < exts[j]
	})
	out := make([]SupportedFormat, 0, len(exts))
	for _, e := range exts {
		f := formatTable[e]
		out = append(out, SupportedFormat{Ext: e, Kind: f.Kind, Mode: f.Mode, Label: f.Label})
	}
	return out
}

// ConvertResult 文档解析结果。
type ConvertResult struct {
	Markdown string   `json:"markdown"`
	Title    string   `json:"title"`
	Warnings []string `json:"warnings,omitempty"`
}

// convertFile 把磁盘上的文档解析成 Markdown（不修改原文件）。
func convertFile(path string, f fileFormat) (ConvertResult, error) {
	res := ConvertResult{}
	var err error
	switch f.Kind {
	case "markdown", "text", "latex":
		res.Markdown, err = readTextFile(path)
	case "html":
		var raw string
		raw, err = readTextFile(path)
		if err == nil {
			res.Markdown = htmlToMarkdown(raw)
		}
	case "rtf":
		var raw string
		raw, err = readTextFile(path)
		if err == nil {
			res.Markdown, res.Warnings = rtfToMarkdown(raw)
		}
	case "docx":
		res, err = docxConvert(path)
	case "odt":
		res, err = odtConvert(path)
	case "epub":
		res, err = epubConvert(path)
	case "pdf":
		res, err = pdfConvert(path)
	case "doc", "pages":
		return res, fmt.Errorf("%s 属于闭源二进制格式，无法解析正文；请先用对应软件打开并另存为 .docx 或 PDF 后再导入", f.Label)
	default:
		return res, errors.New("暂不支持该文件格式")
	}
	if err != nil {
		return res, err
	}

	res.Markdown = strings.TrimSpace(strings.ReplaceAll(res.Markdown, "\r\n", "\n"))
	if res.Markdown == "" {
		return res, errors.New("没有解析到正文内容，文件可能是空的或受保护")
	}
	if res.Title == "" {
		res.Title = titleFromMarkdown(res.Markdown)
	}
	if res.Title == "" {
		res.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return res, nil
}

// readTextFile 读取文本文件（拒绝二进制与超大文件）。
func readTextFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("文件不存在或无权限读取")
	}
	if info.Size() > maxConvertSource {
		return "", errors.New("文件超过 64 MB，暂不支持打开")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errNeedGrant
	}
	if !utf8.Valid(raw) {
		return "", errors.New("文件不是 UTF-8 编码，无法解析；请另存为 UTF-8 后重试")
	}
	return string(raw), nil
}

// titleFromMarkdown 取正文首个一级标题作为文档标题。
func titleFromMarkdown(md string) string {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if t, ok := strings.CutPrefix(line, "# "); ok {
			if t = strings.TrimSpace(t); t != "" {
				return t
			}
		}
	}
	return ""
}

// ---------- HTML → Markdown ----------

var (
	reDropBlocks = regexp.MustCompile(`(?is)<(?:script|style|head|noscript|svg|iframe)\b[^>]*>.*?</(?:script|style|head|noscript|svg|iframe)\s*>`)
	reComment    = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHeading    = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]\s*>`)
	rePara       = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p\s*>`)
	reListItem   = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li\s*>`)
	reQuote      = regexp.MustCompile(`(?is)<blockquote\b[^>]*>(.*?)</blockquote\s*>`)
	rePre        = regexp.MustCompile(`(?is)<pre\b[^>]*>(.*?)</pre\s*>`)
	reTable      = regexp.MustCompile(`(?is)<table\b[^>]*>(.*?)</table\s*>`)
	reRow        = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr\s*>`)
	reCell       = regexp.MustCompile(`(?is)<t[hd]\b[^>]*>(.*?)</t[hd]\s*>`)
	reStrong     = regexp.MustCompile(`(?is)<(?:strong|b)\b[^>]*>(.*?)</(?:strong|b)\s*>`)
	reEm         = regexp.MustCompile(`(?is)<(?:em|i)\b[^>]*>(.*?)</(?:em|i)\s*>`)
	reCodeInline = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code\s*>`)
	reLink       = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*["']([^"']*)["'][^>]*>(.*?)</a\s*>`)
	reImg        = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	reImgSrc     = regexp.MustCompile(`(?is)src\s*=\s*["']([^"']*)["']`)
	reImgAlt     = regexp.MustCompile(`(?is)alt\s*=\s*["']([^"']*)["']`)
	reBr         = regexp.MustCompile(`(?is)<br\s*/?>`)
	reHR         = regexp.MustCompile(`(?is)<hr\b[^>]*>`)
	reHTMLTag    = regexp.MustCompile(`(?is)</?[a-z][^>]*>`)
	reBlankRun   = regexp.MustCompile(`\n{3,}`)
	reMultiSpace = regexp.MustCompile(`[ \t]{2,}`)
)

// htmlToMarkdown 把 HTML 片段转成 Markdown。
// 解析策略是「块级先转、再处理内联」的正则流水线：
// 目标是把正文结构（标题 / 段落 / 列表 / 引用 / 代码 / 表格 / 链接 / 图片）保留下来，
// 不做完整 HTML5 解析（不需要，也避免引入第三方解析器）。
func htmlToMarkdown(src string) string {
	s := strings.ReplaceAll(src, "\r\n", "\n")
	s = reComment.ReplaceAllString(s, "")
	s = reDropBlocks.ReplaceAllString(s, "")

	// 表格先行：单元格内容此时还带内联标签，后面统一处理
	s = reTable.ReplaceAllStringFunc(s, func(tbl string) string {
		rows := reRow.FindAllStringSubmatch(tbl, -1)
		if len(rows) == 0 {
			return ""
		}
		var lines []string
		for i, r := range rows {
			cells := reCell.FindAllStringSubmatch(r[1], -1)
			vals := make([]string, 0, len(cells))
			for _, c := range cells {
				vals = append(vals, strings.TrimSpace(c[1]))
			}
			if len(vals) == 0 {
				continue
			}
			lines = append(lines, "| "+strings.Join(vals, " | ")+" |")
			if i == 0 {
				sep := make([]string, len(vals))
				for j := range sep {
					sep[j] = "---"
				}
				lines = append(lines, "| "+strings.Join(sep, " | ")+" |")
			}
		}
		return "\n\n" + strings.Join(lines, "\n") + "\n\n"
	})

	s = rePre.ReplaceAllStringFunc(s, func(pre string) string {
		inner := reHTMLTag.ReplaceAllString(pre, "")
		inner = html.UnescapeString(inner)
		inner = strings.Trim(inner, "\n")
		return "\n\n```\n" + inner + "\n```\n\n"
	})
	s = reHeading.ReplaceAllStringFunc(s, func(h string) string {
		m := reHeading.FindStringSubmatch(h)
		if m == nil {
			return h
		}
		level, _ := strconv.Atoi(m[1])
		return "\n\n" + strings.Repeat("#", level) + " " + strings.TrimSpace(m[2]) + "\n\n"
	})
	s = rePara.ReplaceAllString(s, "\n\n$1\n\n")
	s = reQuote.ReplaceAllStringFunc(s, func(q string) string {
		m := reQuote.FindStringSubmatch(q)
		if m == nil {
			return q
		}
		lines := strings.Split(strings.TrimSpace(m[1]), "\n")
		for i, l := range lines {
			lines[i] = "> " + strings.TrimSpace(l)
		}
		return "\n\n" + strings.Join(lines, "\n") + "\n\n"
	})
	s = reListItem.ReplaceAllString(s, "\n- $1")
	s = reStrong.ReplaceAllString(s, "**$1**")
	s = reEm.ReplaceAllString(s, "*$1*")
	s = reCodeInline.ReplaceAllString(s, "`$1`")
	s = reLink.ReplaceAllString(s, "[$2]($1)")
	s = reImg.ReplaceAllStringFunc(s, func(img string) string {
		src, alt := "", ""
		if m := reImgSrc.FindStringSubmatch(img); m != nil {
			src = html.UnescapeString(m[1])
		}
		if m := reImgAlt.FindStringSubmatch(img); m != nil {
			alt = html.UnescapeString(m[1])
		}
		if src == "" {
			return ""
		}
		return "![" + alt + "](" + src + ")"
	})
	s = reBr.ReplaceAllString(s, "\n")
	s = reHR.ReplaceAllString(s, "\n\n---\n\n")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	return tidyMarkdown(s)
}

// tidyMarkdown 收尾：统一换行与空白，避免转换产物里出现大片空行。
func tidyMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = reMultiSpace.ReplaceAllString(s, " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	s = strings.Join(lines, "\n")
	s = reBlankRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// ---------- RTF → Markdown ----------

// rtfSkipDest 这些目标组里的内容不是正文，直接跳过。
var rtfSkipDest = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true,
	"pict": true, "object": true, "header": true, "footer": true,
	"headerl": true, "headerr": true, "footerl": true, "footerr": true,
	"listtable": true, "listoverridetable": true, "rsidtbl": true,
	"generator": true, "xmlnstbl": true, "themedata": true, "datastore": true,
	"latentstyles": true, "listtext": true, "filetbl": true, "revtbl": true,
	"fldinst": true, "bkmkstart": true, "bkmkend": true,
}

// rtfToMarkdown 解析 RTF 控制字，抽出正文。
// RTF 里的中文通常以 \uN? 形式存储（Word 的默认做法），据此还原；
// \'hh 按 latin-1 解释，纯西文文档正确，老式 GBK 内码文档可能乱码，此时给出提示。
func rtfToMarkdown(src string) (string, []string) {
	var b strings.Builder
	var warnings []string
	stack := []bool{false} // true = 当前组被跳过
	skip := func() bool { return stack[len(stack)-1] }
	cp := 0      // 当前 codepage（仅用于提示）
	ucSkip := 1  // \uN 之后要跳过的字节数
	pending := 0 // 待跳过的字符数
	latinLost := false
	paraBreak := func() {
		s := b.String()
		if !strings.HasSuffix(s, "\n") {
			b.WriteString("\n")
		}
	}
	listDepth := 0

	for i := 0; i < len(src); i++ {
		c := src[i]
		switch c {
		case '{':
			stack = append(stack, skip())
		case '}':
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case '\\':
			if i+1 >= len(src) {
				continue
			}
			n := src[i+1]
			// 控制符号
			if n == '\\' || n == '{' || n == '}' {
				if !skip() {
					b.WriteByte(n)
				}
				i++
				continue
			}
			if n == '\'' {
				if i+3 < len(src) {
					if v, err := strconv.ParseUint(src[i+2:i+4], 16, 8); err == nil {
						if !skip() {
							if v < 0x80 {
								b.WriteByte(byte(v))
							} else {
								latinLost = true
								b.WriteRune(rune(v))
							}
						}
					}
					i += 3
				}
				continue
			}
			if n == '*' {
				// \* 标记「可忽略目标组」（如 {\*\generator ...}），整组内容不输出
				stack[len(stack)-1] = true
				i++
				continue
			}
			if n == '~' {
				if !skip() {
					b.WriteString(" ")
				}
				i++
				continue
			}
			// 控制字：字母序列 + 可选数字参数
			j := i + 1
			for j < len(src) && ((src[j] >= 'a' && src[j] <= 'z') || (src[j] >= 'A' && src[j] <= 'Z')) {
				j++
			}
			word := src[i+1 : j]
			numStart := j
			if j < len(src) && (src[j] == '-' || (src[j] >= '0' && src[j] <= '9')) {
				j++
				for j < len(src) && src[j] >= '0' && src[j] <= '9' {
					j++
				}
			}
			param, _ := strconv.Atoi(src[numStart:j])
			if j < len(src) && src[j] == ' ' {
				j++ // 控制字后单个空格是分隔符，不属于正文
			}
			i = j - 1

			if rtfSkipDest[word] {
				stack[len(stack)-1] = true
				continue
			}
			if skip() {
				continue
			}
			switch word {
			case "par", "line", "sect", "page":
				paraBreak()
				if word == "par" {
					b.WriteString("\n")
				}
			case "tab":
				b.WriteString("\t")
			case "emdash":
				b.WriteString("—")
			case "endash":
				b.WriteString("–")
			case "lquote":
				b.WriteString("‘")
			case "rquote":
				b.WriteString("’")
			case "ldblquote":
				b.WriteString("“")
			case "rdblquote":
				b.WriteString("”")
			case "bullet":
				b.WriteString("- ")
			case "uc":
				if param > 0 && param <= 4 {
					ucSkip = param
				}
			case "u":
				v := param
				if v < 0 {
					v += 65536
				}
				b.WriteRune(rune(v))
				pending = ucSkip
			case "ansicpg":
				cp = param
			case "pard":
				listDepth = 0
			case "li":
				if param > 0 {
					listDepth++
				}
			}
			continue
		case '\r', '\n':
			// RTF 源码换行不代表正文换行
			continue
		default:
			if pending > 0 {
				pending--
				continue
			}
			if !skip() {
				b.WriteByte(c)
			}
		}
	}

	if latinLost && cp != 0 && cp != 1252 {
		warnings = append(warnings, fmt.Sprintf("文档使用代码页 %d 存储非 ASCII 字符，部分中文可能显示异常；建议用 Word 另存为 .docx 后导入", cp))
	}
	return tidyMarkdown(b.String()), warnings
}

// ---------- zip 辅助 ----------

// zipOpen 打开 Office / EPUB 容器（限制解压体积，防 zip 炸弹）。
func zipOpen(path string) (*zip.ReadCloser, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.New("文件损坏或不是有效的 Office / EPUB 文档")
	}
	return zr, nil
}

// zipEntryText 读取容器内某个条目的文本内容。
func zipEntryText(zr *zip.Reader, name string) (string, error) {
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		raw, err := io.ReadAll(io.LimitReader(rc, maxConvertOutput))
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	return "", fmt.Errorf("文档内缺少 %s", name)
}

// zipFind 按后缀查找容器内第一个匹配条目。
func zipFind(zr *zip.Reader, suffix string) string {
	suffix = strings.ToLower(suffix)
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), suffix) {
			return f.Name
		}
	}
	return ""
}

// xmlDecode 逐 token 解析 XML 文档（Office / EPUB 容器内均为良构 XML，不做 HTML 猜闭合）。
func xmlDecode(text string, fn func(t xml.Token) error) error {
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = false
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "utf-8") || charset == "" {
			return input, nil
		}
		// 其余编码（如 windows-1252）按 latin-1 兜底，避免整篇解析失败
		raw, err := io.ReadAll(io.LimitReader(input, maxConvertOutput))
		if err != nil {
			return nil, err
		}
		out := make([]rune, 0, len(raw))
		for _, b := range raw {
			out = append(out, rune(b))
		}
		return strings.NewReader(string(out)), nil
	}
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if t == nil {
			return nil
		}
		if err := fn(t); err != nil {
			return err
		}
	}
}

// attrValue 读取 XML 属性（忽略命名空间前缀，Office 文档里 w:val 与 val 混用）。
func attrValue(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		local := a.Name.Local
		if strings.EqualFold(local, name) {
			return a.Value
		}
	}
	return ""
}

// isElem 判断 token 是否为指定本地名的起始元素。
func isElem(t xml.Token, name string) (xml.StartElement, bool) {
	if se, ok := t.(xml.StartElement); ok && strings.EqualFold(se.Name.Local, name) {
		return se, true
	}
	return xml.StartElement{}, false
}

// isEnd 判断是否为指定元素的结束标签。
func isEnd(t xml.Token, name string) bool {
	if ee, ok := t.(xml.EndElement); ok && strings.EqualFold(ee.Name.Local, name) {
		return true
	}
	return false
}

// bbuf 带缩进与段落控制的 Markdown 缓冲区。
type bbuf struct {
	b strings.Builder
}

func (w *bbuf) para(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	cur := w.b.String()
	if cur != "" && !strings.HasSuffix(cur, "\n\n") {
		w.b.WriteString("\n\n")
	}
	w.b.WriteString(s)
	w.b.WriteString("\n")
}

func (w *bbuf) String() string { return tidyMarkdown(w.b.String()) }

// bytesReader 便于测试的构造。
func bytesReader(s string) io.Reader { return bytes.NewReader([]byte(s)) }
