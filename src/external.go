package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// 外部 Markdown 文件：飞牛文件管理器通过「打开方式」把 .md 交给本应用，
// 系统在入口 URL 后追加 ?path=，应用读取、编辑并原样写回用户目录。
//
// 安全模型（对应官方「授权与文件」系列文档）：
//  1. 路径必须落在「应用可访问范围」内：应用授权目录（TRIM_DATA_ACCESSIBLE_PATHS）、
//     用户已明确授权的目录/文件，或当前用户的文集存储目录；
//  2. 写回前用当前登录用户的 uid 调 trim.file.checkUserACL 复核读/写权限；
//  3. 最终仍受应用用户自身的 ACL 约束（OS 层兜底），授权只放大到「用户本来就能访问」的范围。
const (
	appName         = "waterfish_anthology"
	maxExternalSize = 8 << 20 // 单个外部文件上限 8 MiB
	maxGrantPaths   = 200     // 授权记录条目上限，防无限增长
	maxScanFiles    = 300     // 单次扫描返回的文件数上限
	maxScanDepth    = 3       // 扫描子目录深度
)

var (
	// errNeedGrant 目标路径不在可访问范围内，需要用户先授权。
	errNeedGrant = errors.New("需要授权访问该文件所在位置")
	// errNotText 不是受支持的文件格式。
	errNotText = errors.New("不支持该文件格式")
)

// allowedExternal 判断扩展名是否受支持（见 convert.go 的 formatTable）。
func allowedExternal(path string) bool {
	_, ok := formatOf(path)
	return ok
}

type extGrants struct {
	Paths []string `json:"paths"`
}

func (u *UserApp) grantsPath() string { return filepath.Join(u.userDir(), "grants.json") }

var grantsMu sync.Mutex

// grantedPaths 读取该用户已授权的路径列表（文件或目录）。
func (u *UserApp) grantedPaths() []string {
	grantsMu.Lock()
	defer grantsMu.Unlock()
	raw, err := os.ReadFile(u.grantsPath())
	if err != nil {
		return nil
	}
	var g extGrants
	if json.Unmarshal(raw, &g) != nil {
		return nil
	}
	return g.Paths
}

// addGrant 记录一条用户授权路径（幂等）。
func (u *UserApp) addGrant(path string) error {
	path = filepath.Clean(path)
	grantsMu.Lock()
	defer grantsMu.Unlock()

	list := []string{}
	if raw, err := os.ReadFile(u.grantsPath()); err == nil {
		var g extGrants
		if json.Unmarshal(raw, &g) == nil {
			list = g.Paths
		}
	}
	for _, p := range list {
		if p == path {
			return nil
		}
	}
	if len(list) >= maxGrantPaths {
		list = list[len(list)-maxGrantPaths+1:]
	}
	list = append(list, path)
	if err := os.MkdirAll(u.userDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(extGrants{Paths: list}, "", "  ")
	return os.WriteFile(u.grantsPath(), data, 0o644)
}

// externalRoots 外部文件可访问的根集合：应用授权目录 + 用户授权路径。
func (u *UserApp) externalRoots() []string {
	roots := u.allowedRoots()
	for _, p := range u.grantedPaths() {
		roots = append(roots, p)
	}
	return roots
}

// resolveExternalPath 规范化并校验用户传入的路径（一律视为不可信输入）。
func resolveExternalPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("缺少文件路径")
	}
	if strings.ContainsRune(raw, 0) || !filepath.IsAbs(raw) {
		return "", errors.New("路径格式不正确")
	}
	abs := filepath.Clean(raw)
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return abs, nil
}

// withinExternal 判断路径是否在可访问范围内（含符号链接解析后的比较）。
func (u *UserApp) withinExternal(path string) bool {
	abs := filepath.Clean(path)
	for _, root := range u.externalRoots() {
		if root == "" {
			continue
		}
		r := filepath.Clean(root)
		if real, err := filepath.EvalSymlinks(r); err == nil {
			r = real
		}
		if abs == r || strings.HasPrefix(abs, r+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// uidOf 当前登录用户的数字 uid（网关头 X-Trim-Userid）。
func (u *UserApp) uidOf() int {
	if u.user == nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(u.user.ID))
	return n
}

// aclOf 复核当前用户对路径的权限：优先问系统，接口不可用时退回本地探测。
func (u *UserApp) aclOf(path string) aclEntry {
	entry := aclEntry{Path: path}
	if list, err := checkUserACL(appName, u.uidOf(), []string{path}); err == nil && len(list) > 0 {
		return list[0]
	}
	// 降级：应用能读到即视为可读；能打开写入句柄即可写（不截断内容）
	if _, err := os.Stat(path); err == nil {
		entry.Readable = true
		if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
			entry.Writable = true
			f.Close()
		}
	}
	return entry
}

// ExternalFile 外部文件内容与元信息。
type ExternalFile struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	ModTime  string `json:"modTime"`
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
	HasMeta  bool   `json:"hasFrontMatter"`

	// 打开方式：editable（可编辑写回）/ readonly（只读预览，另存为文集文章）
	Mode        string   `json:"mode"`
	Format      string   `json:"format"`
	FormatLabel string   `json:"formatLabel"`
	Warnings    []string `json:"warnings,omitempty"`
	// RawURL 浏览器可直接渲染的原始文件地址（目前仅 PDF）
	RawURL string `json:"rawUrl,omitempty"`
	// Linked 是否已加入文章库（外链文章，不复制内容）
	Linked   bool   `json:"linked"`
	LinkedID string `json:"linkedId,omitempty"`
	// DisplayPath 语义化路径（如「存储空间1/admin 的文件/notes/a.md」）。
	// 系统接口不可用时为空，前端退回展示原始路径。
	DisplayPath string `json:"displayPath,omitempty"`
}

// semanticPath 把内部路径转成用户能理解的展示路径。
func semanticPath(path string) string {
	if !trimAPIAvailable() {
		return ""
	}
	m, err := convertPaths(appName, []string{path}, "zh-CN")
	if err != nil {
		return ""
	}
	return m[path]
}

// ---------- frontmatter 处理 ----------

// splitFrontMatter 拆分 `---` 包裹的元信息与正文；无元信息时 head 为空。
func splitFrontMatter(text string) (head, body string, ok bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", text, false
	}
	if end := strings.Index(text[4:], "\n---"); end >= 0 {
		head = text[4 : 4+end]
		body = strings.TrimLeft(text[4+end+4:], "\n")
		return head, body, true
	}
	return "", text, false
}

// mergeFrontMatter 用新的标题与正文生成文件内容：
// 保留原元信息里的其他字段（tags / date 等用户数据绝不丢弃），仅更新 title。
// 原文件没有元信息时保持原样（不擅自添加 frontmatter），返回 false 供前端提示。
func mergeFrontMatter(orig, title, content string) (string, bool) {
	head, _, ok := splitFrontMatter(orig)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !ok {
		return content, false
	}
	title = strings.ReplaceAll(strings.TrimSpace(title), "\n", " ")

	lines := strings.Split(head, "\n")
	found := false
	for i, line := range lines {
		if k, _, has := strings.Cut(line, ":"); has && strings.EqualFold(strings.TrimSpace(k), "title") {
			if title != "" {
				lines[i] = "title: " + title
			}
			found = true
			break
		}
	}
	if !found && title != "" {
		lines = append([]string{"title: " + title}, lines...)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n---\n\n")
	b.WriteString(strings.TrimLeft(content, "\n"))
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	return b.String(), true
}

// openExternal 读取外部文件：范围校验 → 类型/大小校验 → 按可编辑程度返回内容。
//   - editable：返回正文（可编辑写回原文件）
//   - readonly：解析成 Markdown 供只读预览，原文件保持不变
//   - binary：直接拒绝，提示另存为别的格式
func (u *UserApp) openExternal(raw string) (ExternalFile, error) {
	abs, err := resolveExternalPath(raw)
	if err != nil {
		return ExternalFile{}, err
	}
	f, ok := formatOf(abs)
	if !ok {
		return ExternalFile{}, errNotText
	}
	if !u.withinExternal(abs) {
		return ExternalFile{}, errNeedGrant
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ExternalFile{}, errors.New("文件不存在或无权限读取")
	}
	if info.IsDir() {
		return ExternalFile{}, errors.New("这是一个文件夹，不是文件")
	}
	limit := int64(maxExternalSize)
	if f.Mode != modeEditable {
		limit = maxConvertSource
	}
	if info.Size() > limit {
		return ExternalFile{}, fmt.Errorf("文件超过 %d MB，暂不支持打开", limit>>20)
	}

	base := ExternalFile{
		Path:        abs,
		Name:        filepath.Base(abs),
		Size:        info.Size(),
		ModTime:     info.ModTime().Format(time.RFC3339),
		Mode:        f.Mode,
		Format:      f.Kind,
		FormatLabel: f.Label,
		DisplayPath: semanticPath(abs),
	}

	// 闭源二进制：说明清楚为什么打不开，并给出可执行的替代做法
	if f.Mode == modeBinary {
		return base, fmt.Errorf("%s 属于闭源二进制格式，无法解析正文；请先用对应软件打开并另存为 .docx 或 PDF，再从水鱼文集打开", f.Label)
	}

	if f.Mode == modeReadonly {
		res, err := convertFile(abs, f)
		if err != nil {
			return base, err
		}
		base.Title = res.Title
		base.Content = res.Markdown
		base.Warnings = res.Warnings
		base.Readable = true
		if f.Kind == "pdf" {
			// PDF 用浏览器原生阅读器预览，版式与原文件完全一致
			base.RawURL = "api/external/raw?path=" + url.QueryEscape(abs)
		}
		u.markLinked(&base)
		return base, nil
	}

	rawBytes, err := os.ReadFile(abs)
	if err != nil {
		return ExternalFile{}, errNeedGrant // 授权范围内但应用用户无 ACL：引导再授权
	}
	if !utf8.Valid(rawBytes) {
		return ExternalFile{}, errors.New("文件不是 UTF-8 文本，暂无法编辑；请另存为 UTF-8 后重试")
	}

	text := string(rawBytes)
	head, body, hasMeta := splitFrontMatter(text)
	title := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	if hasMeta {
		for _, line := range strings.Split(head, "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "title") {
				if t := strings.TrimSpace(v); t != "" {
					title = t
				}
			}
		}
	} else {
		for _, line := range strings.Split(body, "\n") {
			if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
				title = strings.TrimSpace(t)
				break
			}
		}
	}

	acl := u.aclOf(abs)
	base.Title = title
	base.Content = body
	base.Readable = acl.Readable
	base.Writable = acl.Writable
	base.HasMeta = hasMeta
	u.markLinked(&base)
	return base, nil
}

// saveExternal 把编辑后的正文写回原文件（原子写入，保留文件权限）。
func (u *UserApp) saveExternal(raw, title, content string) (ExternalFile, error) {
	abs, err := resolveExternalPath(raw)
	if err != nil {
		return ExternalFile{}, err
	}
	f, ok := formatOf(abs)
	if !ok || f.Mode != modeEditable {
		return ExternalFile{}, errors.New("该格式不支持直接写回，请用「另存为文集文章」")
	}
	if !u.withinExternal(abs) {
		return ExternalFile{}, errNeedGrant
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ExternalFile{}, errors.New("文件不存在或无权限写入")
	}
	if info.IsDir() {
		return ExternalFile{}, errors.New("这是一个文件夹，不是文件")
	}
	if err := u.checkWritable(abs); err != nil {
		return ExternalFile{}, err
	}
	orig, err := os.ReadFile(abs)
	if err != nil {
		return ExternalFile{}, errors.New("读取原文件失败")
	}

	next, _ := mergeFrontMatter(string(orig), title, content)
	// 优先原子写（同目录临时文件 + rename）；仅有文件级授权、目录不可写时退回直接覆盖
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".wf-*.tmp")
	if err != nil {
		if werr := os.WriteFile(abs, []byte(next), info.Mode().Perm()); werr != nil {
			return ExternalFile{}, errors.New("保存失败：没有写入权限")
		}
		return u.openExternal(abs)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(next); err != nil {
		tmp.Close()
		return ExternalFile{}, errors.New("写入失败")
	}
	if err := tmp.Close(); err != nil {
		return ExternalFile{}, errors.New("写入失败")
	}
	_ = os.Chmod(tmpName, info.Mode().Perm())
	if err := os.Rename(tmpName, abs); err != nil {
		return ExternalFile{}, errors.New("保存失败：目标文件不允许覆盖")
	}
	return u.openExternal(abs)
}

// checkWritable 写前复核权限：系统 ACL 优先，不可用时退回本地探测。
func (u *UserApp) checkWritable(path string) error {
	if list, err := checkUserACL(appName, u.uidOf(), []string{path}); err == nil && len(list) > 0 {
		if !list[0].Writable {
			return errors.New("当前用户对该文件没有写入权限")
		}
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return errors.New("应用没有该文件的写入权限，请先授权所在文件夹")
	}
	f.Close()
	return nil
}

// ---------- 外链文章（加入文章库但不复制内容） ----------

// linkedArticle 指向磁盘文件的书目条目：文章库只记录路径，正文始终从原文件读取，
// 因此在水鱼文集里的改动会直接落到原文件，外部改动也能立刻看到。
type linkedArticle struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Created string `json:"created"`
}

type linkIndex struct {
	Items []linkedArticle `json:"items"`
}

const maxLinkedArticles = 500

var linksMu sync.Mutex

func (u *UserApp) linksPath() string { return filepath.Join(u.userDir(), "links.json") }

// linkIDFor 由路径派生稳定 ID：同一文件多次添加不会产生重复条目。
func linkIDFor(path string) string {
	sum := sha1.Sum([]byte(path))
	return "ext-" + hex.EncodeToString(sum[:6])
}

func (u *UserApp) linkedList() []linkedArticle {
	linksMu.Lock()
	defer linksMu.Unlock()
	raw, err := os.ReadFile(u.linksPath())
	if err != nil {
		return nil
	}
	var idx linkIndex
	if json.Unmarshal(raw, &idx) != nil {
		return nil
	}
	return idx.Items
}

func (u *UserApp) writeLinks(items []linkedArticle) error {
	if err := os.MkdirAll(u.userDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(linkIndex{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.linksPath(), data, 0o644)
}

// addLink 把外部文件加入文章库（幂等）。
func (u *UserApp) addLink(path, title string) (linkedArticle, error) {
	path = filepath.Clean(path)
	id := linkIDFor(path)
	linksMu.Lock()
	var items []linkedArticle
	if raw, err := os.ReadFile(u.linksPath()); err == nil {
		var idx linkIndex
		if json.Unmarshal(raw, &idx) == nil {
			items = idx.Items
		}
	}
	for i := range items {
		if items[i].ID == id {
			if title != "" && items[i].Title != title {
				items[i].Title = title
				_ = u.writeLinks(items)
			}
			out := items[i]
			linksMu.Unlock()
			return out, nil
		}
	}
	entry := linkedArticle{ID: id, Path: path, Title: title, Created: nowStamp()}
	if entry.Title == "" {
		entry.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if len(items) >= maxLinkedArticles {
		items = items[len(items)-maxLinkedArticles+1:]
	}
	items = append(items, entry)
	err := u.writeLinks(items)
	linksMu.Unlock()
	return entry, err
}

// removeLink 只移除书目条目，磁盘上的原文件不受影响。
func (u *UserApp) removeLink(id string) (linkedArticle, bool) {
	linksMu.Lock()
	defer linksMu.Unlock()
	raw, err := os.ReadFile(u.linksPath())
	if err != nil {
		return linkedArticle{}, false
	}
	var idx linkIndex
	if json.Unmarshal(raw, &idx) != nil {
		return linkedArticle{}, false
	}
	out := linkedArticle{}
	found := false
	next := make([]linkedArticle, 0, len(idx.Items))
	for _, it := range idx.Items {
		if it.ID == id {
			out, found = it, true
			continue
		}
		next = append(next, it)
	}
	if !found {
		return linkedArticle{}, false
	}
	_ = u.writeLinks(next)
	return out, true
}

func (u *UserApp) findLink(id string) (linkedArticle, bool) {
	for _, it := range u.linkedList() {
		if it.ID == id {
			return it, true
		}
	}
	return linkedArticle{}, false
}

// markLinked 给已打开的文件标注「已加入文章库」。
func (u *UserApp) markLinked(f *ExternalFile) {
	if f == nil || f.Path == "" {
		return
	}
	id := linkIDFor(f.Path)
	for _, it := range u.linkedList() {
		if it.ID == id {
			f.Linked = true
			f.LinkedID = id
			return
		}
	}
}

// externalRaw 读取原始文件字节，供浏览器原生预览（目前用于 PDF）。
func (u *UserApp) externalRaw(raw string) ([]byte, string, error) {
	abs, err := resolveExternalPath(raw)
	if err != nil {
		return nil, "", err
	}
	f, ok := formatOf(abs)
	if !ok || f.Mode == modeBinary {
		return nil, "", errNotText
	}
	if !u.withinExternal(abs) {
		return nil, "", errNeedGrant
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, "", errors.New("文件不存在或无权限读取")
	}
	if info.Size() > maxConvertSource {
		return nil, "", errors.New("文件过大，无法预览")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", errNeedGrant
	}
	ct := "application/octet-stream"
	switch f.Kind {
	case "pdf":
		ct = "application/pdf"
	case "html":
		ct = "text/html; charset=utf-8"
	}
	return data, ct, nil
}

// scanExternalDir 扫描目录下受支持的文档，供「导入文章」选择。
func (u *UserApp) scanExternalDir(raw string) (string, []ExternalFile, error) {
	abs, err := resolveExternalPath(raw)
	if err != nil {
		return "", nil, err
	}
	if !u.withinExternal(abs) {
		return abs, nil, errNeedGrant
	}
	info, err := os.Stat(abs)
	if err != nil {
		return abs, nil, errors.New("目录不存在或无权限读取")
	}
	if !info.IsDir() {
		return abs, nil, errors.New("请选择一个文件夹")
	}

	out := []ExternalFile{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		names := make([]fs.DirEntry, 0, len(entries))
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
				continue
			}
			names = append(names, e)
		}
		sort.Slice(names, func(i, j int) bool {
			if names[i].IsDir() != names[j].IsDir() {
				return names[i].IsDir()
			}
			return names[i].Name() < names[j].Name()
		})
		for _, e := range names {
			if len(out) >= maxScanFiles {
				return
			}
			full := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if depth < maxScanDepth {
					walk(full, depth+1)
				}
				continue
			}
			if !allowedExternal(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() > maxExternalSize {
				continue
			}
			item := ExternalFile{
				Path:    full,
				Name:    e.Name(),
				Title:   strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
				Size:    info.Size(),
				ModTime: info.ModTime().Format(time.RFC3339),
			}
			// 只读首屏内容取标题，避免整目录读盘
			if f, err := os.Open(full); err == nil {
				buf := make([]byte, 4096)
				n, _ := f.Read(buf)
				f.Close()
				head, body, hasMeta := splitFrontMatter(string(buf[:n]))
				if hasMeta {
					for _, line := range strings.Split(head, "\n") {
						if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "title") {
							if t := strings.TrimSpace(v); t != "" {
								item.Title = t
							}
						}
					}
				} else {
					for _, line := range strings.Split(body, "\n") {
						if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok && strings.TrimSpace(t) != "" {
							item.Title = strings.TrimSpace(t)
							break
						}
					}
				}
			}
			out = append(out, item)
		}
	}
	walk(abs, 1)
	return abs, out, nil
}

// importExternal 把外部文件复制为文集文章（原样保留 frontmatter 与正文）。
func (u *UserApp) importExternal(paths []string) ([]ArticleMeta, []string) {
	imported := []ArticleMeta{}
	skipped := []string{}
	for _, raw := range paths {
		if len(imported) >= 50 {
			break
		}
		abs, err := resolveExternalPath(raw)
		if err != nil || !u.withinExternal(abs) {
			skipped = append(skipped, filepath.Base(raw)+"（未授权）")
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			skipped = append(skipped, filepath.Base(raw)+"（不可读）")
			continue
		}
		f, ok := formatOf(abs)
		if !ok || f.Mode == modeBinary {
			skipped = append(skipped, filepath.Base(raw)+"（"+f.Label+"暂不支持，请另存为 .docx 或 PDF）")
			continue
		}

		var payload []byte
		var title, content string
		art := Article{ArticleMeta: ArticleMeta{Title: strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))}}

		if f.Mode == modeReadonly {
			// 富文本 / 电子书：解析成 Markdown 后存为文集文章，原文件保持不动
			if info.Size() > maxConvertSource {
				skipped = append(skipped, filepath.Base(raw)+"（文件过大）")
				continue
			}
			res, cerr := convertFile(abs, f)
			if cerr != nil {
				skipped = append(skipped, filepath.Base(raw)+"（"+cerr.Error()+"）")
				continue
			}
			title, content = res.Title, res.Markdown
			art.Title, art.Content = title, content
			payload = []byte("---\ntitle: " + yamlQuote(title) + "\nsource: " + yamlQuote(filepath.Base(abs)) +
				"\ndate: " + nowStamp() + "\n---\n\n" + content + "\n")
		} else {
			if info.Size() > maxExternalSize {
				skipped = append(skipped, filepath.Base(raw)+"（文件过大）")
				continue
			}
			rawBytes, rerr := os.ReadFile(abs)
			if rerr != nil || !utf8.Valid(rawBytes) {
				skipped = append(skipped, filepath.Base(raw)+"（读取失败或非 UTF-8）")
				continue
			}
			art = parseArticle("", rawBytes)
			title = strings.TrimSpace(art.Title)
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
			}
			payload = rawBytes
		}

		target := u.uniqueArticlePath(slugify(title))
		if err := os.MkdirAll(u.PassageDir(), 0o755); err != nil {
			skipped = append(skipped, filepath.Base(abs)+"（文集目录不可写）")
			continue
		}
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			skipped = append(skipped, filepath.Base(abs)+"（写入文集失败）")
			continue
		}
		meta := ArticleMeta{
			ID:      strings.TrimSuffix(filepath.Base(target), ".md"),
			Title:   title,
			Created: nowStamp(),
			Updated: nowStamp(),
			Size:    int64(len(payload)),
		}
		if art.Created != "" {
			meta.Created = art.Created
		}
		if art.Updated != "" {
			meta.Updated = art.Updated
		}
		meta.Excerpt = excerptOf(art.Content, 90)
		imported = append(imported, meta)
	}
	return imported, skipped
}

// yamlQuote 给 frontmatter 里的字符串加引号并转义，避免标题里的冒号 / 引号破坏 YAML。
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return "\"" + s + "\""
}

// ---------- HTTP ----------

// handleExternal 外部文件相关接口统一入口。
func (a *App) handleExternal(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/external/"), "/")

	// needGrant 统一响应：前端据此弹出授权引导
	needGrant := func(path, kind string) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":     errNeedGrant.Error(),
			"needGrant": true,
			"path":      path,
			"kind":      kind,
		})
	}
	fail := func(err error, path, kind string) {
		switch {
		case errors.Is(err, errNeedGrant):
			needGrant(path, kind)
		case errors.Is(err, errNotText):
			writeErr(w, http.StatusUnsupportedMediaType, err.Error())
		default:
			writeErr(w, http.StatusBadRequest, err.Error())
		}
	}

	switch action {
	case "caps":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"appName":  appName,
			"uid":      ua.uidOf(),
			"openAPI":  trimAPIAvailable(),
			"roots":    len(ua.externalRoots()),
			"formats":  supportedFormats(),
			"surfaces": []string{"md", "markdown", "mdown", "mdx", "txt"},
		})

	case "formats":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"formats": supportedFormats()})

	case "raw":
		// 浏览器原生预览（PDF 用内置阅读器渲染，版式不走样）
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		data, ct, err := ua.externalRaw(r.URL.Query().Get("path"))
		if err != nil {
			fail(err, r.URL.Query().Get("path"), "file")
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	case "link":
		// 把打开的外部文件加入文章库（不复制内容，正文始终读原文件）
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Path  string `json:"path"`
			Title string `json:"title"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		abs, err := resolveExternalPath(in.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !allowedExternal(abs) {
			writeErr(w, http.StatusUnsupportedMediaType, errNotText.Error())
			return
		}
		if !ua.withinExternal(abs) {
			needGrant(abs, "file")
			return
		}
		if st, err := os.Stat(abs); err != nil || st.IsDir() {
			writeErr(w, http.StatusBadRequest, "文件不存在或不是文件")
			return
		}
		entry, err := ua.addLink(abs, strings.TrimSpace(in.Title))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entry)

	case "unlink":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			writeErr(w, http.StatusBadRequest, "缺少文章 ID")
			return
		}
		entry, ok := ua.removeLink(strings.TrimSpace(in.ID))
		if !ok {
			writeErr(w, http.StatusNotFound, "该文章不在文章库中")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": entry.Path})

	case "open":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		abs, _ := resolveExternalPath(in.Path)
		file, err := ua.openExternal(in.Path)
		if err != nil {
			fail(err, abs, "file")
			return
		}
		file.DisplayPath = semanticPath(file.Path)
		writeJSON(w, http.StatusOK, file)

	case "save":
		if r.Method != http.MethodPut {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Path    string `json:"path"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if len(in.Content) > maxExternalSize {
			writeErr(w, http.StatusBadRequest, "内容过大")
			return
		}
		abs, _ := resolveExternalPath(in.Path)
		file, err := ua.saveExternal(in.Path, in.Title, in.Content)
		if err != nil {
			fail(err, abs, "file")
			return
		}
		file.DisplayPath = semanticPath(file.Path)
		writeJSON(w, http.StatusOK, file)

	case "grant":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		abs, err := resolveExternalPath(in.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// 授权前先确认「应用确实能访问」：否则登记一个完全碰不到的路径毫无意义，
		// 也避免前端被诱导登记任意系统路径。
		if _, err := os.Stat(abs); err != nil {
			writeErr(w, http.StatusForbidden, "应用仍无法访问该位置，请在系统弹窗中完成授权")
			return
		}
		if err := ua.addGrant(abs); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存授权记录失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": abs})

	case "scan":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		abs, list, err := ua.scanExternalDir(in.Path)
		if err != nil {
			fail(err, abs, "dir")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dir": abs, "files": list})

	case "roots":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		roots := ua.externalRoots()
		items := make([]map[string]string, 0, len(roots))
		for _, p := range roots {
			items = append(items, map[string]string{"path": p, "name": filepath.Base(p)})
		}
		writeJSON(w, http.StatusOK, map[string]any{"roots": items})

	case "import":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		var in struct {
			Paths []string `json:"paths"`
		}
		if err := readJSON(r, &in); err != nil || len(in.Paths) == 0 {
			writeErr(w, http.StatusBadRequest, "请选择要导入的文件")
			return
		}
		imported, skipped := ua.importExternal(in.Paths)
		writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped})

	default:
		writeErr(w, http.StatusNotFound, "未知操作")
	}
}
