package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

// localUserKey 未携带身份时的兜底用户（局域网直连 TCP 端口时）。
const localUserKey = "local"

// User 登录用户身份。来自飞牛统一网关的可信请求头（仅 Unix socket 来源会被采信）。
type User struct {
	Key     string // 用于目录名的安全标识
	Name    string // 展示名（原始用户名）
	ID      string // X-Trim-Userid
	IsAdmin bool   // X-Trim-Isadmin
	Source  string // gateway=经飞牛网关；local=局域网直连
}

// userFromCtx 取当前请求的用户应用视图。
func userFromCtx(r *http.Request) *UserApp {
	if ua, ok := r.Context().Value(ctxUserKey).(*UserApp); ok && ua != nil {
		return ua
	}
	return nil
}

func prefixFromCtx(r *http.Request) string {
	if p, ok := r.Context().Value(ctxPrefixKey).(string); ok {
		return p
	}
	return ""
}

type ctxKey int

const (
	ctxUserKey   ctxKey = 1
	ctxPrefixKey ctxKey = 2
)

// sanitizeUserKey 把用户名清洗为安全的目录名（防路径穿越）。
func sanitizeUserKey(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 64 {
			break
		}
	}
	s := strings.Trim(b.String(), "-._")
	if s == "" || s == "." || s == ".." {
		s = localUserKey
	}
	return s
}

// isUnixConn 判断请求是否来自 Unix socket（飞牛网关转发）。
// TCP 监听与 Unix 监听的 LocalAddr 类型不同，可作可靠区分。
func isUnixConn(r *http.Request) bool {
	if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		_, isUnix := la.(*net.UnixAddr)
		return isUnix
	}
	return false
}

// resolveUser 解析请求身份：仅 Unix socket 来源采信 X-Trim-* 头，TCP 直连一律按本地用户。
func (a *App) resolveUser(r *http.Request) *User {
	if isUnixConn(r) {
		name := strings.TrimSpace(r.Header.Get("X-Trim-Username"))
		if name != "" {
			flag := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Trim-Isadmin")))
			return &User{
				Key:     sanitizeUserKey(name),
				Name:    name,
				ID:      strings.TrimSpace(r.Header.Get("X-Trim-Userid")),
				IsAdmin: flag == "true" || flag == "1" || flag == "yes",
				Source:  "gateway",
			}
		}
	}
	return &User{Key: localUserKey, Name: "本地直连", Source: "local", IsAdmin: true}
}

// ---------- 每用户应用视图 ----------

// UserApp 单个登录用户的应用视图：独立配置与存储，共享 App 的系统级上下文。
type UserApp struct {
	*App
	user *User

	mu  sync.RWMutex
	cfg Config
}

// forUser 取（或懒创建）指定用户的应用视图。
func (a *App) forUser(u *User) *UserApp {
	a.usersMu.RLock()
	ua, ok := a.users[u.Key]
	a.usersMu.RUnlock()
	if ok {
		return ua
	}

	a.usersMu.Lock()
	defer a.usersMu.Unlock()
	if existing, ok := a.users[u.Key]; ok {
		return existing
	}
	ua = newUserApp(a, u)
	a.users[u.Key] = ua
	return ua
}

func newUserApp(a *App, u *User) *UserApp {
	ua := &UserApp{App: a, user: u}
	ua.loadConfig()
	ua.ensureStorage()
	return ua
}

// User 返回当前用户身份。
func (u *UserApp) User() *User { return u.user }

func (u *UserApp) userDir() string { return filepath.Join(u.VarDir, "users", u.user.Key) }

// DefaultStorageRoot 该用户的默认存储根目录（应用数据目录下按账号隔离）。
func (u *UserApp) DefaultStorageRoot() string { return filepath.Join(u.userDir(), "data") }

func (u *UserApp) configPath() string { return filepath.Join(u.userDir(), "config.json") }

// legacyConfigPath / legacyDataRoot 升级前的共享配置与数据位置。
func (u *UserApp) legacyConfigPath() string { return filepath.Join(u.EtcDir, "config.json") }
func (u *UserApp) legacyDataRoot() string   { return filepath.Join(u.VarDir, "data") }

// legacyAdoptedMarker 历史数据认领标记：只允许第一位管理员/本地直连继承，
// 避免多位管理员各自指向同一份旧数据、破坏账号隔离。
func (u *UserApp) legacyAdoptedMarker() string {
	return filepath.Join(u.VarDir, ".legacy-adopted")
}

func (u *UserApp) loadConfig() {
	u.cfg.StorageRoot = u.DefaultStorageRoot()
	if data, err := os.ReadFile(u.configPath()); err == nil {
		var c Config
		if json.Unmarshal(data, &c) == nil {
			if strings.TrimSpace(c.StorageRoot) != "" {
				u.cfg = c
			} else {
				// 空路径表示用默认目录；云床与外观配置照常继承
				u.cfg.StorageRoot = u.DefaultStorageRoot()
				u.cfg.Cloudinary = c.Cloudinary
				u.cfg.Appearance = c.Appearance
			}
			// 外观为主题打包模型：旧配置（按字段微调）加载时迁移并落盘
			if migrated := sanitizeAppearance(u.cfg.Appearance); !reflect.DeepEqual(migrated, u.cfg.Appearance) {
				u.cfg.Appearance = migrated
				if err := u.saveConfig(); err != nil {
					log.Printf("迁移用户 %s 的外观配置失败: %v", u.user.Name, err)
				}
			}
			// 自定义存储路径下按用户建子文件夹：旧版数据平铺在路径根下，加载时下移
			u.adoptUserSubfolder()
			return
		}
		log.Printf("用户 %s 的配置损坏，按默认处理", u.user.Name)
	}
	// 首次使用：管理员或本地直连继承升级前的共享配置与数据，保证历史文章/图片不丢
	if u.user.IsAdmin || u.user.Source == "local" {
		u.adoptLegacy()
		// 继承来的共享路径同样套用户子文件夹：
		// 旧数据平铺在路径根下时当次会话即下移，避免升级后文章看似丢失
		u.adoptUserSubfolder()
	}
}

// adoptLegacy 把升级前的共享配置搬为当前用户的配置。
// 兼容两种历史形态：旧版保存过 config.json（读取其中的存储路径/图床配置），
// 或从未保存过设置但旧默认数据目录里确有内容（直接指向该目录，文章图片原样可见）。
func (u *UserApp) adoptLegacy() {
	if _, err := os.Stat(u.legacyAdoptedMarker()); err == nil {
		return // 已被其他管理员认领，后续管理员各用各的新目录
	}

	var c Config
	if data, err := os.ReadFile(u.legacyConfigPath()); err == nil {
		if json.Unmarshal(data, &c) != nil {
			log.Printf("历史配置解析失败，按默认处理: %v", err)
		}
	} else if !os.IsNotExist(err) {
		log.Printf("读取历史配置失败(%s): %v", u.user.Name, err)
	}

	adopted := false
	switch {
	case strings.TrimSpace(c.StorageRoot) != "":
		u.cfg.StorageRoot = c.StorageRoot
		adopted = true
	case dirHasContent(u.legacyDataRoot()):
		// 旧配置用默认路径且确有数据：显式指向旧数据目录，内容原样可见
		u.cfg.StorageRoot = u.legacyDataRoot()
		adopted = true
	}
	if !adopted && c.Cloudinary == (CloudConfig{}) {
		return // 没有可继承的内容
	}
	u.cfg.Cloudinary = c.Cloudinary

	if err := u.saveConfig(); err != nil {
		log.Printf("继承历史配置失败(%s): %v", u.user.Name, err)
		return
	}
	// 认领成功后落标记，防止其他管理员重复继承
	if err := os.WriteFile(u.legacyAdoptedMarker(), []byte(u.user.Key), 0o644); err != nil {
		log.Printf("写入历史认领标记失败: %v", err)
	}
}

func dirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// adoptUserSubfolder 自定义存储路径下按用户建子文件夹。
// 旧版数据平铺在路径根下（passage/photo/seo 直接在根目录），加载时整体下移到
// 路径根下的用户子文件夹中；已是新布局或无从迁移内容时静默跳过。
func (u *UserApp) adoptUserSubfolder() {
	root := u.cfg.StorageRoot
	if strings.TrimSpace(root) == "" || root == u.DefaultStorageRoot() {
		return
	}
	sub := filepath.Join(root, u.user.Key)
	if _, err := os.Stat(filepath.Join(sub, dirPassage)); err == nil {
		return // 已是按用户分子文件夹的新布局
	}
	for _, d := range []string{dirPassage, dirPhoto, dirSEO, dirMeta} {
		if _, err := os.Stat(filepath.Join(root, d)); err != nil {
			continue
		}
		log.Printf("用户 %s 的存储数据下移到用户子文件夹：%s -> %s", u.user.Name, root, sub)
		if err := moveStorageDown(root, sub); err != nil {
			log.Printf("下移存储数据失败(%s): %v", u.user.Name, err)
		}
		return
	}
}

// moveStorageDown 把存储根下的四类内容搬到 root 内的用户子文件夹 sub。
// 与 migrateStorage 的区别：不回避包含关系（sub 本就在 root 内），
// 逐个文件搬迁，旧目录搬空后才移除。
func moveStorageDown(root, sub string) error {
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return err
	}
	for _, name := range []string{dirPassage, dirPhoto, dirSEO, dirMeta} {
		src := filepath.Join(root, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(sub, name)
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
			if err := moveFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		if rest, err := os.ReadDir(src); err == nil && len(rest) == 0 {
			_ = os.Remove(src)
		}
	}
	return nil
}

func (u *UserApp) saveConfig() error {
	if err := os.MkdirAll(u.userDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(u.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.configPath(), data, 0o600)
}

// StorageRoot 当前用户的有效数据根目录（文章 / 图片 / SEO 都放在它下面）。
// 未自定义时用每用户默认目录 users/<用户>/data；
// 自定义了共享存储路径时，数据落在该路径下以用户标识命名的子文件夹中，
// 多个账号指向同一路径也各自独立、互不覆盖。
func (u *UserApp) StorageRoot() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.effectiveRoot(u.cfg.StorageRoot)
}

// StorageBase 用户自定义的共享存储路径（未自定义时为空串）。
// 配置里显式存了默认路径（旧版本落盘或恢复默认后的形态）同样视为未自定义。
func (u *UserApp) StorageBase() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	configured := strings.TrimSpace(u.cfg.StorageRoot)
	if configured == "" || configured == u.DefaultStorageRoot() {
		return ""
	}
	return configured
}

// effectiveRoot 把配置里的存储路径折算为有效数据根目录。
// 默认路径本身已含用户目录，不再套一层；自定义路径下按用户建子文件夹。
func (u *UserApp) effectiveRoot(configured string) string {
	if strings.TrimSpace(configured) == "" || configured == u.DefaultStorageRoot() {
		return u.DefaultStorageRoot()
	}
	return filepath.Join(configured, u.user.Key)
}

// Cloudinary 当前用户的图床配置。
func (u *UserApp) Cloudinary() CloudConfig {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cfg.Cloudinary
}

// setCloudinary 更新图床配置并持久化。
func (u *UserApp) setCloudinary(c CloudConfig) error {
	u.mu.Lock()
	u.cfg.Cloudinary = c
	err := u.saveConfig()
	u.mu.Unlock()
	return err
}

// setStorageRoot 更新存储路径并持久化（存的是用户选择的共享路径原文，
// 空串或默认路径表示使用每用户默认目录）。
func (u *UserApp) setStorageRoot(root string) error {
	u.mu.Lock()
	u.cfg.StorageRoot = root
	err := u.saveConfig()
	u.mu.Unlock()
	return err
}

// Appearance 当前用户的外观配置。
func (u *UserApp) Appearance() AppearanceConfig {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cfg.Appearance
}

// setAppearance 更新外观配置并持久化（会做字段清洗与体积封顶）。
func (u *UserApp) setAppearance(a AppearanceConfig) error {
	a = sanitizeAppearance(a)
	u.mu.Lock()
	u.cfg.Appearance = a
	err := u.saveConfig()
	u.mu.Unlock()
	return err
}

func (u *UserApp) PassageDir() string { return filepath.Join(u.StorageRoot(), dirPassage) }
func (u *UserApp) PhotoDir() string   { return filepath.Join(u.StorageRoot(), dirPhoto) }
func (u *UserApp) SEODir() string     { return filepath.Join(u.StorageRoot(), dirSEO) }
func (u *UserApp) MetaDir() string    { return filepath.Join(u.StorageRoot(), dirMeta) }

func (u *UserApp) ensureStorage() {
	root := u.StorageRoot()
	for _, d := range []string{u.userDir(), root, u.PassageDir(), u.PhotoDir(), u.SEODir(), u.MetaDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Printf("警告：无法创建目录 %s: %v", d, err)
		}
	}
}

// allowedRoots 允许被浏览/写入的根目录集合（系统授权 + 当前用户存储）。
func (u *UserApp) allowedRoots() []string {
	roots := append([]string{}, u.AuthPaths...)
	roots = append(roots, u.DefaultStorageRoot(), u.StorageRoot())
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// withinAllowed 判断 path 是否位于当前用户允许的根目录之内。
func (u *UserApp) withinAllowed(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	for _, root := range u.allowedRoots() {
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		if abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
