# spool · 内部 AI 网关品牌资产

全部通过 Sub2API 管理端设置落地，不改代码。

| 文件 | 用途 |
| --- | --- |
| `spool.svg` | 彩色标志，上传到「站点 Logo」。346 字节，viewBox 512，透明底，浅色/深色底都可用 |
| `spool-mono.svg` | 单色标志（currentColor），只用于文档、贴纸、内联 SVG；不要上传（`<img>` 里取不到页面颜色，会渲成黑色） |
| `home_content.html` | 首页内容，整段粘贴到「首页内容」。按项目技能 design-taste-frontend-v1（variance 8 / motion 6 / density 4）设计的深色编辑式非对称页，内联 CSS + CSS 动效、无 JS、限定在 `.spool-home` 作用域；页面自带深色底，应用切到浅色主题时它仍是深色，属有意为之 |
| `site-copy.txt` | 站点名称与副标题文案 |

## 上传步骤

1. 管理后台 → 系统设置 → 站点设置。
2. 站点名称填 `spool`（全小写）；副标题填 `site-copy.txt` 里那句。
3. 站点 Logo 上传 `spool.svg`（接受 SVG，≤300KB）。上传后前端把文件转成 data URL 保存，所有位置引用同一份。
4. 首页内容：把 `home_content.html` 里的地址换成实际地址后整段粘贴。内容不能以 `http://` 或 `https://` 开头（那会切成 iframe 模式）。
5. 保存后硬刷新（浏览器会缓存旧 favicon）。

## 粘贴前唯一要改的地方

把 `https://spool.corp.internal` 换成你实例的实际地址，共 3 处（hero 地址块、`ANTHROPIC_BASE_URL`、`OPENAI_BASE_URL`），以控制台 API Keys 页面显示的地址为准。其余文案已定稿：额度按人分配、出事找管理员、登录由管理员开通账号。

## 已按源码核实的行为描述（改文案时别写反）

- 额度用完：`429` + code `QUOTA_EXHAUSTED` / `USAGE_LIMIT_EXCEEDED`（Codex 路径为 `insufficient_quota`），重试无用。
- 限速：`429` + code `USER_RPM_EXCEEDED` / `GROUP_RPM_EXCEEDED`，退避重试。
- 余额不足：`403` + code `INSUFFICIENT_BALANCE`。
- 池里无可用账号：`503`。
- key 在 `/keys` 列表里随时可整条复制回来（不是只显示一次）。
- 建 key 时分组必填，`/v1/models` 只返回本 key 所属分组的模型。

## 注意事项

- 首页 HTML 里的 JavaScript 不会执行，所以没有复制按钮：base URL 与命令行用 `user-select:all`，点一下整行选中；`$` 提示符不会被选进去。
- 「请求流程」一节的转发日志是示例回放（已写明「非实时」），不是真实流量；动效全部走 transform/opacity，系统开启「减弱动态效果」时全部静止。
- Safari 不显示 SVG favicon（Chrome / Edge / Firefox 正常）。若 Safari 用户多，导出一张 256×256 PNG 代替上传。
- 模型广场导航栏的 logo 容器浅色模式是白色 tile、深色模式是 `#1e293b`，标志在两种底上都验过。
- 上线后核对：标签页 favicon、侧栏 36px、登录页 64px、模型广场导航栏、首页亮/暗与手机宽度、首页 base URL 与控制台一致、占位符已全部替换。
