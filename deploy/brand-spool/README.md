# spool · 内部 AI 网关品牌资产

从这个版本起，默认站名、副标题、标志和页面标题已直接编进前后端（`spool` / `Claude / Codex 模型网关` / `frontend/public/logo.svg`），管理端不再需要上传 logo 或改站名；只有「首页内容」仍通过管理端设置粘贴。教程与博客是构建进前端的静态页面：`/docs.html`、`/blog.html`。

| 文件 | 用途 |
| --- | --- |
| `spool.svg` | 绿色版标志（早期品牌色）。当前默认 logo 是赤陶配色的 `frontend/public/logo.svg`，与首页配色一致；想换回绿色版就把它上传到「站点 Logo」 |
| `spool-mono.svg` | 单色标志（currentColor），只用于文档、贴纸、内联 SVG；不要上传（`<img>` 里取不到页面颜色，会渲成黑色） |
| `home_content.html` | 首页内容，整段粘贴到「首页内容」。按用户指定参考站 ddshub.cc 复刻的深色页（风格与文案照搬，去掉对方品牌与业务信息），内联 CSS、无 JS、限定在 `.spool-home` 作用域；页面自带深色底，应用切到浅色主题时它仍是深色，属有意为之 |
| `site-copy.txt` | 站点名称与副标题文案 |

## 上线步骤

1. 部署这个版本（站名、副标题、logo、标题已内置，无需设置）。
2. 管理后台 → 系统设置 → 站点设置 → 首页内容：把 `home_content.html` 整段粘贴（地址已固定为本实例域名，换域名时先全局替换）。内容不能以 `http://` 或 `https://` 开头（那会切成 iframe 模式）。
3. 保存后硬刷新。

## 粘贴前唯一要改的地方

地址已固定为本实例的 `https://rq.yunyc.work`（首页 3 处、教程与博客页若干处）；换域名时全局替换即可。

## 已按源码核实的行为描述（改文案时别写反）

- 额度用完：`429` + code `QUOTA_EXHAUSTED` / `USAGE_LIMIT_EXCEEDED`（Codex 路径为 `insufficient_quota`），重试无用。
- 限速：`429` + code `USER_RPM_EXCEEDED` / `GROUP_RPM_EXCEEDED`，退避重试。
- 余额不足：`403` + code `INSUFFICIENT_BALANCE`。
- 池里无可用账号：`503`。
- key 在 `/keys` 列表里随时可整条复制回来（不是只显示一次）。
- 建 key 时分组必填，`/v1/models` 只返回本 key 所属分组的模型。

## 注意事项

- 首页 HTML 里的 JavaScript 不会执行，所以没有复制按钮：base URL 与命令行用 `user-select:all`，点一下整行选中；`$` 提示符不会被选进去。
- 首屏终端里的 curl 输出是示意，不是真实响应；动效全部走 transform/opacity，系统开启「减弱动态效果」时全部静止。
- Safari 不显示 SVG favicon（Chrome / Edge / Firefox 正常）。若 Safari 用户多，导出一张 256×256 PNG 代替上传。
- 模型广场导航栏的 logo 容器浅色模式是白色 tile、深色模式是 `#1e293b`，标志在两种底上都验过。
- 上线后核对：标签页 favicon、侧栏 36px、登录页 64px、模型广场导航栏、首页亮/暗与手机宽度、首页 base URL 与控制台一致、占位符已全部替换。
