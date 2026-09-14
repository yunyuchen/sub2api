/**
 * 运维仪表盘的首字（TTFT）阈值判定。
 *
 * 阈值来自「指标阈值配置」里的 ttft_p99_ms_max：为空或 ≤ 0 视为关闭首字预警，
 * 此时 TTFT 卡片按 'off' 档显示中性色、诊断面板也不再给出「首 Token 时间偏高」。
 * TTFT 卡片与诊断面板共用同一个判定（诊断 = 卡片进入 critical 档），保证两处口径一致。
 */
export type OpsThresholdLevel = 'off' | 'normal' | 'warning' | 'critical'

/** 达到阈值的 80% 即进入 warning 档。 */
const TTFT_WARNING_RATIO = 0.8

export function isTtftAlertEnabled(threshold: number | null | undefined): threshold is number {
  return threshold != null && Number.isFinite(threshold) && threshold > 0
}

export function ttftThresholdLevel(
  ttftMs: number | null | undefined,
  threshold: number | null | undefined
): OpsThresholdLevel {
  if (!isTtftAlertEnabled(threshold)) return 'off'
  if (ttftMs == null) return 'normal'
  if (ttftMs >= threshold) return 'critical'
  if (ttftMs >= threshold * TTFT_WARNING_RATIO) return 'warning'
  return 'normal'
}

/** 诊断面板：与卡片同口径，仅当预警开启且 P99 达到阈值（critical 档）时提示。 */
export function isTtftP99High(
  ttftP99Ms: number | null | undefined,
  threshold: number | null | undefined
): boolean {
  return ttftThresholdLevel(ttftP99Ms, threshold) === 'critical'
}
