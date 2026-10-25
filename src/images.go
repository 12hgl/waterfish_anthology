package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CloudImage 图床图片索引项。
type CloudImage struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	PublicID string `json:"publicId"`
	Bytes    int64  `json:"bytes"`
	Created  string `json:"created"`
}

// LocalImage 本地图片。
type LocalImage struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Updated  string `json:"updated"`
	URL      string `json:"url"`
	Markdown string `json:"markdown"`
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".avif": true, ".bmp": true, ".svg": true,
}

var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)`)

func (u *UserApp) cloudIndexPath() string { return filepath.Join(u.MetaDir(), "cloud.json") }

func (u *UserApp) loadCloudIndex() []CloudImage {
	data, err := os.ReadFile(u.cloudIndexPath())
	if err != nil {
		return []CloudImage{}
	}
	var list []CloudImage
	if json.Unmarshal(data, &list) != nil {
		return []CloudImage{}
	}
	return list
}

func (u *UserApp) saveCloudIndex(list []CloudImage) error {
	if err := os.MkdirAll(u.MetaDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.cloudIndexPath(), data, 0o644)
}

func (u *UserApp) listLocalImages() []LocalImage {
	entries, err := os.ReadDir(u.PhotoDir())
	if err != nil {
		return []LocalImage{}
	}
	out := make([]LocalImage, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, LocalImage{
			Name:     e.Name(),
			Size:     info.Size(),
			Updated:  info.ModTime().Format(time.RFC3339),
			URL:      "photo/" + e.Name(),
			Markdown: "photo/" + e.Name(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

func (a *App) handleImages(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"local": ua.listLocalImages(),
		"cloud": ua.loadCloudIndex(),
		"cloudEnabled": func() bool {
			c := ua.Cloudinary()
			return c.Enabled && c.CloudName != "" && c.UploadPreset != ""
		}(),
	})
}

func (a *App) handleImage(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/images/"), "/")
	switch {
	case rest == "local" && r.Method == http.MethodDelete:
		// 批量删除：/api/images/local?name=xxx
		name, err := safeName(r.URL.Query().Get("name"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "非法文件名")
			return
		}
		if !imageExts[strings.ToLower(filepath.Ext(name))] {
			writeErr(w, http.StatusBadRequest, "只允许删除图片文件")
			return
		}
		if err := os.Remove(filepath.Join(ua.PhotoDir(), name)); err != nil {
			writeErr(w, http.StatusInternalServerError, "删除失败："+err.Error())
			return
		}
		ua.forgetPhotoHash(name)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case strings.HasPrefix(rest, "local/"):
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		name, err := safeName(strings.TrimPrefix(rest, "local/"))
		if err != nil || !imageExts[strings.ToLower(filepath.Ext(name))] {
			writeErr(w, http.StatusBadRequest, "非法文件名")
			return
		}
		if err := os.Remove(filepath.Join(ua.PhotoDir(), name)); err != nil {
			writeErr(w, http.StatusInternalServerError, "删除失败："+err.Error())
			return
		}
		ua.forgetPhotoHash(name)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case strings.HasPrefix(rest, "cloud/") && r.Method == http.MethodDelete:
		idx := strings.TrimPrefix(rest, "cloud/")
		list := ua.loadCloudIndex()
		out := list[:0]
		removed := false
		for i, c := range list {
			if fmt.Sprint(i) == idx || c.PublicID == idx {
				removed = true
				continue
			}
			out = append(out, c)
		}
		if !removed {
			writeErr(w, http.StatusNotFound, "记录不存在")
			return
		}
		if err := ua.saveCloudIndex(out); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeErr(w, http.StatusNotFound, "未知操作")
	}
}

// handleLocalImage 直出本地图片，供 Markdown 相对路径引用。
func (a *App) handleLocalImage(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	name, err := safeName(strings.TrimPrefix(r.URL.Path, "/photo/"))
	if err != nil || !imageExts[strings.ToLower(filepath.Ext(name))] {
		http.Error(w, "非法文件名", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeFile(w, r, filepath.Join(ua.PhotoDir(), name))
}

// ---------- 上传 ----------

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "上传内容过大或格式错误")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少上传文件")
		return
	}
	defer file.Close()

	name := cleanUploadName(header)
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取文件失败")
		return
	}

	c := ua.Cloudinary()
	if c.Enabled && c.CloudName != "" && c.UploadPreset != "" {
		url, publicID, err := uploadToCloudinary(c, name, data)
		if err != nil {
			// 图床失败时明确反馈，不静默降级成本地
			writeErr(w, http.StatusBadGateway, "图床上传失败："+err.Error())
			return
		}
		list := ua.loadCloudIndex()
		list = append([]CloudImage{{
			Name: name, URL: url, PublicID: publicID,
			Bytes: int64(len(data)), Created: time.Now().Format(time.RFC3339),
		}}, list...)
		_ = ua.saveCloudIndex(list)
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": "cloud", "name": name, "url": url, "markdown": url,
		})
		return
	}

	if err := os.MkdirAll(ua.PhotoDir(), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "创建图片目录失败")
		return
	}
	// 重复图片检测：内容哈希命中已有文件时直接复用，不重复落盘
	ua.rebuildPhotoHash()
	sum := sha256Hex(data)
	if existing, ok := ua.findPhotoByHash(sum); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": "local", "name": existing, "duplicate": true,
			"url":      "photo/" + existing,
			"markdown": "photo/" + existing,
		})
		return
	}
	final := ua.uniquePhotoName(name)
	if err := os.WriteFile(filepath.Join(ua.PhotoDir(), final), data, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存图片失败："+err.Error())
		return
	}
	idx := ua.loadPhotoHash()
	idx[sum] = final
	_ = ua.savePhotoHash(idx)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": "local", "name": final,
		"url":      "photo/" + final,
		"markdown": "photo/" + final,
	})
}

// ---------- 图片内容哈希索引（去重） ----------

func (u *UserApp) photoHashPath() string { return filepath.Join(u.MetaDir(), "photo-hash.json") }

func (u *UserApp) loadPhotoHash() map[string]string {
	data, err := os.ReadFile(u.photoHashPath())
	if err != nil {
		return map[string]string{}
	}
	var m map[string]string
	if json.Unmarshal(data, &m) != nil || m == nil {
		return map[string]string{}
	}
	return m
}

func (u *UserApp) savePhotoHash(m map[string]string) error {
	if err := os.MkdirAll(u.MetaDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.photoHashPath(), data, 0o644)
}

// rebuildPhotoHash 在索引为空时扫描 photo 目录补建，兼容历史图片。
func (u *UserApp) rebuildPhotoHash() {
	idx := u.loadPhotoHash()
	if len(idx) > 0 {
		return
	}
	entries, err := os.ReadDir(u.PhotoDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(u.PhotoDir(), e.Name()))
		if err != nil {
			continue
		}
		if sum := sha256Hex(data); idx[sum] == "" {
			idx[sum] = e.Name()
		}
	}
	_ = u.savePhotoHash(idx)
}

// findPhotoByHash 返回与给定内容一致的已有文件名（文件仍存在才有效）。
func (u *UserApp) findPhotoByHash(sum string) (string, bool) {
	idx := u.loadPhotoHash()
	name, ok := idx[sum]
	if !ok {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(u.PhotoDir(), name)); err != nil {
		delete(idx, sum)
		_ = u.savePhotoHash(idx)
		return "", false
	}
	return name, true
}

// forgetPhotoHash 删除图片后清理哈希索引中对应项。
func (u *UserApp) forgetPhotoHash(name string) {
	idx := u.loadPhotoHash()
	changed := false
	for sum, n := range idx {
		if n == name {
			delete(idx, sum)
			changed = true
		}
	}
	if changed {
		_ = u.savePhotoHash(idx)
	}
}

func sha256Hex(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func cleanUploadName(h *multipart.FileHeader) string {
	name := filepath.Base(strings.ReplaceAll(h.Filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, name)
	// 空白（含全角空格）统一压成连字符：URL 与 Markdown 都不容忍空格
	name = whitespaceRe.ReplaceAllString(name, "-")
	name = multiDashRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" || name == "." || strings.HasPrefix(name, ".") {
		name = "image"
	}
	if !imageExts[strings.ToLower(filepath.Ext(name))] {
		name += ".png"
	}
	return name
}

var (
	whitespaceRe = regexp.MustCompile(`[\s\x{A0}\x{3000}]+`)
	multiDashRe  = regexp.MustCompile(`-{2,}`)
)

func (u *UserApp) uniquePhotoName(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 0; i <= 999; i++ {
		cand := base + ext
		if i > 0 {
			cand = fmt.Sprintf("%s-%d%s", base, i+1, ext)
		}
		if _, err := os.Stat(filepath.Join(u.PhotoDir(), cand)); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().Unix(), ext)
}

// ---------- 复制文章 ----------

// buildCopyText 生成用于剪贴板的 Markdown：图床图片保留 URL，本地图片仅保留文件名。
func (u *UserApp) buildCopyText(art Article) (string, []string, []string) {
	var local, cloud []string
	text := mdImageRe.ReplaceAllStringFunc(art.Content, func(m string) string {
		sub := mdImageRe.FindStringSubmatch(m)
		alt, src := sub[1], sub[2]
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			cloud = append(cloud, src)
			return m
		}
		name := filepath.Base(strings.ReplaceAll(src, "\\", "/"))
		if name == "" {
			return m
		}
		local = append(local, name)
		return fmt.Sprintf("![%s](%s)", alt, name)
	})
	return text, local, cloud
}
