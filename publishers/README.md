# 发布适配器（publishers）

「文章发布（Beta）」的适配器代码仓库。适配器**不内置在应用内**，由应用在
「设置 → 更新与实验 → 实验性功能」中按需下载（GitHub 直连失败时自动走
gh-proxy 加速），下载后配置密钥即可在文章编辑页发布。

## 目录结构

```
publishers/
├── index.js          # 适配器清单（应用启动时读取，决定「可下载」列表）
├── notionnext.js     # NotionNext 适配器
└── <your>.js         # 新适配器放这里
```

`index.js` 的数组必须是**标准 JSON**（键名带双引号），应用按 JSON 解析：

```js
window.waterfishPublishers = [
  { "id": "notionnext", "name": "NotionNext", "desc": "…", "file": "notionnext.js", "version": "1.0.0" }
];
```

## 适配器契约

每个适配器是一个独立 JS 文件，加载后向 `window.WFPublishers` 注册一个对象：

```js
window.WFPublishers.<id> = {
  id: '<id>',                 // 与 index.js 登记一致（仅字母数字 - _ .）
  name: '展示名',
  desc: '一句话说明',
  version: '1.0.0',

  // 配置项：发布前由用户填写，仅保存在本机
  configFields: [
    { key: 'token', label: 'API 令牌', type: 'password', required: true, hint: '…' },
    { key: 'databaseId', label: '数据库 ID', type: 'text', required: true, hint: '…' },
    { key: 'statusValue', label: '发布状态', type: 'text', required: false, hint: '默认 Published' },
  ],

  // 发布动作：返回 { url }（发布后的页面地址，可省略）
  publish: async function (payload, config) {
    // payload = { title, tags, markdown, excerpt, date }
    // config  = { token: '…', databaseId: '…', … }（对应 configFields）
    // 成功返回 { url: 'https://…' }；失败 throw new Error('中文提示')
    return { url: 'https://example.com/post' };
  },
};
```

### payload 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `title` | string | 文章标题 |
| `tags` | string[] | 标签数组（可能为空） |
| `markdown` | string | 正文 Markdown；本地图片已被替换为公网 URL（图床可用时） |
| `excerpt` | string | 纯文本摘要（约 120 字） |
| `date` | string | ISO 8601 时间戳 |

### 运行环境与注意事项

- 适配器运行在**用户的浏览器**里（经应用页面 `<script>` 加载），可直接 `fetch`
  第三方 API；目标 API 需允许跨域（返回 `Access-Control-Allow-Origin`）。
  网络路径是「用户电脑 → 目标 API」，不经过 NAS。
- 体积上限 256 KiB；文件必须包含 `window.WFPublishers` 与 `publish` 字样，
  否则应用会拒绝安装。
- 密钥（token 等）只保存在用户本机配置文件中，**不要**在适配器里写死任何凭证。
- 错误提示用中文、面向用户可操作（如「令牌无效，请检查后重试」）。

## 新增一个适配器的步骤

1. 编写 `<id>.js`（文件名与 id 一致），实现上述契约；
2. 在 `index.js` 数组中登记；
3. 提交推送——用户在应用里点「下载」即可使用（无需升级应用）。
