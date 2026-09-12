/**
 * 排行榜页的线条图标（16×16 viewBox，stroke 1.5，`fill: none`）。
 *
 * 路径照已确认画板的生成器（scratchpad/leaderboard-design-canvas/v3/gen_taste.py 的 `ICONS`）
 * 原样移植，只是把 SVG 片段拆成结构化的图元，免得组件里用 `v-html` 注入标记。
 * 名字就是 `LbIcon` 的 `name` 取值，MUST NOT 随手改名：它同时是 `data-icon` 的值。
 */

/** 一个图元。`kind` 决定用到哪几个字段，其余字段缺席（模板里未绑定的属性不会出现在 DOM 上）。 */
export interface LbIconShape {
  kind: 'path' | 'circle' | 'rect'
  /** kind = path */
  d?: string
  /** kind = circle */
  cx?: number
  cy?: number
  r?: number
  /** kind = rect */
  x?: number
  y?: number
  width?: number
  height?: number
  rx?: number
}

export const LEADERBOARD_ICONS = {
  'arrow-left': [{ kind: 'path', d: 'M12.5 8h-9M7 3.5 2.5 8 7 12.5' }],
  sun: [
    { kind: 'circle', cx: 8, cy: 8, r: 3.2 },
    {
      kind: 'path',
      d: 'M8 1v1.6M8 13.4V15M15 8h-1.6M2.6 8H1M12.9 3.1l-1.1 1.1M4.2 11.8l-1.1 1.1M12.9 12.9l-1.1-1.1M4.2 4.2 3.1 3.1',
    },
  ],
  moon: [{ kind: 'path', d: 'M13.4 9.6A5.8 5.8 0 0 1 6.4 2.6a5.9 5.9 0 1 0 7 7Z' }],
  refresh: [
    { kind: 'path', d: 'M13.6 6.6A5.6 5.6 0 0 0 3.3 5.4M2.4 9.4a5.6 5.6 0 0 0 10.3 1.2' },
    { kind: 'path', d: 'M13.9 2.9v3.7h-3.7M2.1 13.1V9.4h3.7' },
  ],
  'eye-off': [
    {
      kind: 'path',
      d: 'M6.2 3.3A6.2 6.2 0 0 1 8 3.1c3.4 0 5.7 2.6 6.6 4.1a1 1 0 0 1 0 .9 12 12 0 0 1-1.9 2.4M4 4.5A11 11 0 0 0 1.4 7.2a1 1 0 0 0 0 .9c.9 1.5 3.2 4.1 6.6 4.1a6.6 6.6 0 0 0 3.1-.8',
    },
    { kind: 'path', d: 'm2 2 12 12' },
    { kind: 'path', d: 'M6.6 6.7a2 2 0 0 0 2.7 2.8' },
  ],
  clock: [
    { kind: 'circle', cx: 8, cy: 8, r: 6.2 },
    { kind: 'path', d: 'M8 4.4V8l2.4 1.5' },
  ],
  rows: [{ kind: 'path', d: 'M2.5 4h11M2.5 8h11M2.5 12h11' }],
  layers: [
    { kind: 'path', d: 'M8 1.8 1.8 5 8 8.2 14.2 5 8 1.8Z' },
    { kind: 'path', d: 'm1.8 9.2 6.2 3.2 6.2-3.2' },
  ],
  pulse: [{ kind: 'path', d: 'M1.5 8h3l2-4.6L9.3 12l1.8-4h3.4' }],
  calendar: [
    { kind: 'rect', x: 2, y: 3, width: 12, height: 11, rx: 1.4 },
    { kind: 'path', d: 'M5.2 1.5V4M10.8 1.5V4M2 6.6h12' },
  ],
  bars: [{ kind: 'path', d: 'M2.5 13.5V9M6.5 13.5V4M10.5 13.5V6.5M14 13.5v-3' }],
  crown: [{ kind: 'path', d: 'M2 12.5h12M2.4 4.2 4.9 7 8 3l3.1 4 2.5-2.8-1 6.1H3.4Z' }],
} satisfies Record<string, LbIconShape[]>

export type LbIconName = keyof typeof LEADERBOARD_ICONS
