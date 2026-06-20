import { Button } from '@/shared/components'
import {
  deleteRecording,
  getRecordings,
  openRecording,
  refreshRecordingFiles,
  revealRecording,
  setRecordingProtected,
} from '@/shared/lib'
import type { RecordingRecord, StatKey } from '@/shared/types'
import { ArrowRightLeft, ExternalLink, FolderOpen, Lock, LockOpen, PinOff, RefreshCw, Trash2, Video } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  buildRunStats,
  formatDurationLabel,
  formatPercent,
  formatRunTimestamp,
  formatScore,
  formatSessionTitle,
  type HistoryRun,
} from '../../lib/historyModels'
import { CompareMetric, CompareStatRow, HeroStat, StatRow, StatsGroup } from './shared'

/** Read a raw numeric value from a run's stats map. */
function readNumericStat(run: HistoryRun, key: StatKey): number | null {
  const raw = run.item.stats?.[key]
  if (typeof raw === 'number' && Number.isFinite(raw)) return raw
  return null
}

/** Read a formatted stat value produced by buildRunStats. */
function readFormattedStat(run: HistoryRun, key: StatKey): string {
  const all = buildRunStats(run.item)
  return all.find(s => s.label === key)?.value ?? '–'
}

/** Percentage change from A to B. Returns null when A is zero. */
function computeDelta(a: number, b: number): number | null {
  if (!Number.isFinite(a) || !Number.isFinite(b) || a === 0) return null
  return ((b - a) / Math.abs(a)) * 100
}

const STAT_CATEGORIES: [string, StatKey[]][] = [
  ['Overview', ['Score', 'Kills', 'Hit Count', 'Accuracy']],
  ['Accuracy Details', ['Hit Count', 'Miss Count', 'Total Overshots', 'Damage Done', 'Damage Taken']],
  ['Timing', ['Fight Time', 'Time Remaining', 'Avg TTK', 'Real Avg TTK', 'Pause Count', 'Pause Duration', 'Challenge Start', 'Duration']],
  ['Controls', ['Sens Scale', 'Sens Increment', 'Horiz Sens', 'Vert Sens', 'DPI', 'cm/360']],
  ['Display', ['FOV', 'FOVScale', 'Resolution', 'Hide Gun', 'Crosshair', 'Crosshair Scale', 'Crosshair Color']],
  ['Technical', ['Input Lag', 'Max FPS (config)', 'Avg FPS', 'Resolution Scale']],
  ['Game Information', ['Scenario', 'Hash', 'Game Version', 'Score', 'Date Played', 'Distance Traveled', 'MBS Points', 'Challenge Start']],
  ['Additional Stats', ['Midairs', 'Midaired', 'Directs', 'Directed', 'Deaths', 'Avg Target Scale', 'Avg Time Dilation', 'Reloads']],
]

/** Map each stat label into its category. First match wins. */
function buildCategoryLookup(): Map<string, string> {
  const lookup = new Map<string, string>()
  for (const [cat, keys] of STAT_CATEGORIES) {
    for (const k of keys) {
      if (!lookup.has(k)) lookup.set(k, cat)
    }
  }
  return lookup
}
const CATEGORY_LOOKUP = buildCategoryLookup()

type CategorizedStats = { category: string; stats: Array<{ label: string; value: string }> }[]

function formatRecordingLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/\b\w/g, letter => letter.toUpperCase())
}

function recordingMatchesRun(recording: RecordingRecord, run: HistoryRun): boolean {
  const runId = run.item.runId?.trim()
  return !!runId && recording.runId === runId
}

function getCategorizedStats(run: HistoryRun): CategorizedStats {
  const all = buildRunStats(run.item)
  const buckets = new Map<string, Array<{ label: string; value: string }>>()

  for (const s of all) {
    const cat = CATEGORY_LOOKUP.get(s.label) ?? 'Other'
    let arr = buckets.get(cat)
    if (!arr) { arr = []; buckets.set(cat, arr) }
    arr.push(s)
  }

  const result: CategorizedStats = []
  const definedCats = new Set(STAT_CATEGORIES.map(([cat]) => cat))
  for (const [cat] of STAT_CATEGORIES) {
    const stats = buckets.get(cat)
    if (stats && stats.length > 0) result.push({ category: cat, stats })
  }
  const otherStats = buckets.get('Other')
  if (otherStats && otherStats.length > 0 && !definedCats.has('Other')) {
    result.push({ category: 'Other', stats: otherStats })
  }
  return result
}

export function StatsTab({ primaryRun, compareRun, onClearPrimaryRun, onClearComparison }: {
  primaryRun: HistoryRun
  compareRun: HistoryRun | null
  onClearPrimaryRun: () => void
  onClearComparison: () => void
}) {
  if (compareRun) {
    return (
      <CompareStatsView
        primaryRun={primaryRun}
        compareRun={compareRun}
        onClearPrimaryRun={onClearPrimaryRun}
        onClearComparison={onClearComparison}
      />
    )
  }

  const categories = getCategorizedStats(primaryRun)

  return (
    <>
      <div className="min-w-0">
        <div className="font-medium text-foreground">{primaryRun.scenarioName}</div>
        <div className="mt-0.5 text-xs text-surface-muted-foreground">
          {formatRunTimestamp(primaryRun.playedAt)} · {formatSessionTitle(primaryRun.session)}
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3">
        <HeroStat label="Score" value={formatScore(primaryRun.score)} />
        <HeroStat label="Accuracy" value={formatPercent(primaryRun.accuracy)} />
        <HeroStat label="Duration" value={formatDurationLabel(primaryRun.durationMs)} />
      </div>

      <RunRecordingsSection primaryRun={primaryRun} />

      {categories.map(({ category, stats }) => (
        <StatsGroup key={category} label={category}>
          {stats.map(s => <StatRow key={s.label} label={s.label} value={s.value} />)}
        </StatsGroup>
      ))}

      {primaryRun.item.fileName && (
        <div className="text-[11px] text-surface-muted-foreground truncate" title={primaryRun.item.filePath || primaryRun.item.fileName}>
          {primaryRun.item.fileName}
        </div>
      )}
    </>
  )
}

function RunRecordingsSection({ primaryRun }: { primaryRun: HistoryRun }) {
  const [records, setRecords] = useState<RecordingRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState('')
  const [error, setError] = useState('')

  const runKey = primaryRun.item.runId || primaryRun.item.filePath || primaryRun.item.fileName

  const linkedRecords = useMemo(
    () => records.filter(recording => recordingMatchesRun(recording, primaryRun)),
    [primaryRun, records],
  )

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      setRecords(await getRecordings())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to load linked recordings')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [runKey])

  const handleRefresh = async () => {
    setBusyId('refresh')
    setError('')
    try {
      setRecords(await refreshRecordingFiles())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to refresh linked recordings')
    } finally {
      setBusyId('')
    }
  }

  const handleOpen = async (recording: RecordingRecord) => {
    setBusyId(recording.id)
    setError('')
    try {
      await openRecording(recording.id)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to open recording')
      setRecords(await refreshRecordingFiles())
    } finally {
      setBusyId('')
    }
  }

  const handleReveal = async (recording: RecordingRecord) => {
    setBusyId(recording.id)
    setError('')
    try {
      await revealRecording(recording.id)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to reveal recording')
      setRecords(await refreshRecordingFiles())
    } finally {
      setBusyId('')
    }
  }

  const handleProtect = async (recording: RecordingRecord) => {
    setBusyId(recording.id)
    setError('')
    try {
      const updated = await setRecordingProtected(recording.id, !recording.protected)
      setRecords(prev => prev.map(item => item.id === updated.id ? updated : item))
    } catch (e) {
      setError((e as Error)?.message || 'Failed to update recording protection')
    } finally {
      setBusyId('')
    }
  }

  const handleDelete = async (recording: RecordingRecord) => {
    const label = recording.scenario || recording.runFileName || 'this recording'
    if (!window.confirm(`Delete ${label}? This removes only the recording metadata and video file, not the run.`)) return

    setBusyId(recording.id)
    setError('')
    try {
      await deleteRecording(recording.id)
      await load()
    } catch (e) {
      setError((e as Error)?.message || 'Failed to delete recording')
    } finally {
      setBusyId('')
    }
  }

  return (
    <StatsGroup label="Recordings">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs text-surface-muted-foreground">
          {loading ? 'Loading linked recordings...' : `${linkedRecords.length} linked recording${linkedRecords.length === 1 ? '' : 's'}`}
        </div>
        <Button variant="ghost" size="sm" onClick={() => void handleRefresh()} disabled={busyId === 'refresh'} title="Refresh missing-file state">
          {busyId === 'refresh' ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
        </Button>
      </div>

      {error && <div className="text-xs text-destructive">{error}</div>}

      {!loading && linkedRecords.length === 0 && (
        <div className="flex items-center gap-2 rounded-lg bg-surface px-3 py-2 text-xs text-surface-muted-foreground">
          <Video className="h-3.5 w-3.5" />
          No recording metadata is linked to this run.
        </div>
      )}

      {linkedRecords.map(recording => {
        const canUseVideo = !!recording.videoPath && recording.status !== 'missing'
        const rowBusy = busyId === recording.id
        return (
          <div key={recording.id} className="rounded-lg bg-surface px-3 py-2">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-foreground">{formatRecordingLabel(recording.keepReason)}</span>
                  <span className="rounded bg-surface-subtle px-1.5 py-0.5 text-[11px] text-surface-muted-foreground">
                    {formatRecordingLabel(recording.status)}
                  </span>
                  {recording.protected && (
                    <span className="inline-flex items-center rounded bg-primary/10 px-1.5 py-0.5 text-[11px] text-primary">
                      <Lock className="mr-1 h-3 w-3" />Protected
                    </span>
                  )}
                </div>
                <div className="mt-1 truncate font-mono text-[11px] text-surface-muted-foreground" title={recording.videoPath || recording.obsSourcePath || ''}>
                  {recording.videoPath || recording.obsSourcePath || 'No video path'}
                </div>
                {recording.lastError && <div className="mt-1 text-xs text-destructive">{recording.lastError}</div>}
              </div>
              <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                <Button size="sm" variant="outline" disabled={!canUseVideo || rowBusy} onClick={() => void handleOpen(recording)} title="Open video">
                  <ExternalLink className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="outline" disabled={!canUseVideo || rowBusy} onClick={() => void handleReveal(recording)} title="Reveal in Explorer">
                  <FolderOpen className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="outline" disabled={rowBusy} onClick={() => void handleProtect(recording)} title={recording.protected ? 'Unprotect recording' : 'Protect recording'}>
                  {recording.protected ? <LockOpen className="h-3.5 w-3.5" /> : <Lock className="h-3.5 w-3.5" />}
                </Button>
                <Button size="sm" variant="outline" disabled={rowBusy} onClick={() => void handleDelete(recording)} title="Delete recording">
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </div>
        )
      })}
    </StatsGroup>
  )
}

function CompareStatsView({ primaryRun, compareRun, onClearPrimaryRun, onClearComparison }: {
  primaryRun: HistoryRun
  compareRun: HistoryRun
  onClearPrimaryRun: () => void
  onClearComparison: () => void
}) {
  const primaryCats = getCategorizedStats(primaryRun)
  const compareCats = getCategorizedStats(compareRun)

  const primaryMaps = new Map<string, Map<string, string>>()
  for (const { category, stats } of primaryCats) {
    primaryMaps.set(category, new Map(stats.map(s => [s.label, s.value])))
  }
  const compareMaps = new Map<string, Map<string, string>>()
  for (const { category, stats } of compareCats) {
    compareMaps.set(category, new Map(stats.map(s => [s.label, s.value])))
  }

  const allCategories = [...new Set([...primaryCats.map(c => c.category), ...compareCats.map(c => c.category)])]
  const orderedCategories = STAT_CATEGORIES
    .map(([cat]) => cat)
    .filter(cat => allCategories.includes(cat))

  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex items-start justify-between gap-2 rounded-xl bg-surface-subtle px-3 py-2.5">
          <div className="min-w-0">
            <div className="text-xs text-surface-muted-foreground">Pinned</div>
            <div className="mt-0.5 font-medium text-foreground truncate">{primaryRun.scenarioName}</div>
            <div className="text-[11px] text-surface-muted-foreground">{formatRunTimestamp(primaryRun.playedAt)}</div>
          </div>
          <Button variant="ghost" size="sm" className="shrink-0" onClick={onClearPrimaryRun}>
            <PinOff className="h-3.5 w-3.5" />
          </Button>
        </div>
        <div className="flex items-start justify-between gap-2 rounded-xl bg-surface-subtle px-3 py-2.5">
          <div className="min-w-0">
            <div className="text-xs text-surface-muted-foreground">Compare</div>
            <div className="mt-0.5 font-medium text-foreground truncate">{compareRun.scenarioName}</div>
            <div className="text-[11px] text-surface-muted-foreground">{formatRunTimestamp(compareRun.playedAt)}</div>
          </div>
          <Button variant="ghost" size="sm" className="shrink-0" onClick={onClearComparison}>
            <ArrowRightLeft className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {(() => {
        const ttkA = readNumericStat(primaryRun, 'Avg TTK')
        const ttkB = readNumericStat(compareRun, 'Avg TTK')

        return (
          <div className="grid grid-cols-3 gap-3">
            <CompareMetric
              label="Score"
              a={formatScore(primaryRun.score)}
              b={formatScore(compareRun.score)}
              delta={computeDelta(primaryRun.score, compareRun.score)}
              lowerIsBetter={false}
            />
            <CompareMetric
              label="Accuracy"
              a={formatPercent(primaryRun.accuracy)}
              b={formatPercent(compareRun.accuracy)}
              delta={computeDelta(primaryRun.accuracy ?? 0, compareRun.accuracy ?? 0)}
              lowerIsBetter={false}
            />
            <CompareMetric
              label="Avg TTK"
              a={readFormattedStat(primaryRun, 'Avg TTK')}
              b={readFormattedStat(compareRun, 'Avg TTK')}
              delta={ttkA != null && ttkB != null ? computeDelta(ttkA, ttkB) : null}
              lowerIsBetter
            />
          </div>
        )
      })()}

      {orderedCategories.map(cat => {
        const pMap = primaryMaps.get(cat) ?? new Map<string, string>()
        const cMap = compareMaps.get(cat) ?? new Map<string, string>()
        const mergedKeys = [...new Set([...pMap.keys(), ...cMap.keys()])]
        if (mergedKeys.length === 0) return null
        return (
          <StatsGroup key={cat} label={cat}>
            {mergedKeys.map(key => (
              <CompareStatRow key={key} label={key} a={pMap.get(key) ?? '–'} b={cMap.get(key) ?? '–'} />
            ))}
          </StatsGroup>
        )
      })}
    </>
  )
}
