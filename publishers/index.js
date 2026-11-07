/* 发布适配器清单
 *
 * 水鱼文集「设置 → 更新与实验 → 实验性功能」读取本文件，
 * 按需下载对应适配器——适配器代码不内置在应用内，可独立更新。
 *
 * 新增一个适配器只需两步：
 *   1. 把适配器 .js 放进本目录（契约见 README.md）
 *   2. 在下面数组中登记：id / name / desc / file / version
 *
 * 注意：数组必须是标准 JSON（键名带双引号），应用按 JSON 解析。
 */
window.waterfishPublishers = [
  {
    "id": "notionnext",
    "name": "NotionNext",
    "desc": "发布到 NotionNext 等基于 Notion 数据库的博客系统（标题 / 标签 / 状态 / 正文）",
    "file": "notionnext.js",
    "version": "1.0.0"
  },
  {
    "id": "wordpress",
    "name": "WordPress",
    "desc": "通过 WordPress REST API 发布文章（标题 / 标签 / 正文 / 摘要），可存草稿或直接发布",
    "file": "wordpress.js",
    "version": "1.0.0"
  }
];
