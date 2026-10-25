package main

import (
	"fmt"
	"net/http"
	"strings"
)

// ---------- 外观 ----------

const (
	maxThemeJSSize   = 64 * 1024 // 单个自定义主题脚本体积上限
	maxThemeNameSize = 64        // 自定义主题名称长度上限
	maxCustomThemes  = 20        // 自定义主题数量上限
)

// builtinThemes 内置主题 id（与前端 app.js 的 THEMES 表保持一致）。
var builtinThemes = []string{"default", "amber", "forest", "violet", "rose"}

func isBuiltinTheme(id string) bool {
	for _, t := range builtinThemes {
		if t == id {
			return true
		}
	}
	return false
}

// sanitizeThemeName 清洗主题名：去首尾空白、压平内部空白、剔除路径分隔符与控制字符。
func sanitizeThemeName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return -1
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "自定义主题"
	}
	// 折叠连续点并去掉首尾点，避免出现 ".." 这类可疑名称
	for strings.Contains(name, "..") {
		name = strings.ReplaceAll(name, "..", ".")
	}
	name = strings.Trim(name, ".")
	if name == "" {
		name = "自定义主题"
	}
	runes := []rune(name)
	if len(runes) > maxThemeNameSize {
		runes = runes[:maxThemeNameSize]
	}
	return string(runes)
}

// dedupeThemeNames 给重名主题加 (2)、(3)… 后缀，保证主题名唯一。
func dedupeThemeNames(themes []CustomTheme) []CustomTheme {
	used := make(map[string]bool, len(themes))
	out := make([]CustomTheme, 0, len(themes))
	for _, t := range themes {
		name := t.Name
		if used[name] {
			for i := 2; ; i++ {
				cand := fmt.Sprintf("%s(%d)", name, i)
				if !used[cand] {
					name = cand
					break
				}
			}
		}
		used[name] = true
		t.Name = name
		out = append(out, t)
	}
	return out
}

func hasTheme(themes []CustomTheme, name string) bool {
	for _, t := range themes {
		if t.Name == name {
			return true
		}
	}
	return false
}

// sanitizeAppearance 清洗外观配置：
//   - 迁移更早版本的按字段微调配置（折算为内置主题 + 单条自定义主题）
//   - 模式白名单；当前主题须为内置 id 或已存在的自定义主题，否则回落默认
//   - 自定义主题列表：清洗名称、去重、脚本截断到体积上限
func sanitizeAppearance(a AppearanceConfig) AppearanceConfig {
	switch a.Mode {
	case "light", "dark":
	default:
		a.Mode = "auto"
	}

	// 旧版迁移：没有主题选择时，按旧预设折算；旧的自定义脚本收编为一条自定义主题
	if a.Theme == "" {
		a.Theme = a.LegacyPreset
		if strings.TrimSpace(a.LegacyCustomJS) != "" {
			name := sanitizeThemeName(a.LegacyCustomName)
			a.Themes = append(a.Themes, CustomTheme{Name: name, JS: a.LegacyCustomJS})
			a.Theme = name // 旧版上传后自定义主题即处于生效状态
		}
	}
	a.LegacyPreset, a.LegacyAccent, a.LegacyFontFamily = "", "", ""
	a.LegacyFontScale, a.LegacyBgStyle = 0, ""
	a.LegacyCustomJS, a.LegacyCustomName = "", ""

	themes := make([]CustomTheme, 0, len(a.Themes))
	for _, t := range a.Themes {
		if strings.TrimSpace(t.JS) == "" || len(themes) >= maxCustomThemes {
			continue
		}
		t.Name = sanitizeThemeName(t.Name)
		if len(t.JS) > maxThemeJSSize {
			t.JS = t.JS[:maxThemeJSSize]
		}
		themes = append(themes, t)
	}
	a.Themes = dedupeThemeNames(themes)

	if !isBuiltinTheme(a.Theme) && !hasTheme(a.Themes, a.Theme) {
		a.Theme = "default"
	}
	return a
}

// handleThemeJS 下发当前用户生效中的自定义主题脚本；未启用自定义主题时返回 204。
func (a *App) handleThemeJS(w http.ResponseWriter, r *http.Request) {
	ua := userFromCtx(r)
	if ua == nil {
		writeErr(w, http.StatusUnauthorized, "未识别到登录用户")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	ap := ua.Appearance()
	js := ""
	for _, t := range ap.Themes {
		if t.Name == ap.Theme {
			js = t.JS
			break
		}
	}
	if strings.TrimSpace(js) == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(js))
}
