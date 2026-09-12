# 排行榜页自托管字体

只由 `frontend/src/styles/leaderboard-tokens.css` 的 `@font-face` 引用，随 `/leaderboard`
的懒加载路由按需加载，不进全局样式入口。

自托管而不是 `@import` Google Fonts 的原因见 `openspec/changes/add-user-leaderboard/design.md`
的 D16：站点 CSP 的 `font-src` 是 `'self' data:`（另带 fonts.gstatic.com），自托管在任何部署
（含内网与国内网络）下都可达，也不给全站引入一个必须可达的外部依赖。

| 文件 | 来源 | 说明 |
| --- | --- | --- |
| `geist-sans-latin.woff2` | Fontsource `@fontsource-variable/geist`，latin 子集 | 可变字体，`wght` 100–900，用于标题、正文、标签 |
| `geist-mono-latin.woff2` | Fontsource `@fontsource-variable/geist-mono`，latin 子集 | 可变字体，`wght` 100–900，用于全部数字（`font-variant-numeric: tabular-nums`） |

两个文件都只含 latin 子集，`@font-face` 的 `unicode-range` 也只声明 latin，因此中文与 emoji
自然回落到系统字体；字体拿不到时兜底栈是 `system-ui, -apple-system, "PingFang SC",
"Noto Sans CJK SC", sans-serif` 与 `ui-monospace, SFMono-Regular, Menlo, monospace`，
页面仍然可读。

报表皮肤里没有一处用斜体（`<em>` / `<i>` 都是语义或色块用途，`font-style` 另行置成
`normal`），因此不下载 italic 子集，也不声明 `font-style: italic` 的 `@font-face`。

v1 的 Editorial 皮肤用的 `fraunces-latin-normal.woff2`、`fraunces-latin-italic.woff2` 与
`inter-latin.woff2`，以及 v2 极客皮肤用的 `jetbrains-mono-latin.woff2`、
`ibm-plex-sans-latin.woff2`，在 v3 换皮后已从仓库删除，目录里只剩 Geist 两个文件。

## 许可

- Geist / Geist Mono — SIL Open Font License 1.1，版权归 Vercel, Inc.，见 <https://github.com/vercel/geist-font>

OFL 允许随软件一并分发与自托管，保留上述版权与许可声明即可。
