# 上架自查（对照飞牛官方文档）

对照 [飞牛应用开放平台文档](https://developer.fnnas.com/docs/guide) 逐项检查本应用包。
标 ✅ 为已满足，⚠️ 为有前提或需在真机复核，❌ 为未做。

## 1. 包结构

| 项 | 要求 | 现状 |
| --- | --- | --- |
| 目录结构 | `manifest` + `app/` + `cmd/` + `config/` + `wizard/` + 包图标 | ✅ 见 `packaging/` |
| 打包工具 | 官方 `fnpack` | ✅ `build.sh` 固定 fnpack 1.2.3，`fnpack build --directory` |
| 架构 | 双架构分别出包 | ✅ `platform` 分别写 `x86` / `arm`，二进制交叉编译 `CGO_ENABLED=0` |
| 图标 | 包图标 64 / 256 | ✅ `ICON.PNG` 64×64、`ICON_256.PNG` 256×256（RGBA） |
| 入口图标 | `app/ui/images/icon_{0}.png` | ✅ `icon_64.png` / `icon_256.png` |

## 2. manifest

| 字段 | 现状 | 说明 |
| --- | --- | --- |
| `appname` | `waterfish_anthology` | 唯一标识 |
| `version` | `1.1.0` | 构建脚本按 `VERSION` 注入 |
| `display_name` / `desc` | ✅ | 描述含实际能力，无占位文案 |
| `source` | `thirdparty` | |
| `platform` | `x86` / `arm` | 分架构构建 |
| `maintainer` / `maintainer_url` | ✅ | 指向项目仓库 |
| `os_min_version` | `1.0.0` | ⚠️ 核心功能（文章库 / 编辑器 / 图片库）在 1.0.0 可用；「文件关联授权」与「语义化路径」需 1.2.0401+，低版本走手动授权降级（见第 6 节） |
| `desktop_uidir` / `desktop_applaunchname` | `ui` / `waterfish_anthology.main` | 与入口 ID 一致 |
| `service_port` / `checkport` | `8631` / `true` | 仅本机回环，见第 7 节 |
| `ctl_stop` | `true` | 有常驻服务，需要启停 |
| `install_type` | 未设置 | 交由用户选择安装位置（数据落在用户存储空间） |
| `micro_app` | `true` | 前端需调用开放平台 JS SDK（文件/目录授权） |
| `changelog` | ✅ | 面向用户描述，含版本前提说明 |

## 3. 应用入口（`packaging/app/ui/config`）

| 项 | 现状 |
| --- | --- |
| 桌面入口 | `waterfish_anthology.main`，`type=iframe`，统一网关（`gatewayPrefix=/app/waterfish_anthology` + `gatewaySocket=app.sock`），`allUsers=true` |
| 文件打开方式 | `waterfish_anthology.edit`，`fileTypes=["md","markdown","mdown","mdx","txt"]`，`noDisplay=true`，同一网关 |
| 入口 ID 前缀 | 均以 `appname` 开头 ✅ |
| 只声明实际支持的类型 | ✅ 仅 Markdown 系与纯文本；不抢 `.docx` / `.pdf` 等 |
| `path` 参数处理 | ✅ 视为不可信输入：绝对路径校验 → 授权范围校验 → 用户 ACL 复核（`trim.file.checkUserACL`）→ OS ACL 兜底 |

## 4. 运行身份与权限

| 项 | 现状 |
| --- | --- |
| `config/privilege` | `run-as=package`，专用用户/组 `waterfish_anthology`，非 root ✅ |
| `join-groups` | 未使用（不需要设备/GPU） ✅ |
| 用户文件访问 | 默认零访问；由用户在系统弹窗中授权目录/文件（`authorizeUserFile` / `pickUserFile`），或管理员在「应用设置 → 授权目录」添加 ✅ |
| 授权持久化 | 记录在 `${TRIM_PKGVAR}/users/<用户>/grants.json`，仅存路径；可在系统侧随时移除 ✅ |

## 5. 资源与开放接口

`packaging/config/resource`：

```json
{ "api-scope": ["trim.file.userAccess", "trim.file.userAcl", "trim.file.path"] }
```

| Scope | 用途 | 对应接口 |
| --- | --- | --- |
| `trim.file.userAccess` | 用户选择/授权目录与文件 | `pickUserFile`、`authorizeUserFile`（前端 SDK） |
| `trim.file.userAcl` | 返回内容或写入前复核当前用户权限 | `trim.file.checkUserACL`（后端） |
| `trim.file.path` | 路径语义化展示，不直接暴露 `/vol1/...` | `trim.file.convertPath`（后端） |

不需要的 scope 未声明（未使用共享授权 `trim.file.sharedAccess`）。
后端调用统一经 `/var/run/trim_open_gateway_apiscope.socket`，`TRIM_API_TOKEN` 每次从环境变量读取，不落盘、不下发前端 ✅。

## 6. 依赖不可用时的降级

| 场景 | 行为 |
| --- | --- |
| 系统版本低 / 无开放接口（socket 或 token 缺失） | 后端 `checkUserACL` 退回本地探测；语义路径为空时前端展示原始路径；授权入口在设置页的手动授权目录仍可用 |
| 页面不在宿主中（独立浏览器） | `isStandaloneWeb=true` 时给出「请从飞牛桌面打开」提示，不静默失败 |
| 网络不可用 | 更新检测 / 适配器下载失败均返回中文可操作提示，不阻塞主流程 |
| 文件无写权限 | 顶部横幅显示「只读」，保存返回明确原因 |

## 7. 安全取舍

| 项 | 现状 | 原因 |
| --- | --- | --- |
| TCP 端口绑定 | **仅 `127.0.0.1`**（`WF_LISTEN` 可覆盖） | 端口直连请求不带 NAS 登录态，若对外监听，局域网内任何人都会以本地管理员身份操作数据 |
| Unix socket 权限 | `0666`（网关转发所需） | ⚠️ 依赖系统网关只转发已鉴权请求；本机同用户进程理论上可伪造身份头，属平台通用模型 |
| 路径穿越 | `filepath.Clean` + 绝对路径校验 + 符号链接解析后比较 | 见 `resolveExternalPath` / `withinAllowed` |
| 内容安全 | 前端统一 `esc()` 转义，Markdown 渲染走本地 sanitize | 见 `web/ui.js`、`web/markdown.js` |
| 上传体积 | 图片与文本均设上限（文本 8 MB，适配器 256 KB） | 防误传大文件拖垮服务 |

## 8. 测试覆盖

| 类型 | 内容 |
| --- | --- |
| Go 单元测试 | 版本比较、releases.js 解析、适配器元数据、路径校验、frontmatter 保真、外部文件读写与导入 |
| 浏览器回归 | 主流程、编辑器、外观与设置、存储路径、代码块/离开确认、网关前缀、更新面板、发布功能、文件关联与导入 |
| 适配器端到端 | NotionNext 适配器（mock Notion API）：属性映射、块转换、分块、错误提示 |
| 打包验证 | 解包核对 manifest 字段、入口配置、`api-scope`、二进制特性与版本号 |

## 9. 上架前仍需在真机确认

1. 文件管理器「打开方式」是否按 `fileTypes` 出现在 `.md` 的菜单中，且 `?path=` 正常追加。
2. 授权弹窗文案与范围是否符合预期（授权到文件所在文件夹）。
3. 系统 ≥ 1.2.0401 与 < 1.2.0401 两种环境下的降级路径。
4. 桌面图标、入口图标在深/浅色主题下的显示效果。
5. 应用中心「设置 → 授权目录」中手动添加目录后，`config_callback` 自动重启是否生效。
