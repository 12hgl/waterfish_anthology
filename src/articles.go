package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ---------- 数据模型 ----------

// ArticleMeta 文章摘要（不含正文）。
type ArticleMeta struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Size    int64  `json:"size"`
	Excerpt string `json:"excerpt"`

	// 外链文章：文章库只记录磁盘路径，正文始终读写原文件（不复制内容）
	External bool   `json:"external,omitempty"`
	Path     string `json:"path,omitempty"`
	Format   string `json:"format,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
}

// Article 文章全文。
type Article struct {
	ArticleMeta
	Content string `json:"content"`
}

// ---------- frontmatter 读写 ----------

func nowStamp() string { return time.Now().Format(time.RFC3339) }

// unquoteYAML 去掉 frontmatter 值的引号（导入时会写成 title: "xxx" 以免冒号破坏 YAML）。
func unquoteYAML(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			inner := v[1 : len(v)-1]
			inner = strings.ReplaceAll(inner, "\\\"", "\"")
			inner = strings.ReplaceAll(inner, "\\\\", "\\")
			return inner
		}
	}
	return v
}

// parseArticle 解析 `.md` 文件：可选的 YAML 风格 frontmatter + 正文。
func parseArticle(id string, raw []byte) Article {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	a := Article{ArticleMeta: ArticleMeta{ID: id, Title: id}}

	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			head := text[4 : 4+end]
			a.Content = strings.TrimPrefix(text[4+end+4:], "\n")
			for _, line := range strings.Split(head, "\n") {
				k, v, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				k, v = strings.TrimSpace(k), unquoteYAML(strings.TrimSpace(v))
				switch k {
				case "title":
					if v != "" {
						a.Title = v
					}
				case "created":
					a.Created = v
				case "updated":
					a.Updated = v
				}
			}
			return a
		}
	}

	a.Content = text
	// 无 frontmatter 时用首个标题行作为标题
	for _, line := range strings.Split(text, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			a.Title = strings.TrimSpace(t)
			break
		}
	}
	return a
}

// renderArticle 序列化为带 frontmatter 的 `.md`。
func renderArticle(a Article) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + strings.ReplaceAll(a.Title, "\n", " ") + "\n")
	b.WriteString("created: " + a.Created + "\n")
	b.WriteString("updated: " + a.Updated + "\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(a.Content, "\n"))
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func excerptOf(content string, limit int) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") ||
			strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "---") {
			continue
		}
		out = append(out, line)
		if len([]rune(strings.Join(out, " "))) >= limit {
			break
		}
	}
	s := strings.Join(out, " ")
	// 去掉常见 Markdown 标记
	repl := strings.NewReplacer("**", "", "__", "", "`", "", "*", "", "> ", "", "[", "", "]", "")
	s = repl.Replace(s)
	s = mdLinkRe.ReplaceAllString(s, "$1")
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// ---------- 文件名 ----------

// slugify 生成安全文件名，保留中日韩字符。
func slugify(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 180 {
			break
		}
	}
	s := strings.Trim(b.String(), "-_")
	if s == "" {
		s = fmt.Sprintf("untitled-%d", time.Now().Unix())
	}
	return s
}

func (u *UserApp) uniqueArticlePath(slug string) string {
	dir := u.PassageDir()
	for i := 0; i <= 999; i++ {
		name := slug
		if i > 0 {
			name = fmt.Sprintf("%s-%d", slug, i+1)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".md")); errors.Is(err, os.ErrNotExist) {
			return filepath.Join(dir, name+".md")
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.md", slug, time.Now().Unix()))
}

func (u *UserApp) articlePath(id string) (string, error) {
	name, err := safeName(id)
	if err != nil {
		return "", err
	}
	name = strings.TrimSuffix(name, ".md")
	if name == "" {
		return "", errors.New("文章 ID 不能为空")
	}
	return filepath.Join(u.PassageDir(), name+".md"), nil
}

// ---------- 业务操作 ----------

func (u *UserApp) listArticles() ([]ArticleMeta, error) {
	entries, err := os.ReadDir(u.PassageDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ArticleMeta{}, nil
		}
		return nil, err
	}
	out := make([]ArticleMeta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(u.PassageDir(), e.Name()))
		if err != nil {
			continue
		}
		art := parseArticle(strings.TrimSuffix(e.Name(), ".md"), raw)
		art.Size = int64(len(raw))
		art.Excerpt = excerptOf(art.Content, 90)
		if art.Created == "" {
			if info, err := e.Info(); err == nil {
				art.Created = info.ModTime().Format(time.RFC3339)
			}
		}
		if art.Updated == "" {
			art.Updated = art.Created
		}
		out = append(out, art.ArticleMeta)
	}
	// 合并「外链文章」：只读磁盘上的原文件，不复制内容
	for _, ln := range u.linkedList() {
		meta := ArticleMeta{
			ID: ln.ID, Title: ln.Title, Created: ln.Created, Updated: ln.Created,
			External: true, Path: ln.Path,
		}
		if f, ok := formatOf(ln.Path); ok {
			meta.Format = f.Label
		}
		if st, err := os.Stat(ln.Path); err == nil && !st.IsDir() {
			meta.Size = st.Size()
			meta.Updated = st.ModTime().Format(time.RFC3339)
			if meta.Created == "" {
				meta.Created = meta.Updated
			}
		} else {
			meta.Missing = true
		}
		if strings.TrimSpace(meta.Title) == "" {
			meta.Title = strings.TrimSuffix(filepath.Base(ln.Path), filepath.Ext(ln.Path))
		}
		if meta.Excerpt == "" {
			meta.Excerpt = "外部文件 · " + meta.Path
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out, nil
}

func (u *UserApp) getArticle(id string) (Article, error) {
	// 外链文章：内容来自磁盘原文件
	if ln, ok := u.findLink(id); ok {
		file, err := u.openExternal(ln.Path)
		if err != nil {
			return Article{}, err
		}
		art := Article{ArticleMeta: ArticleMeta{
			ID: id, Title: file.Title, Created: ln.Created, Updated: file.ModTime,
			Size: file.Size, External: true, Path: file.Path,
		}}
		art.Content = file.Content
		if file.Format != "" {
			art.Format = file.FormatLabel
		}
		art.Excerpt = excerptOf(art.Content, 90)
		return art, nil
	}
	p, err := u.articlePath(id)
	if err != nil {
		return Article{}, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return Article{}, errors.New("文章不存在")
	}
	art := parseArticle(strings.TrimSuffix(filepath.Base(p), ".md"), raw)
	if art.Created == "" {
		art.Created = nowStamp()
	}
	if art.Updated == "" {
		art.Updated = art.Created
	}
	art.Size = int64(len(raw))
	return art, nil
}

func (u *UserApp) saveArticle(id, title, content string, isNew bool) (Article, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "未命名文章"
	}

	created := nowStamp()
	oldPath := ""
	if !isNew {
		p, err := u.articlePath(id)
		if err != nil {
			return Article{}, err
		}
		old, err := u.getArticle(id)
		if err != nil {
			return Article{}, err
		}
		if old.Created != "" {
			created = old.Created
		}
		oldPath = p
	}

	target := u.uniqueArticlePath(slugify(title))
	if !isNew && oldPath != "" && slugify(title) == id {
		target = oldPath // 标题未变，原地覆盖
	}
	art := Article{
		ArticleMeta: ArticleMeta{
			ID:      strings.TrimSuffix(filepath.Base(target), ".md"),
			Title:   title,
			Created: created,
			Updated: nowStamp(),
		},
		Content: content,
	}
	data := renderArticle(art)
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return Article{}, err
	}
	// 标题变化导致文件重命名时，清理旧文件并迁移编辑器状态
	if oldPath != "" && oldPath != target {
		_ = os.Remove(oldPath)
		u.renameEditorState(id, art.ID)
	}
	art.Size = int64(len(data))
	art.Excerpt = excerptOf(content, 90)
	return art, nil
}

func (u *UserApp) deleteArticle(id string) error {
	p, err := u.articlePath(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return errors.New("文章不存在")
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	u.deleteEditorState(id)
	return nil
}

// ---------- HTTP ----------

func (a *App) handleArticles(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := ua.listArticles()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var in struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		art, err := ua.saveArticle("", in.Title, in.Content, true)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, art)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

func (a *App) handleArticle(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/articles/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeErr(w, http.StatusBadRequest, "缺少文章 ID")
		return
	}
	id := rest
	action := ""
	if i := strings.LastIndex(rest, "/"); i >= 0 {
		id, action = rest[:i], rest[i+1:]
	}

	switch {
	case action == "copy" && r.Method == http.MethodGet:
		art, err := ua.getArticle(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		text, local, cloud := ua.buildCopyText(art)
		writeJSON(w, http.StatusOK, map[string]any{
			"text":         text,
			"localImages":  local,
			"cloudImages":  cloud,
			"articleTitle": art.Title,
		})
	case action == "editor" && r.Method == http.MethodGet:
		// 读取编辑器状态（自动保存开关 + 修改记录）；无侧车文件时返回空状态
		writeJSON(w, http.StatusOK, ua.loadEditorState(id))
	case action == "editor" && r.Method == http.MethodPut:
		var in EditorState
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if err := ua.saveEditorState(id, in); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case action != "":
		writeErr(w, http.StatusNotFound, "未知操作")
	case r.Method == http.MethodGet:
		art, err := ua.getArticle(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, art)
	case r.Method == http.MethodPut:
		if ln, ok := ua.findLink(id); ok {
			// 外链文章：写回磁盘原文件，而不是在文集目录里另存副本
			writeErr(w, http.StatusBadRequest, "这是外部文件，请在编辑器里保存："+ln.Path)
			return
		}
		var in struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		art, err := ua.saveArticle(id, in.Title, in.Content, false)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, art)
	case r.Method == http.MethodDelete:
		if _, ok := ua.removeLink(id); ok {
			// 外链文章：只移出文章库，磁盘上的原文件保持不动
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "keptFile": true})
			return
		}
		if err := ua.deleteArticle(id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}
