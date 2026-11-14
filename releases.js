window.waterfishRelease = {
  "version": "1.2.4",
  "date": "2026-11-14",
  "notes": "## v1.2.4 更新\n\n### 主题系统升级（简单 / 复杂两种写法）\n- **简单主题**：一行一条声明（主色 / 副色 / 背景 / 字体…）\n- **复杂主题**：文件内定义 `function theme(api)`，通过 `api.setVar / api.css / api.icon / api.dark / api.el` 调整界面样式、动画与图标；每次应用先清理再执行，脚本出错只回退不崩页面\n- 外观抽屉提供「简单示例 / 复杂示例」一键下载\n\n### 侧栏布局调整\n- 收起侧栏按钮移到**顶部**品牌行，账号与外观（用户卡片）内置在**侧栏底部**，所有页面可用\n- 点用户卡片从左侧滑出「外观」抽屉：主题模式、主题卡片、自定义主题都在里面\n\n### 发布体系\n- 各平台密钥统一在「设置 → 发布设置 → 平台密钥」配置，仅保存在自己的账号下\n- 发布弹窗只设置**封面、标题、标签**这类每篇文章不同的内容，一次勾选多平台统一发布\n- 平台适配器的安装 / 卸载仅管理员可操作（管理员设置），适配器托管在项目仓库按需下载，不内置在安装包里\n\n### 关于与更新合并\n- 「更新」与「关于」合并为一个子菜单，检查更新与版本信息在同一页\n\n### SEO 检测 → 网页检测\n- **在本地检测网页**的质量（SEO 与 AI Agent 就绪度），全程离线，不访问网络、不上传内容\n- 检查项：标题 / 摘要 / 标题层级 / 图片 alt / 锚文本 / 字数等 SEO 组 + 语义结构 / 内容文本化 / 代码块语言标注等 Agent 就绪组\n- 输出 0–100 评分与逐条「通过 / 建议 / 问题」清单\n\n**安装包**：x86 架构请下载 `waterfish_anthology_x86.fpk`，ARM 架构请下载 `waterfish_anthology_arm.fpk`。",
  "x86": "https://raw.githubusercontent.com/12hgl/waterfish_anthology/latest/waterfish_anthology_x86.fpk",
  "arm": "https://raw.githubusercontent.com/12hgl/waterfish_anthology/latest/waterfish_anthology_arm.fpk",
  "release": "https://github.com/12hgl/waterfish_anthology/releases/tag/v1.2.4"
};
