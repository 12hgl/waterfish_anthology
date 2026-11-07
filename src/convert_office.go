package main

// Office / EPUB 容器解析：.docx（OOXML）、.odt（OpenDocument）、.epub（电子书）。
// 三者都是 zip + XML，用标准库 archive/zip + encoding/xml 直接抽取正文结构。

import (
	"encoding/xml"
	"errors"
	"html"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	reHeadingStyle = regexp.MustCompile(`(?i)^(?:heading|标题)\s*([1-6])$`)
	reEpubTitle    = regexp.MustCompile(`(?is)<dc:title[^>]*>(.*?)</dc:title\s*>`)
)

// docxConvert 解析 .docx：按段落 / 标题 / 列表 / 表格还原 Markdown。
func docxConvert(p string) (ConvertResult, error) {
	zr, err := zipOpen(p)
	if err != nil {
		return ConvertResult{}, err
	}
	defer zr.Close()
	doc, err := zipEntryText(&zr.Reader, "word/document.xml")
	if err != nil {
		return ConvertResult{}, errors.New("不是有效的 .docx 文档（缺少 word/document.xml）")
	}

	var (
		out        bbuf
		para       strings.Builder
		runText    strings.Builder
		paraLevel  int
		paraList   bool
		inPara     bool
		inRun      bool
		inRPr      bool
		inText     bool
		inDel      bool
		runBold    bool
		runItalic  bool
		inTbl      bool
		inCell     bool
		cellText   strings.Builder
		row        []string
		rows       [][]string
		warn       []string
		titleParts []string
	)

	flushPara := func() {
		txt := strings.TrimSpace(para.String())
		if txt == "" {
			para.Reset()
			paraLevel, paraList = 0, false
			return
		}
		if paraLevel > 0 {
			out.para(strings.Repeat("#", paraLevel) + " " + txt)
			if paraLevel == 1 && len(titleParts) == 0 {
				titleParts = append(titleParts, txt)
			}
		} else if paraList {
			out.para("- " + txt)
		} else {
			out.para(txt)
		}
		para.Reset()
		paraLevel, paraList = 0, false
	}

	err = xmlDecode(doc, func(t xml.Token) error {
		switch v := t.(type) {
		case xml.StartElement:
			switch strings.ToLower(v.Name.Local) {
			case "p":
				if inCell {
					// 单元格内的段落直接并入单元格文本
					cellText.WriteString("\n")
					break
				}
				inPara = true
				if inTbl {
					// 表格内段落：同样并入单元格
					inPara = false
				}
			case "pstyle":
				val := attrValue(v, "val")
				if m := reHeadingStyle.FindStringSubmatch(strings.TrimSpace(val)); m != nil {
					lv, _ := strconv.Atoi(m[1])
					paraLevel = lv
				} else if lv, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && lv >= 1 && lv <= 6 {
					// 中文版 Word 的标题样式 id 常为 1..6
					paraLevel = lv
				}
			case "numpr":
				paraList = true
			case "r":
				inRun = true
				runBold, runItalic = false, false
				runText.Reset()
			case "rpr":
				if inRun {
					inRPr = true
				}
			case "b":
				if inRPr {
					runBold = attrValue(v, "val") != "0" && attrValue(v, "val") != "false"
				}
			case "i":
				if inRPr {
					runItalic = attrValue(v, "val") != "0" && attrValue(v, "val") != "false"
				}
			case "del":
				inDel = true
			case "t":
				inText = true
			case "tab":
				if inText || inRun {
					runText.WriteString(" ")
				}
			case "br":
				if inRun {
					runText.WriteString(" ")
				}
			case "tbl":
				flushPara()
				inTbl = true
				rows = nil
			case "tr":
				if inTbl {
					row = nil
				}
			case "tc":
				if inTbl {
					inCell = true
					cellText.Reset()
				}
			}
		case xml.CharData:
			if !inText || inDel {
				return nil
			}
			txt := string(v)
			if inCell {
				cellText.WriteString(txt)
				break
			}
			runText.WriteString(txt)
		case xml.EndElement:
			switch strings.ToLower(v.Name.Local) {
			case "t":
				inText = false
			case "rpr":
				inRPr = false
			case "del":
				inDel = false
			case "r":
				inRun = false
				if inCell {
					break
				}
				body := runText.String()
				if strings.TrimSpace(body) != "" {
					if runBold && runItalic {
						body = "***" + body + "***"
					} else if runBold {
						body = "**" + body + "**"
					} else if runItalic {
						body = "*" + body + "*"
					}
				}
				para.WriteString(body)
				runText.Reset()
			case "p":
				if inCell {
					break
				}
				if inPara && !inTbl {
					flushPara()
				}
				inPara = false
			case "tc":
				if inCell {
					inCell = false
					row = append(row, strings.TrimSpace(cellText.String()))
				}
			case "tr":
				if inTbl && len(row) > 0 {
					rows = append(rows, row)
					row = nil
				}
			case "tbl":
				if inTbl {
					inTbl = false
					out.para(tableToMarkdown(rows))
					rows = nil
				}
			}
		}
		return nil
	})
	if err != nil {
		warn = append(warn, "文档部分内容无法解析，可能有缺失")
	}
	flushPara()

	res := ConvertResult{Markdown: out.String(), Warnings: warn}
	if len(titleParts) > 0 {
		res.Title = strings.TrimSpace(titleParts[0])
	}
	return res, nil
}

// tableToMarkdown 把二维单元格转成 Markdown 表格（首行作表头）。
func tableToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	if width == 0 {
		return ""
	}
	clean := func(s string) string {
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.ReplaceAll(s, "|", "\\|")
		return strings.TrimSpace(s)
	}
	var lines []string
	for i, r := range rows {
		cells := make([]string, width)
		for j := 0; j < width; j++ {
			if j < len(r) {
				cells[j] = clean(r[j])
			}
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			sep := make([]string, width)
			for j := range sep {
				sep[j] = "---"
			}
			lines = append(lines, "| "+strings.Join(sep, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}

// odtConvert 解析 .odt：按标题 / 段落 / 列表 / 表格还原 Markdown。
func odtConvert(p string) (ConvertResult, error) {
	zr, err := zipOpen(p)
	if err != nil {
		return ConvertResult{}, err
	}
	defer zr.Close()
	body, err := zipEntryText(&zr.Reader, "content.xml")
	if err != nil {
		return ConvertResult{}, errors.New("不是有效的 .odt 文档（缺少 content.xml）")
	}

	var (
		out      bbuf
		para     strings.Builder
		level    int
		inH      bool
		inP      bool
		inList   bool
		inTbl    bool
		inCell   bool
		cellText strings.Builder
		row      []string
		rows     [][]string
		title    string
	)

	flush := func() {
		txt := strings.TrimSpace(para.String())
		para.Reset()
		if txt == "" {
			return
		}
		switch {
		case level > 0:
			out.para(strings.Repeat("#", level) + " " + txt)
			if level == 1 && title == "" {
				title = txt
			}
		case inList:
			out.para("- " + txt)
		default:
			out.para(txt)
		}
		level = 0
	}

	_ = xmlDecode(body, func(t xml.Token) error {
		switch v := t.(type) {
		case xml.StartElement:
			switch strings.ToLower(v.Name.Local) {
			case "h":
				inH = true
				level, _ = strconv.Atoi(attrValue(v, "outline-level"))
				if level < 1 || level > 6 {
					level = 2
				}
			case "p":
				if inCell {
					break
				}
				inP = true
			case "list":
				inList = true
			case "list-item":
				// 列表项内的段落由 text:p 提供
			case "table":
				flush()
				inTbl = true
				rows = nil
			case "table-row":
				if inTbl {
					row = nil
				}
			case "table-cell":
				if inTbl {
					inCell = true
					cellText.Reset()
				}
			case "s":
				n, _ := strconv.Atoi(attrValue(v, "c"))
				if n <= 0 {
					n = 1
				}
				if inCell {
					cellText.WriteString(strings.Repeat(" ", n))
				} else {
					para.WriteString(strings.Repeat(" ", n))
				}
			case "tab":
				para.WriteString(" ")
			case "line-break":
				if inCell {
					cellText.WriteString(" ")
				} else {
					para.WriteString(" ")
				}
			}
		case xml.CharData:
			if inCell {
				cellText.WriteString(string(v))
			} else if inH || inP {
				para.WriteString(string(v))
			}
		case xml.EndElement:
			switch strings.ToLower(v.Name.Local) {
			case "h":
				inH = false
				flush()
			case "p":
				if inCell {
					break
				}
				inP = false
				flush()
			case "list":
				inList = false
			case "table-cell":
				if inCell {
					inCell = false
					row = append(row, strings.TrimSpace(cellText.String()))
				}
			case "table-row":
				if inTbl && len(row) > 0 {
					rows = append(rows, row)
					row = nil
				}
			case "table":
				if inTbl {
					inTbl = false
					out.para(tableToMarkdown(rows))
					rows = nil
				}
			}
		}
		return nil
	})

	return ConvertResult{Markdown: out.String(), Title: title}, nil
}

// epubConvert 解析 .epub：按 OPF spine 顺序合并各章正文。
func epubConvert(p string) (ConvertResult, error) {
	zr, err := zipOpen(p)
	if err != nil {
		return ConvertResult{}, err
	}
	defer zr.Close()

	container, err := zipEntryText(&zr.Reader, "META-INF/container.xml")
	if err != nil {
		return ConvertResult{}, errors.New("不是有效的 EPUB（缺少 META-INF/container.xml）")
	}
	opfPath := ""
	_ = xmlDecode(container, func(t xml.Token) error {
		if se, ok := isElem(t, "rootfile"); ok {
			if fp := attrValue(se, "full-path"); fp != "" && opfPath == "" {
				opfPath = fp
			}
		}
		return nil
	})
	if opfPath == "" {
		opfPath = zipFind(&zr.Reader, ".opf")
	}
	if opfPath == "" {
		return ConvertResult{}, errors.New("EPUB 缺少 OPF 清单，无法定位正文")
	}
	opf, err := zipEntryText(&zr.Reader, opfPath)
	if err != nil {
		return ConvertResult{}, errors.New("EPUB 内容损坏")
	}

	var (
		title    string
		manifest = map[string]string{} // id → href
		spine    []string              // idref 顺序
	)
	if m := reEpubTitle.FindStringSubmatch(opf); m != nil {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	_ = xmlDecode(opf, func(t xml.Token) error {
		if se, ok := isElem(t, "item"); ok {
			id, href := attrValue(se, "id"), attrValue(se, "href")
			if id != "" && href != "" {
				manifest[id] = href
			}
			return nil
		}
		if se, ok := isElem(t, "itemref"); ok {
			if id := attrValue(se, "idref"); id != "" {
				spine = append(spine, id)
			}
		}
		return nil
	})

	dir := path.Dir(opfPath)
	if dir == "." {
		dir = ""
	}
	if len(spine) == 0 {
		// 没有 spine 时退化为按文件名排序的全部 XHTML
		for _, f := range zr.File {
			n := strings.ToLower(f.Name)
			if strings.HasSuffix(n, ".xhtml") || strings.HasSuffix(n, ".html") || strings.HasSuffix(n, ".htm") {
				spine = append(spine, "file:"+f.Name)
			}
		}
	}

	var out bbuf
	for _, id := range spine {
		href := ""
		if rest, ok := strings.CutPrefix(id, "file:"); ok {
			href = rest
		} else {
			href = manifest[id]
			if dir != "" {
				href = path.Join(dir, href)
			}
		}
		if href == "" {
			continue
		}
		chapter, err := zipEntryText(&zr.Reader, href)
		if err != nil {
			continue
		}
		md := htmlToMarkdown(chapter)
		if strings.TrimSpace(md) == "" {
			continue
		}
		out.para(md)
	}
	if strings.TrimSpace(out.String()) == "" {
		return ConvertResult{}, errors.New("没有从 EPUB 中解析到正文")
	}
	return ConvertResult{
		Markdown: out.String(),
		Title:    title,
		Warnings: []string{"电子书中的图片未提取，如需保留插图请手动插入"},
	}, nil
}

// pdfConvert 解析 PDF 文本层（见 convert_pdf.go）。
func pdfConvert(p string) (ConvertResult, error) {
	raw, err := readBinaryFile(p)
	if err != nil {
		return ConvertResult{}, err
	}
	text, err := pdfExtractText(raw)
	if err != nil {
		return ConvertResult{}, err
	}
	md := tidyMarkdown(text)
	if md == "" {
		return ConvertResult{}, errors.New("没有提取到文字：该 PDF 可能是扫描件或使用了嵌入字体子集；请在 PDF 阅读器中复制正文，或用 Word / Pages 另存为 .docx 后导入")
	}
	return ConvertResult{
		Markdown: md,
		Warnings: []string{"PDF 版式固定，提取的是纯文字，换行与排版可能与原文略有差异，建议核对后保存"},
	}, nil
}
