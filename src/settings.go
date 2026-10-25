package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ---------- 状态 ----------

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	articles, _ := ua.listArticles()
	cfg := ua.Cloudinary()

	type rootInfo struct {
		Path     string `json:"path"`
		Label    string `json:"label"`
		Writable bool   `json:"writable"`
		Free     uint64 `json:"free"`
	}
	roots := []rootInfo{}
	for _, p := range ua.allowedRoots() {
		label := "授权目录"
		switch {
		case p == ua.StorageRoot():
			label = "当前存储路径"
		case p == ua.DefaultStorageRoot():
			label = "应用数据目录"
		}
		roots = append(roots, rootInfo{Path: p, Label: label, Writable: writable(p), Free: freeSpace(p)})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"appName": "水鱼文集",
		"version": a.Version,
		"port":    a.Port,
		"user": map[string]any{
			"name":    ua.user.Name,
			"isAdmin": ua.user.IsAdmin,
			"source":  ua.user.Source,
		},
		"storageRoot":        ua.StorageRoot(),
		"storageBase":        ua.StorageBase(),
		"defaultStorageRoot": ua.DefaultStorageRoot(),
		"authorizedPaths":    a.AuthPaths,
		"roots":              roots,
		"cloudinary":         cfg,
		"appearance":         ua.Appearance(),
		"stats": map[string]int{
			"articles":    len(articles),
			"localImages": len(ua.listLocalImages()),
			"cloudImages": len(ua.loadCloudIndex()),
			"seoRecords":  len(ua.listSEORecords()),
		},
	})
}

func freeSpace(path string) uint64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return st.Bavail * uint64(st.Bsize)
}

// ---------- 设置 ----------

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"storageRoot": ua.StorageRoot(),
			"cloudinary":  ua.Cloudinary(),
			"appearance":  ua.Appearance(),
		})
	case http.MethodPut:
		// 分段保存：请求体带哪段就更新哪段，未带的段保持原值
		var in struct {
			Cloudinary *CloudConfig      `json:"cloudinary"`
			Appearance *AppearanceConfig `json:"appearance"`
		}
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if in.Cloudinary != nil {
			// 图床凭证影响整机上传流向，仅管理员（或局域网直连）可修改
			if !ua.user.IsAdmin && ua.user.Source != "local" {
				writeErr(w, http.StatusForbidden, "仅管理员可修改图床设置")
				return
			}
			c := *in.Cloudinary
			c.CloudName = strings.TrimSpace(c.CloudName)
			c.UploadPreset = strings.TrimSpace(c.UploadPreset)
			if c.Enabled && (c.CloudName == "" || c.UploadPreset == "") {
				writeErr(w, http.StatusBadRequest, "启用图床需同时填写 cloudName 与 uploadPreset")
				return
			}
			if err := ua.setCloudinary(c); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if in.Appearance != nil {
			if err := ua.setAppearance(*in.Appearance); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if in.Cloudinary == nil && in.Appearance == nil {
			writeErr(w, http.StatusBadRequest, "没有需要保存的设置")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// ---------- 存储路径 ----------

func (a *App) handleSetStorage(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	// 存储路径影响整机数据布局，仅管理员（或局域网直连）可更改
	if !ua.user.IsAdmin && ua.user.Source != "local" {
		writeErr(w, http.StatusForbidden, "仅管理员可更改存储路径")
		return
	}
	var in struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	raw := strings.TrimSpace(in.Path)
	var newRoot string
	if raw == "" {
		// 恢复默认：回到每用户默认目录
		newRoot = ua.DefaultStorageRoot()
	} else {
		abs, err := filepath.Abs(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "路径不合法")
			return
		}
		if !ua.withinAllowed(abs) {
			writeErr(w, http.StatusForbidden,
				"该目录不在授权范围内。请先前往「应用中心 → 水鱼文集 → 设置 → 授权目录」添加该文件夹，再回到本页选择。")
			return
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			writeErr(w, http.StatusInternalServerError, "无法创建目标目录："+err.Error())
			return
		}
		if !writable(abs) {
			writeErr(w, http.StatusForbidden, "目标目录不可写，请检查文件夹权限")
			return
		}
		// 自定义路径下按用户建子文件夹，多账号共存互不覆盖
		newRoot = filepath.Join(abs, ua.user.Key)
	}

	old := ua.StorageRoot()
	if newRoot != old {
		if err := migrateStorage(old, newRoot); err != nil {
			writeErr(w, http.StatusInternalServerError, "迁移文件失败："+err.Error())
			return
		}
		if err := ua.setStorageRoot(raw); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	ua.ensureStorage()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "storageRoot": ua.StorageRoot()})
}

// migrateStorage 把旧存储根下的内容搬到新存储根。
func migrateStorage(oldRoot, newRoot string) error {
	if _, err := os.Stat(oldRoot); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// 新旧目录存在包含关系时不做搬迁，避免边遍历边写入
	if isInside(newRoot, oldRoot) || isInside(oldRoot, newRoot) {
		return nil
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		return err
	}
	for _, sub := range []string{dirPassage, dirPhoto, dirSEO, dirMeta} {
		src := filepath.Join(oldRoot, sub)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(newRoot, sub)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			from := filepath.Join(src, e.Name())
			to := uniquePath(dst, e.Name())
			if err := moveFile(from, to); err != nil {
				return err
			}
		}
		// 仅当旧目录已空时才移除，避免误删
		if rest, err := os.ReadDir(src); err == nil && len(rest) == 0 && src != newRoot {
			_ = os.Remove(src)
		}
	}
	return nil
}

// isInside 判断 path 是否位于 root 之内（不含 root 自身）。
func isInside(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.HasPrefix(absPath, absRoot+string(os.PathSeparator))
}

func uniquePath(dir, name string) string {
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return target
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i <= 999; i++ {
		cand := filepath.Join(dir, base+"-"+strconv.Itoa(i)+ext)
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
	return filepath.Join(dir, base+"-"+strconv.FormatInt(time.Now().Unix(), 10)+ext)
}

func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	// 跨设备时回退为复制 + 删除
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(from)
}

// ---------- 目录浏览 ----------

func (a *App) handleBrowse(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	reqPath := strings.TrimSpace(r.URL.Query().Get("path"))

	type dirItem struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	if reqPath == "" {
		items := []dirItem{}
		for _, p := range ua.allowedRoots() {
			items = append(items, dirItem{Name: filepath.Base(p) + "  (" + p + ")", Path: p})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path": "", "parent": "", "roots": true,
			"dirs": items, "writable": false,
		})
		return
	}

	abs, err := filepath.Abs(reqPath)
	if err != nil || !ua.withinAllowed(abs) {
		writeErr(w, http.StatusForbidden, "无权访问该目录")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "无法读取目录："+err.Error())
		return
	}
	dirs := []dirItem{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, dirItem{Name: e.Name(), Path: filepath.Join(abs, e.Name())})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })

	parent := filepath.Dir(abs)
	if parent == abs || !ua.withinAllowed(parent) {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": abs, "parent": parent, "roots": false,
		"dirs": dirs, "writable": writable(abs), "free": freeSpace(abs),
	})
}
