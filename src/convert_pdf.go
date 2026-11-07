package main

// PDF 文本层提取（纯标准库实现，不引入第三方 PDF 库）。
//
// 只做「把正文文字取出来」这一件事，按以下步骤：
//  1. 扫描 N 0 obj 建立对象表，展开 /ObjStm 压缩对象流（PDF 1.5+）；
//  2. 按 /Type /Page 取页面，解码 /Contents（FlateDecode）得到内容流；
//  3. 解析字体 /ToUnicode CMap，把字节码还原成 Unicode（中文 PDF 的关键）；
//  4. 从内容流里按 Tj / TJ / ' / " 抽取文字，按文本定位操作切分段落。
//
// 版式、表格、图片一律不做还原：PDF 没有语义结构，强行还原只会产生错误内容。

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pdfObj struct {
	num       int
	body      string // 对象体（含字典文本，不含 stream 数据）
	stream    []byte // 原始流数据（未解码）
	hasStream bool
}

type pdfDoc struct {
	objs map[int]*pdfObj
}

var (
	rePDFObj     = regexp.MustCompile(`(?s)(\d+)\s+(\d+)\s+obj\b`)
	rePDFRef     = regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	rePDFInt     = regexp.MustCompile(`/Length\s+(\d+)`)
	reToUniRef   = regexp.MustCompile(`/ToUnicode\s+(\d+)\s+\d+\s+R`)
	reFontDict   = regexp.MustCompile(`/Font\s*<<(.*?)>>`)
	reFontSub    = regexp.MustCompile(`/([A-Za-z0-9#_.+-]+)\s+(\d+)\s+\d+\s+R`)
	reBFChar     = regexp.MustCompile(`(?s)beginbfchar(.*?)endbfchar`)
	reBFRange    = regexp.MustCompile(`(?s)beginbfrange(.*?)endbfrange`)
	reHexToken   = regexp.MustCompile(`<([0-9A-Fa-f\s]+)>`)
	reNumToken   = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)
	reWSClean    = regexp.MustCompile(`[ \t]{2,}`)
	reManyBreaks = regexp.MustCompile(`\n{3,}`)
	rePageType   = regexp.MustCompile(`/Type\s*/Page\b`)
	reCodeSpace  = regexp.MustCompile(`(?s)begincodespacerange(.*?)endcodespacerange`)
	reFilterName = regexp.MustCompile(`/([A-Za-z0-9]+Decode|Fl|A85|AHx|RL)\b`)
	reBFRangeOne = regexp.MustCompile(`<\s*([0-9A-Fa-f]+)\s*>\s*<\s*([0-9A-Fa-f]+)\s*>\s*(<[0-9A-Fa-f\s]*>|\[[^\]]*\])`)
)

// pdfExtractText 抽取 PDF 正文文字。
func pdfExtractText(raw []byte) (string, error) {
	doc := parsePDF(raw)
	if len(doc.objs) == 0 {
		return "", errors.New("PDF 结构无法识别（可能已加密），无法提取正文")
	}
	doc.expandObjStreams()

	pages := doc.pageObjects()
	if len(pages) == 0 {
		// 没有页对象时退化为扫描全部流：仍可能提取到可用文字
		pages = doc.allStreamObjs()
	}

	var out []string
	for _, page := range pages {
		content := doc.pageContent(page)
		if len(content) == 0 {
			continue
		}
		fonts := doc.pageFonts(page)
		text := extractFromContent(content, fonts)
		if strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n\n"), nil
}

// ---------- 对象表 ----------

func parsePDF(raw []byte) *pdfDoc {
	doc := &pdfDoc{objs: map[int]*pdfObj{}}
	locs := rePDFObj.FindAllSubmatchIndex(raw, -1)
	for _, loc := range locs {
		num, _ := strconv.Atoi(string(raw[loc[2]:loc[3]]))
		start := loc[1]
		end := bytes.Index(raw[start:], []byte("endobj"))
		if end < 0 {
			continue
		}
		body := raw[start : start+end]
		obj := &pdfObj{num: num}
		if si := bytes.Index(body, []byte("stream")); si >= 0 {
			dict := string(body[:si])
			data := body[si+len("stream"):]
			data = bytes.TrimPrefix(data, []byte("\r\n"))
			data = bytes.TrimPrefix(data, []byte("\n"))
			data = bytes.TrimPrefix(data, []byte("\r"))
			// 优先按 /Length 截断，长度不可解析时退回 endstream
			if m := rePDFInt.FindStringSubmatch(dict); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= len(data) {
					data = data[:n]
				}
			}
			if ei := bytes.LastIndex(data, []byte("endstream")); ei >= 0 {
				data = data[:ei]
			}
			obj.body = dict
			obj.stream = data
			obj.hasStream = true
		} else {
			obj.body = string(body)
		}
		doc.objs[num] = obj
	}
	return doc
}

// expandObjStreams 展开 /ObjStm 压缩对象流里的对象（PDF 1.5+ 常见）。
func (d *pdfDoc) expandObjStreams() {
	nums := make([]int, 0, len(d.objs))
	for n := range d.objs {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		o := d.objs[n]
		if !o.hasStream || !strings.Contains(o.body, "/ObjStm") {
			continue
		}
		data, err := d.streamData(o)
		if err != nil || len(data) == 0 {
			continue
		}
		count := pdfIntKey(o.body, "/N")
		first := pdfIntKey(o.body, "/First")
		if count <= 0 || first <= 0 || first > len(data) {
			continue
		}
		header := strings.Fields(string(data[:first]))
		type pair struct{ num, off int }
		var pairs []pair
		for i := 0; i+1 < len(header); i += 2 {
			num, err1 := strconv.Atoi(header[i])
			off, err2 := strconv.Atoi(header[i+1])
			if err1 != nil || err2 != nil {
				break
			}
			pairs = append(pairs, pair{num, off})
		}
		for i, p := range pairs {
			start := first + p.off
			if start < 0 || start > len(data) {
				continue
			}
			end := len(data)
			if i+1 < len(pairs) {
				if e := first + pairs[i+1].off; e > start && e <= len(data) {
					end = e
				}
			}
			if _, exists := d.objs[p.num]; exists {
				continue // 直接定义的对象优先
			}
			d.objs[p.num] = &pdfObj{num: p.num, body: string(data[start:end])}
		}
	}
}

// streamData 按 /Filter 链依次解码流数据。
func (d *pdfDoc) streamData(o *pdfObj) ([]byte, error) {
	if !o.hasStream {
		return nil, errors.New("对象没有流")
	}
	filters := pdfFilters(o.body)
	data := o.stream
	for _, f := range filters {
		var err error
		switch f {
		case "FlateDecode", "Fl":
			data, err = inflate(data)
			if err != nil {
				data, err = inflateRaw(data)
			}
		case "ASCII85Decode", "A85":
			data, err = decodeASCII85(data)
		case "ASCIIHexDecode", "AHx":
			data = decodeASCIIHex(data)
		case "RunLengthDecode", "RL":
			data = decodeRunLength(data)
		default:
			return nil, errors.New("不支持的流编码：" + f)
		}
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// pdfFilters 解析 /Filter 链（单个名字或名字数组），保持解码顺序。
func pdfFilters(body string) []string {
	idx := strings.Index(body, "/Filter")
	if idx < 0 {
		return nil
	}
	tail := body[idx:]
	// 只取到字典结束，避免把后面别的键当成过滤器
	if end := strings.Index(tail, "/Length"); end > 0 {
		tail = tail[:end]
	}
	var out []string
	for _, m := range reFilterName.FindAllStringSubmatch(tail, -1) {
		out = append(out, m[1])
	}
	return out
}

func decodeASCII85(raw []byte) ([]byte, error) {
	var out []byte
	var group [5]byte
	n := 0
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '~':
			// 结束标记 ~>：跳出后仍需处理末尾不足 5 个字符的组
			i = len(raw)
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
		case c >= '!' && c <= 'u':
			group[n] = c - '!'
			n++
			if n == 5 {
				var v uint32
				for _, g := range group {
					v = v*85 + uint32(g)
				}
				out = append(out, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
				n = 0
			}
		}
	}
	if n > 0 {
		// 不足 5 个字符：补 'u' 后解出前 n-1 个字节
		for k := n; k < 5; k++ {
			group[k] = 84
		}
		var v uint32
		for _, g := range group {
			v = v*85 + uint32(g)
		}
		full := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
		out = append(out, full[:n-1]...)
	}
	return out, nil
}

func decodeRunLength(raw []byte) []byte {
	var out []byte
	for i := 0; i < len(raw); i++ {
		n := int(raw[i])
		if n == 128 {
			break
		}
		if n < 128 {
			for k := 0; k <= n && i+1 < len(raw); k++ {
				i++
				out = append(out, raw[i])
			}
		} else {
			if i+1 < len(raw) {
				i++
				for k := 0; k < 257-n; k++ {
					out = append(out, raw[i])
				}
			}
		}
	}
	return out
}

func inflate(raw []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, maxConvertOutput))
}

func inflateRaw(raw []byte) ([]byte, error) {
	fr := flate.NewReader(bytes.NewReader(raw))
	defer fr.Close()
	return io.ReadAll(io.LimitReader(fr, maxConvertOutput))
}

func decodeASCIIHex(raw []byte) []byte {
	clean := make([]byte, 0, len(raw))
	for _, b := range raw {
		if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') {
			clean = append(clean, b)
		} else if b == '>' {
			break
		}
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	out := make([]byte, len(clean)/2)
	for i := 0; i < len(out); i++ {
		v, _ := strconv.ParseUint(string(clean[i*2:i*2+2]), 16, 8)
		out[i] = byte(v)
	}
	return out
}

// pageObjects 取 /Type /Page 对象（按对象号升序，与常见阅读顺序一致）。
func (d *pdfDoc) pageObjects() []*pdfObj {
	var out []*pdfObj
	for _, o := range d.objs {
		if rePageType.MatchString(o.body) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].num < out[j].num })
	return out
}

// allStreamObjs 兜底：所有成功解码的流都当作候选内容流。
func (d *pdfDoc) allStreamObjs() []*pdfObj {
	var out []*pdfObj
	for _, o := range d.objs {
		if o.hasStream {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].num < out[j].num })
	return out
}

// pageContent 解码页面内容流（/Contents 可能是单个引用或引用数组）。
func (d *pdfDoc) pageContent(page *pdfObj) []byte {
	idx := strings.Index(page.body, "/Contents")
	if idx < 0 {
		if page.hasStream {
			data, err := d.streamData(page)
			if err == nil {
				return data
			}
		}
		return nil
	}
	tail := page.body[idx:]
	var refs []int
	if arrays := strings.Index(tail, "["); arrays >= 0 && arrays < 200 {
		end := strings.Index(tail, "]")
		if end > arrays {
			for _, m := range rePDFRef.FindAllStringSubmatch(tail[arrays:end], -1) {
				n, _ := strconv.Atoi(m[1])
				refs = append(refs, n)
			}
		}
	}
	if len(refs) == 0 {
		if m := rePDFRef.FindStringSubmatch(tail); m != nil {
			n, _ := strconv.Atoi(m[1])
			refs = append(refs, n)
		}
	}
	var out bytes.Buffer
	for _, n := range refs {
		if o, ok := d.objs[n]; ok {
			if data, err := d.streamData(o); err == nil {
				out.Write(data)
				out.WriteByte('\n')
			}
		}
	}
	return out.Bytes()
}

// ---------- 字体与 ToUnicode ----------

type cMap struct {
	m    map[uint16]string
	wide bool
}

// pageFonts 收集页面资源里的字体 → CMap。
func (d *pdfDoc) pageFonts(page *pdfObj) map[string]*cMap {
	out := map[string]*cMap{}
	res := d.resolveDict(page.body, "/Resources")
	if res == "" {
		return out
	}
	fontSection := ""
	if m := reFontDict.FindStringSubmatch(res); m != nil {
		fontSection = m[1]
	} else if strings.Contains(res, "/Font") {
		// /Font 12 0 R 形式
		idx := strings.Index(res, "/Font")
		if m := rePDFRef.FindStringSubmatch(res[idx:]); m != nil {
			n, _ := strconv.Atoi(m[1])
			if o, ok := d.objs[n]; ok {
				fontSection = o.body
			}
		}
	}
	if fontSection == "" {
		return out
	}
	for _, m := range reFontSub.FindAllStringSubmatch(fontSection, -1) {
		name := m[1]
		num, _ := strconv.Atoi(m[2])
		fontObj, ok := d.objs[num]
		if !ok {
			continue
		}
		cm := &cMap{m: map[uint16]string{}}
		if tm := reToUniRef.FindStringSubmatch(fontObj.body); tm != nil {
			if tn, err := strconv.Atoi(tm[1]); err == nil {
				if to, ok := d.objs[tn]; ok {
					if data, err := d.streamData(to); err == nil {
						parseCMap(data, cm)
					}
				}
			}
		}
		out[name] = cm
	}
	return out
}

// resolveDict 解析 /Key 指向的字典（内联 << >> 或间接引用 N 0 R）。
func (d *pdfDoc) resolveDict(body, key string) string {
	idx := strings.Index(body, key)
	if idx < 0 {
		return ""
	}
	tail := strings.TrimLeft(body[idx+len(key):], " \t\r\n")
	if strings.HasPrefix(tail, "<<") {
		return tail
	}
	if m := rePDFRef.FindStringSubmatch(tail); m != nil {
		n, _ := strconv.Atoi(m[1])
		if o, ok := d.objs[n]; ok {
			return o.body
		}
	}
	return ""
}

// parseCMap 解析 ToUnicode CMap 的 codespace / bfchar / bfrange。
func parseCMap(data []byte, cm *cMap) {
	text := string(data)

	// codespacerange 决定字节码宽度：<0000> <FFFF> 是双字节编码（CID 字体常见）
	for _, block := range reCodeSpace.FindAllStringSubmatch(text, -1) {
		if m := reHexToken.FindStringSubmatch(block[1]); m != nil {
			if hexLen(m[1]) >= 4 {
				cm.wide = true
			}
		}
	}

	// bfchar：<code> <unicode> 成对出现
	for _, block := range reBFChar.FindAllStringSubmatch(text, -1) {
		chunks := reHexToken.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+1 < len(chunks); i += 2 {
			src, ok := hexToU16(chunks[i][1])
			if !ok {
				continue
			}
			if hexLen(chunks[i][1]) >= 4 {
				cm.wide = true
			}
			dst := hexToUnicode(chunks[i+1][1])
			if dst == "" {
				continue
			}
			cm.m[src] = dst
		}
	}

	// bfrange：<lo> <hi> <dst> 或 <lo> <hi> [ <d1> <d2> … ]
	for _, block := range reBFRange.FindAllStringSubmatch(text, -1) {
		for _, one := range reBFRangeOne.FindAllStringSubmatch(block[1], -1) {
			lo, ok1 := hexToU16(one[1])
			hi, ok2 := hexToU16(one[2])
			if !ok1 || !ok2 || hi < lo {
				continue
			}
			if hexLen(one[1]) >= 4 {
				cm.wide = true
			}
			target := strings.TrimSpace(one[3])
			if strings.HasPrefix(target, "[") {
				// 数组形式：逐个映射，超出范围的部分忽略
				seq := reHexToken.FindAllStringSubmatch(target, -1)
				for k, t := range seq {
					code := int(lo) + k
					if code > int(hi) {
						break
					}
					if s := hexToUnicode(t[1]); s != "" {
						cm.m[uint16(code)] = s
					}
				}
			} else {
				base := hexToUnicode(strings.Trim(target, "<>"))
				if base == "" {
					continue
				}
				runes := []rune(base)
				for code := int(lo); code <= int(hi); code++ {
					if len(runes) == 0 {
						break
					}
					next := append([]rune{}, runes...)
					next[len(next)-1] += rune(code - int(lo))
					cm.m[uint16(code)] = string(next)
				}
			}
		}
	}
}

// hexLen 十六进制串的有效位数（忽略空白）。
func hexLen(s string) int {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	return len(s)
}

// hexToU16 十六进制串 → 单字节码值。
func hexToU16(s string) (uint16, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return 0, false
	}
	if len(s) > 4 {
		s = s[len(s)-4:] // 取低 16 位
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}

// hexToUnicode UTF-16BE 十六进制 → 字符串（4 位一组；非 4 倍数时按 2 位一组）。
func hexToUnicode(s string) string {
	s = strings.Trim(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), "<>")
	if s == "" || len(s)%2 != 0 {
		return ""
	}
	var runes []rune
	if len(s)%4 == 0 {
		for i := 0; i+4 <= len(s); i += 4 {
			v, err := strconv.ParseUint(s[i:i+4], 16, 32)
			if err != nil {
				return ""
			}
			runes = append(runes, rune(v))
		}
	} else {
		for i := 0; i+2 <= len(s); i += 2 {
			v, err := strconv.ParseUint(s[i:i+2], 16, 32)
			if err != nil {
				return ""
			}
			runes = append(runes, rune(v))
		}
	}
	if len(runes) == 0 {
		return ""
	}
	return string(runes)
}

// ---------- 内容流文字抽取 ----------

// extractFromContent 扫描内容流，按 Tj / TJ 取文字，按定位操作切段。
func extractFromContent(content []byte, fonts map[string]*cMap) string {
	var out strings.Builder
	cur := &cMap{m: map[uint16]string{}}
	lastWasText := false
	lastX, lastY := 0.0, 0.0
	hasPos := false

	i := 0
	n := len(content)
	for i < n {
		c := content[i]
		switch c {
		case '%':
			for i < n && content[i] != '\n' && content[i] != '\r' {
				i++
			}
		case '(':
			lit, next := readPDFString(content, i)
			i = next
			// 读操作符
			op := nextOperator(content, i)
			emit := func() {
				s := decodeWithCMap(lit, cur)
				if s == "" {
					return
				}
				out.WriteString(s)
				lastWasText = true
			}
			switch op {
			case "Tj", "'", "\"":
				if op != "Tj" {
					newlineIfNeeded(&out, &lastWasText)
				}
				emit()
				i = skipOperator(content, i)
			}
			continue
		case '<':
			if i+1 < n && content[i+1] == '<' {
				i += 2
				continue
			}
			hexStr, next := readPDFHex(content, i)
			i = next
			op := nextOperator(content, i)
			if op == "Tj" || op == "'" || op == "\"" {
				if s := decodeWithCMap(hexStr, cur); s != "" {
					out.WriteString(s)
					lastWasText = true
				}
				i = skipOperator(content, i)
			}
			continue
		case '[':
			// TJ 数组
			arr, next := readTJArray(content, i)
			i = next
			if nextOperator(content, i) == "TJ" {
				if s := decodeWithCMap(arr, cur); s != "" {
					out.WriteString(s)
					lastWasText = true
				}
				i = skipOperator(content, i)
			}
			continue
		case '/':
			// 字体名：/F1 12 Tf（字号参数可有可无，中间可能还夹着其他 token）
			start := i + 1
			j := start
			for j < n && !isPDFDelimiter(content[j]) {
				j++
			}
			name := string(content[start:j])
			if name != "" {
				if matched, next := matchFontOp(content, j); matched {
					if cm, ok := fonts[name]; ok && cm != nil {
						cur = cm
					} else {
						cur = &cMap{m: map[uint16]string{}}
					}
					i = next
					continue
				}
			}
			i = j
			continue
		case 'T', 'E', 'B':
			op := nextOperator(content, i)
			switch op {
			case "Tm":
				// 文本矩阵：[a b c d x y] Tm，按 y 变化分行、x 间隔补空格
				if ns := numsBefore(content, i, 6); len(ns) == 6 {
					x, y := ns[4], ns[5]
					if hasPos {
						if math.Abs(y-lastY) > 1 {
							newlineIfNeeded(&out, &lastWasText)
						} else if x-lastX > 4 {
							spaceIfNeeded(&out, &lastWasText)
						}
					}
					lastX, lastY, hasPos = x, y, true
				}
				i = skipOperator(content, i)
				continue
			case "Td", "TD":
				if ns := numsBefore(content, i, 2); len(ns) == 2 {
					lastX += ns[0]
					lastY += ns[1]
					if math.Abs(ns[1]) > 1 {
						newlineIfNeeded(&out, &lastWasText)
					} else if ns[0] > 4 {
						spaceIfNeeded(&out, &lastWasText)
					}
					hasPos = true
				}
				i = skipOperator(content, i)
				continue
			case "T*", "TL":
				newlineIfNeeded(&out, &lastWasText)
				i = skipOperator(content, i)
				continue
			case "BT":
				i = skipOperator(content, i)
				continue
			case "ET":
				i = skipOperator(content, i)
				continue
			}
			i++
			continue
		default:
			i++
		}
	}

	return tidyPDFText(out.String())
}

func newlineIfNeeded(out *strings.Builder, last *bool) {
	if !*last {
		return
	}
	s := out.String()
	if s != "" && !strings.HasSuffix(s, "\n") {
		out.WriteString("\n")
	}
	*last = false
}

// spaceIfNeeded 同一行内两段文字之间补一个空格（x 方向有明显间隔时）。
func spaceIfNeeded(out *strings.Builder, last *bool) {
	if !*last {
		return
	}
	s := out.String()
	if s == "" || strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\n") {
		return
	}
	out.WriteString(" ")
}

// numsBefore 从操作符位置往前收集数字操作数（最多 max 个）。
func numsBefore(src []byte, pos, max int) []float64 {
	var toks []string
	j := pos - 1
	for j >= 0 && len(toks) < max {
		for j >= 0 && isSpaceByte(src[j]) {
			j--
		}
		end := j + 1
		for j >= 0 && !isSpaceByte(src[j]) && !isPDFDelimiter(src[j]) {
			j--
		}
		tok := string(src[j+1 : end])
		if tok == "" || !reNumToken.MatchString(tok) {
			break
		}
		toks = append([]string{tok}, toks...)
	}
	out := make([]float64, 0, len(toks))
	for _, t := range toks {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// matchFontOp 判断字体名之后是否会跟 Tf（允许中间夹一个字号参数）。
func matchFontOp(src []byte, j int) (bool, int) {
	k := j
	for step := 0; step < 3; step++ {
		tok := nextOperator(src, k)
		if tok == "" {
			return false, j
		}
		k = skipOperator(src, k)
		if tok == "Tf" {
			return true, k
		}
		// 只接受数字参数，遇到其他操作符说明不是字体设置
		if !reNumToken.MatchString(tok) {
			return false, j
		}
	}
	return false, j
}

// isPDFDelimiter 判断 PDF 分隔符。
func isPDFDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '/', '[', ']', '(', ')', '<', '>', '{', '}', '%':
		return true
	}
	return false
}

// nextOperator 跳过空白，读出紧随其后的操作符。
func nextOperator(src []byte, i int) string {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\r' || src[i] == '\n') {
		i++
	}
	start := i
	for i < len(src) && !isPDFDelimiter(src[i]) {
		i++
	}
	return string(src[start:i])
}

// skipOperator 跳过操作符本身。
func skipOperator(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\r' || src[i] == '\n') {
		i++
	}
	for i < len(src) && !isPDFDelimiter(src[i]) {
		i++
	}
	return i
}

// readPDFString 读取 (...) 字面量字符串，处理转义与嵌套括号。
func readPDFString(src []byte, i int) ([]byte, int) {
	depth := 0
	var out []byte
	for ; i < len(src); i++ {
		c := src[i]
		switch c {
		case '\\':
			if i+1 < len(src) {
				i++
				switch src[i] {
				case 'n':
					out = append(out, '\n')
				case 'r':
					out = append(out, '\r')
				case 't':
					out = append(out, '\t')
				case 'b':
					out = append(out, '\b')
				case 'f':
					out = append(out, '\f')
				case '(', ')', '\\':
					out = append(out, src[i])
				default:
					if src[i] >= '0' && src[i] <= '7' {
						// 八进制
						v := 0
						for k := 0; k < 3 && i < len(src) && src[i] >= '0' && src[i] <= '7'; k++ {
							v = v*8 + int(src[i]-'0')
							i++
						}
						i--
						out = append(out, byte(v))
					} else {
						out = append(out, src[i])
					}
				}
			}
		case '(':
			depth++
			if depth > 1 {
				out = append(out, c)
			}
		case ')':
			depth--
			if depth == 0 {
				return out, i + 1
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out, i
}

// readPDFHex 读取 <...> 十六进制字符串。
func readPDFHex(src []byte, i int) ([]byte, int) {
	i++ // 跳过 <
	var hex []byte
	for ; i < len(src); i++ {
		c := src[i]
		if c == '>' {
			i++
			break
		}
		if !isPDFDelimiter(c) {
			hex = append(hex, c)
		}
	}
	return decodeASCIIHex(hex), i
}

// readTJArray 读取 [ (..) -120 (..) ] 数组，把其中的字符串拼接（字距忽略）。
func readTJArray(src []byte, i int) ([]byte, int) {
	i++ // 跳过 [
	var out []byte
	for i < len(src) {
		c := src[i]
		if c == ']' {
			return out, i + 1
		}
		switch c {
		case '(':
			s, next := readPDFString(src, i)
			out = append(out, s...)
			i = next
		case '<':
			if i+1 < len(src) && src[i+1] == '<' {
				i += 2
				continue
			}
			s, next := readPDFHex(src, i)
			out = append(out, s...)
			i = next
		default:
			i++
		}
	}
	return out, i
}

// decodeWithCMap 按当前字体的 CMap 把字节码还原成文字。
func decodeWithCMap(raw []byte, cm *cMap) string {
	if len(raw) == 0 {
		return ""
	}
	if cm == nil || len(cm.m) == 0 {
		// 没有 ToUnicode：按 ASCII / Latin-1 处理，非打印字符丢弃
		var b strings.Builder
		for _, c := range raw {
			if c >= 32 && c < 127 {
				b.WriteByte(c)
			} else if c >= 0xC0 {
				b.WriteRune(rune(c))
			}
		}
		return b.String()
	}
	var b strings.Builder
	if cm.wide {
		for i := 0; i+1 < len(raw); i += 2 {
			code := uint16(raw[i])<<8 | uint16(raw[i+1])
			if s, ok := cm.m[code]; ok {
				b.WriteString(s)
			}
		}
	} else {
		for _, c := range raw {
			if s, ok := cm.m[uint16(c)]; ok {
				b.WriteString(s)
			} else if c >= 32 && c < 127 {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// tidyPDFText 清理 PDF 抽取结果里的多余空白。
func tidyPDFText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\u0000", "")
	s = reWSClean.ReplaceAllString(s, " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	s = strings.Join(lines, "\n")
	s = reManyBreaks.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// pdfIntKey 读取字典里的整数值 /Key N。
func pdfIntKey(body, key string) int {
	idx := strings.Index(body, key)
	if idx < 0 {
		return 0
	}
	rest := strings.TrimSpace(body[idx+len(key):])
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(rest[:end])
	return n
}

// readBinaryFile 读取二进制文档（限制大小）。
func readBinaryFile(p string) ([]byte, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, errors.New("文件不存在或无权限读取")
	}
	if info.Size() > maxConvertSource {
		return nil, errors.New("文件超过 64 MB，暂不支持打开")
	}
	return os.ReadFile(p)
}
