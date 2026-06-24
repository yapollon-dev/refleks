import type { RecordingRecord } from '@/shared/types'

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

export function formatLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/\b\w/g, letter => letter.toUpperCase())
}

export function formatRecordingScore(score: number): string {
  if (!Number.isFinite(score)) return ''
  return score.toLocaleString(undefined, { maximumFractionDigits: 2 })
}

export function formatRecordingDate(value?: string): string {
  if (!value) return ''
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? new Date(parsed).toLocaleString() : value
}

export function recordingScenarioLabel(recording: RecordingRecord): string {
  return recording.scenario || recording.runFileName || 'Unlinked recording'
}

export function recordingTitle(recording: RecordingRecord): string {
  const base = recordingScenarioLabel(recording)
  if (recording.scenario && Number.isFinite(recording.score) && recording.score > 0) {
    return `${base} - ${formatRecordingScore(recording.score)}`
  }
  return base
}
