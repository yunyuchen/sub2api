import { describe, expect, it } from 'vitest'
import { isTtftAlertEnabled, isTtftP99High, ttftThresholdLevel } from '../opsThresholds'

describe('isTtftAlertEnabled', () => {
  it('treats null, undefined, zero, negative and non-finite thresholds as disabled', () => {
    expect(isTtftAlertEnabled(null)).toBe(false)
    expect(isTtftAlertEnabled(undefined)).toBe(false)
    expect(isTtftAlertEnabled(0)).toBe(false)
    expect(isTtftAlertEnabled(-1)).toBe(false)
    expect(isTtftAlertEnabled(Number.NaN)).toBe(false)
    expect(isTtftAlertEnabled(Number.POSITIVE_INFINITY)).toBe(false)
  })

  it('accepts any positive finite threshold', () => {
    expect(isTtftAlertEnabled(1)).toBe(true)
    expect(isTtftAlertEnabled(500)).toBe(true)
  })
})

describe('ttftThresholdLevel', () => {
  it('returns off for any value once the alert is disabled (threshold 0 or missing)', () => {
    expect(ttftThresholdLevel(16553, 0)).toBe('off')
    expect(ttftThresholdLevel(16553, null)).toBe('off')
    expect(ttftThresholdLevel(16553, undefined)).toBe('off')
    expect(ttftThresholdLevel(null, 0)).toBe('off')
  })

  it('returns normal when the alert is on but the metric is missing', () => {
    expect(ttftThresholdLevel(null, 500)).toBe('normal')
    expect(ttftThresholdLevel(undefined, 500)).toBe('normal')
  })

  it('bands against the configured threshold with an 80% warning zone', () => {
    expect(ttftThresholdLevel(399, 500)).toBe('normal')
    expect(ttftThresholdLevel(400, 500)).toBe('warning')
    expect(ttftThresholdLevel(499, 500)).toBe('warning')
    expect(ttftThresholdLevel(500, 500)).toBe('critical')
    expect(ttftThresholdLevel(16553, 500)).toBe('critical')
  })

  it('follows a raised threshold instead of the 500ms default', () => {
    expect(ttftThresholdLevel(16553, 30000)).toBe('normal')
    expect(ttftThresholdLevel(24000, 30000)).toBe('warning')
    expect(ttftThresholdLevel(30000, 30000)).toBe('critical')
  })
})

describe('isTtftP99High', () => {
  it('never flags when the alert is disabled', () => {
    expect(isTtftP99High(16553, 0)).toBe(false)
    expect(isTtftP99High(16553, null)).toBe(false)
    expect(isTtftP99High(16553, undefined)).toBe(false)
  })

  it('flips together with the card: flagged exactly when the card is critical', () => {
    expect(isTtftP99High(null, 500)).toBe(false)
    expect(isTtftP99High(499, 500)).toBe(false)
    expect(isTtftP99High(500, 500)).toBe(true)
    expect(isTtftP99High(501, 500)).toBe(true)
    expect(isTtftP99High(16553, 30000)).toBe(false)
  })
})
