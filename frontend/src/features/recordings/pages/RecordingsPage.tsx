import {
  Button,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/shared/components'
import {
  deleteRecording,
  getRecentRuns,
  getRecordings,
  getRecordingStatus,
  openRecording,
  previewRecordingCleanup,
  refreshRecordingFiles,
  revealRecording,
  runRecordingCleanup,
  saveCurrentReplay,
  setRecordingProtected,
  startRecordingReplayBuffer,
  testRecordingConnection,
} from '@/shared/lib'
import type { RecordingCleanupPreview, RecordingRecord, RecordingRuntimeStatus, RecordingStatusValue, RunRecord } from '@/shared/types'
import { EventsOn } from '@wails/runtime'
import { ExternalLink, FolderOpen, HardDrive, Lock, LockOpen, RefreshCw, Save, Search, Trash2, Video } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

type StatusFilter = 'all' | RecordingStatusValue
type ProtectionFilter = 'all' | 'protected' | 'unprotected'
type SortMode = 'newest' | 'oldest' | 'scenario' | 'status' | 'size_desc'

const manualReplayUnlinkedValue = '__unlinked__'

function formatBytes(bytes: number): string {
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

function formatLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/\b\w/g, letter => letter.toUpperCase())
}

function runScenario(run: RunRecord): string {
  const scenario = run.stats?.['Scenario']
  if (typeof scenario === 'string' && scenario.trim()) return scenario.trim()
  if (run.fileName.includes(' - ')) return run.fileName.split(' - ')[0].trim()
  return run.fileName || 'Unknown run'
}

function runDateLabel(run: RunRecord): string {
  const raw = run.stats?.['Date Played']
  const parsed = raw ? Date.parse(String(raw)) : 0
  return Number.isFinite(parsed) && parsed > 0 ? new Date(parsed).toLocaleString() : 'No date'
}

function statusClass(status: RecordingStatusValue): string {
  if (status === 'saved') return 'bg-success/10 text-success'
  if (status === 'failed' || status === 'missing') return 'bg-destructive/10 text-destructive'
  if (status === 'skipped') return 'bg-surface-subtle text-surface-muted-foreground'
  return 'bg-warning/10 text-warning'
}

function sortTimestamp(value: string | undefined): number {
  if (!value) return 0
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function checkedAtLabel(value?: string): string {
  if (!value) return ''
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? new Date(parsed).toLocaleTimeString() : ''
}

export function RecordingsPage() {
  const [status, setStatus] = useState<RecordingRuntimeStatus | null>(null)
  const [recordings, setRecordings] = useState<RecordingRecord[]>([])
  const [manualRuns, setManualRuns] = useState<RunRecord[]>([])
  const [manualRunId, setManualRunId] = useState(manualReplayUnlinkedValue)
  const [cleanupPreview, setCleanupPreview] = useState<RecordingCleanupPreview | null>(null)
  const [loading, setLoading] = useState(true)
  const [testing, setTesting] = useState(false)
  const [startingReplayBuffer, setStartingReplayBuffer] = useState(false)
  const [saving, setSaving] = useState(false)
  const [refreshingFiles, setRefreshingFiles] = useState(false)
  const [previewingCleanup, setPreviewingCleanup] = useState(false)
  const [runningCleanup, setRunningCleanup] = useState(false)
  const [busyId, setBusyId] = useState('')
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [protectionFilter, setProtectionFilter] = useState<ProtectionFilter>('all')
  const [sortMode, setSortMode] = useState<SortMode>('newest')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const [nextStatus, nextRecordings] = await Promise.all([
        getRecordingStatus(),
        getRecordings(),
      ])
      setStatus(nextStatus)
      setRecordings(nextRecordings)
      setManualRuns(await getRecentRuns(20))
      setCleanupPreview(await previewRecordingCleanup())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to load recordings')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  useEffect(() => {
    const offRecordings = EventsOn('recordings:changed', () => {
      void load()
    })
    const offStatus = EventsOn('recording:status:changed', () => {
      void load()
    })
    return () => {
      offRecordings()
      offStatus()
    }
  }, [])

  const visibleRecordings = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const filtered = recordings.filter(recording => {
      if (statusFilter !== 'all' && recording.status !== statusFilter) return false
      if (protectionFilter === 'protected' && !recording.protected) return false
      if (protectionFilter === 'unprotected' && recording.protected) return false
      if (!normalizedQuery) return true
      const haystack = [
        recording.scenario,
        recording.runFileName,
        recording.runId,
        recording.videoPath,
        recording.keepReason,
        recording.linkSource,
        recording.lastError,
      ].join(' ').toLowerCase()
      return haystack.includes(normalizedQuery)
    })

    return [...filtered].sort((a, b) => {
      if (sortMode === 'oldest') return sortTimestamp(a.createdAt) - sortTimestamp(b.createdAt)
      if (sortMode === 'scenario') return (a.scenario || a.runFileName).localeCompare(b.scenario || b.runFileName)
      if (sortMode === 'status') return a.status.localeCompare(b.status)
      if (sortMode === 'size_desc') return b.sizeBytes - a.sizeBytes
      return sortTimestamp(b.createdAt) - sortTimestamp(a.createdAt)
    })
  }, [protectionFilter, query, recordings, sortMode, statusFilter])

  const handleTestConnection = async () => {
    setTesting(true)
    setError('')
    try {
      setStatus(await testRecordingConnection())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to test OBS connection')
    } finally {
      setTesting(false)
    }
  }

  const handleStartReplayBuffer = async () => {
    setStartingReplayBuffer(true)
    setError('')
    try {
      setStatus(await startRecordingReplayBuffer())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to start OBS replay buffer')
    } finally {
      setStartingReplayBuffer(false)
    }
  }

  const handleSaveCurrentReplay = async () => {
    setSaving(true)
    setError('')
    try {
      await saveCurrentReplay(manualRunId === manualReplayUnlinkedValue ? '' : manualRunId)
      await load()
    } catch (e) {
      setError((e as Error)?.message || 'Failed to save current replay')
      try {
        setRecordings(await getRecordings())
      } catch {
        // Keep the primary save error visible.
      }
    } finally {
      setSaving(false)
    }
  }

  const handleRefreshFiles = async () => {
    setRefreshingFiles(true)
    setError('')
    try {
      const nextRecordings = await refreshRecordingFiles()
      const nextStatus = await getRecordingStatus()
      setRecordings(nextRecordings)
      setStatus(nextStatus)
      setCleanupPreview(await previewRecordingCleanup())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to refresh recording files')
    } finally {
      setRefreshingFiles(false)
    }
  }

  const handleOpen = async (recording: RecordingRecord) => {
    setBusyId(recording.id)
    setError('')
    try {
      await openRecording(recording.id)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to open recording')
      setRecordings(await refreshRecordingFiles())
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
      setRecordings(await refreshRecordingFiles())
    } finally {
      setBusyId('')
    }
  }

  const handleProtect = async (recording: RecordingRecord) => {
    setBusyId(recording.id)
    setError('')
    try {
      const updated = await setRecordingProtected(recording.id, !recording.protected)
      setRecordings(prev => prev.map(item => item.id === updated.id ? updated : item))
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

  const handlePreviewCleanup = async () => {
    setPreviewingCleanup(true)
    setError('')
    try {
      setCleanupPreview(await previewRecordingCleanup())
    } catch (e) {
      setError((e as Error)?.message || 'Failed to preview cleanup')
    } finally {
      setPreviewingCleanup(false)
    }
  }

  const handleRunCleanup = async () => {
    const count = cleanupPreview?.plannedDeleteCount ?? 0
    if (count <= 0) return
    if (!window.confirm(`Delete ${count} cleanup candidate${count === 1 ? '' : 's'}? Linked runs will not be deleted.`)) return

    setRunningCleanup(true)
    setError('')
    try {
      const result = await runRecordingCleanup()
      setCleanupPreview(result)
      const [nextStatus, nextRecordings] = await Promise.all([
        getRecordingStatus(),
        getRecordings(),
      ])
      setStatus(nextStatus)
      setRecordings(nextRecordings)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to run cleanup')
    } finally {
      setRunningCleanup(false)
    }
  }

  const hasFilters = query.trim() !== '' || statusFilter !== 'all' || protectionFilter !== 'all'
  const storageLimit = status?.storageLimitBytes || cleanupPreview?.storageLimitBytes || 0
  const freeSpace = status?.freeSpaceBytes || cleanupPreview?.freeSpaceBytes || 0
  const minFreeSpace = status?.minFreeSpaceBytes || cleanupPreview?.minFreeSpaceBytes || 0
  const obsStatusLabel = status?.lastConnectionStatus || status?.connectionStatus || 'not checked'
  const replayStatusLabel = status?.lastReplayBufferStatus || status?.replayBufferStatus || 'not checked'
  const obsCheckedAt = checkedAtLabel(status?.lastConnectionCheckedAt)

  return (
    <div className="flex flex-1 flex-col overflow-hidden text-sm">
      <div className="sticky top-0 z-10 bg-canvas/95 px-5 py-4 backdrop-blur">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-0.5">
            <h1 className="text-lg font-semibold text-foreground">Recordings</h1>
            <p className="text-xs text-surface-muted-foreground">OBS replay metadata, connection status, and local file actions.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Select value={manualRunId} onValueChange={setManualRunId}>
              <SelectTrigger className="w-[18rem] max-w-full" title="Manual replay link target">
                <SelectValue placeholder="Manual save target" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={manualReplayUnlinkedValue}>Unlinked recording</SelectItem>
                {manualRuns.filter(run => !!run.runId).map(run => (
                  <SelectItem key={run.runId} value={run.runId as string}>
                    {runScenario(run)} · {runDateLabel(run)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button onClick={() => void handleSaveCurrentReplay()} disabled={saving} variant="default" size="sm">
              {saving ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Saving...</> : <><Save className="mr-1.5 h-4 w-4" />Save Current Replay</>}
            </Button>
            <Button onClick={() => void handleRefreshFiles()} disabled={refreshingFiles} variant="outline" size="sm">
              {refreshingFiles ? <RefreshCw className="mr-1.5 h-4 w-4 animate-spin" /> : null}
              Detect Missing
            </Button>
            <Button onClick={() => void handleTestConnection()} disabled={testing} variant="outline" size="sm">
              {testing ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Testing...</> : 'Test OBS'}
            </Button>
            <Button onClick={() => void handleStartReplayBuffer()} disabled={startingReplayBuffer} variant="outline" size="sm">
              {startingReplayBuffer ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Starting...</> : 'Start Buffer'}
            </Button>
            <Button onClick={() => void load()} disabled={loading} variant="outline" size="sm">
              {loading ? <RefreshCw className="h-4 w-4 animate-spin" /> : 'Refresh'}
            </Button>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-5">
        <div className="grid gap-4 md:grid-cols-6">
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Recording</div>
            <div className="mt-2 text-base font-medium text-foreground">
              {status?.enabled ? 'Enabled' : 'Disabled'}
            </div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Last OBS check</div>
            <div className="mt-2 text-base font-medium text-foreground">{obsStatusLabel}</div>
            {obsCheckedAt && <div className="mt-0.5 text-[11px] text-surface-muted-foreground">{obsCheckedAt}</div>}
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Replay buffer</div>
            <div className="mt-2 text-base font-medium text-foreground">{replayStatusLabel}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Saved clips</div>
            <div className="mt-2 text-base font-medium text-foreground">{status?.totalRecordings ?? recordings.length}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Storage used</div>
            <div className="mt-2 text-base font-medium text-foreground">{formatBytes(status?.totalSizeBytes ?? 0)}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Storage limit</div>
            <div className="mt-2 text-base font-medium text-foreground">{formatBytes(storageLimit)}</div>
          </div>
        </div>

        <div className="mt-4 rounded-xl bg-surface px-5 py-4 shadow-sm">
          <div className="space-y-1">
            <h2 className="text-sm font-medium text-foreground">Recording folder</h2>
            <p className="break-all font-mono text-xs text-surface-muted-foreground">
              {status?.recordingDir || 'Not configured'}
            </p>
          </div>
          {status?.metadataPath && (
            <div className="mt-4 space-y-1">
              <h2 className="text-sm font-medium text-foreground">Metadata file</h2>
              <p className="break-all font-mono text-xs text-surface-muted-foreground">{status.metadataPath}</p>
            </div>
          )}
          {(status?.obsVersion || status?.obsWebSocketVersion) && (
            <div className="mt-4 space-y-1">
              <h2 className="text-sm font-medium text-foreground">OBS versions</h2>
              <p className="text-xs text-surface-muted-foreground">
                OBS {status.obsVersion || 'unknown'} · WebSocket {status.obsWebSocketVersion || 'unknown'}
              </p>
            </div>
          )}
          {status?.lastError && (
            <p className="mt-4 text-xs text-destructive">{status.lastError}</p>
          )}
        </div>

        <div className="mt-4 rounded-xl bg-surface px-5 py-4 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <h2 className="flex items-center gap-2 text-sm font-medium text-foreground">
                <HardDrive className="h-4 w-4 text-surface-muted-foreground" />
                Storage cleanup
              </h2>
              <p className="text-xs text-surface-muted-foreground">
                Cleanup deletes unprotected non-PB recordings only. Run files stay untouched.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" size="sm" onClick={() => void handlePreviewCleanup()} disabled={previewingCleanup}>
                {previewingCleanup ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Previewing...</> : 'Preview Cleanup'}
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                onClick={() => void handleRunCleanup()}
                disabled={runningCleanup || (cleanupPreview?.plannedDeleteCount ?? 0) <= 0}
              >
                {runningCleanup ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Cleaning...</> : 'Run Cleanup'}
              </Button>
            </div>
          </div>

          <div className="mt-4 grid gap-3 md:grid-cols-4">
            <div className="rounded-lg bg-surface-subtle px-3 py-2">
              <div className="text-xs text-surface-muted-foreground">Free space</div>
              <div className="mt-1 text-sm font-medium text-foreground">{formatBytes(freeSpace)}</div>
              <div className="mt-0.5 text-[11px] text-surface-muted-foreground">Minimum {formatBytes(minFreeSpace)}</div>
            </div>
            <div className="rounded-lg bg-surface-subtle px-3 py-2">
              <div className="text-xs text-surface-muted-foreground">Required cleanup</div>
              <div className="mt-1 text-sm font-medium text-foreground">{formatBytes(cleanupPreview?.requiredBytes ?? 0)}</div>
            </div>
            <div className="rounded-lg bg-surface-subtle px-3 py-2">
              <div className="text-xs text-surface-muted-foreground">Planned delete</div>
              <div className="mt-1 text-sm font-medium text-foreground">
                {cleanupPreview?.plannedDeleteCount ?? 0} files · {formatBytes(cleanupPreview?.plannedDeleteBytes ?? 0)}
              </div>
            </div>
            <div className="rounded-lg bg-surface-subtle px-3 py-2">
              <div className="text-xs text-surface-muted-foreground">Excluded</div>
              <div className="mt-1 text-sm font-medium text-foreground">
                {(cleanupPreview?.excludedProtectedCount ?? 0) + (cleanupPreview?.excludedPbCount ?? 0)} protected/PB
              </div>
              <div className="mt-0.5 text-[11px] text-surface-muted-foreground">
                {cleanupPreview?.excludedUnavailableCount ?? 0} unavailable
              </div>
            </div>
          </div>

          {cleanupPreview && (
            <div className="mt-3 space-y-2">
              <p className={cleanupPreview.willMeetLimits ? 'text-xs text-surface-muted-foreground' : 'text-xs text-destructive'}>
                {cleanupPreview.reason}
                {cleanupPreview.deletedCount > 0 ? ` Deleted ${cleanupPreview.deletedCount} files (${formatBytes(cleanupPreview.deletedBytes)}).` : ''}
              </p>
              {(cleanupPreview.items?.length ?? 0) > 0 && (
                <div className="max-h-44 divide-y divide-surface-border overflow-y-auto rounded-lg border border-surface-border">
                  {cleanupPreview.items?.map(item => (
                    <div key={item.id} className="grid gap-2 px-3 py-2 text-xs md:grid-cols-[1fr_7rem]">
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">{item.scenario || 'Unknown recording'}</div>
                        <div className="truncate font-mono text-surface-muted-foreground" title={item.videoPath}>{item.videoPath}</div>
                      </div>
                      <div className="text-right text-surface-muted-foreground">{formatBytes(item.sizeBytes)}</div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <div className="mt-4 rounded-xl bg-surface px-5 py-4 shadow-sm">
          <div className="grid gap-3 lg:grid-cols-[1.4fr_0.6fr_0.6fr_0.7fr]">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-surface-muted-foreground" />
              <Input
                value={query}
                onChange={e => setQuery(e.target.value)}
                className="pl-8"
                placeholder="Search scenario, run, path, reason, or error"
              />
            </div>
            <Select value={statusFilter} onValueChange={value => setStatusFilter(value as StatusFilter)}>
              <SelectTrigger><SelectValue placeholder="Status" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="saved">Saved</SelectItem>
                <SelectItem value="failed">Failed</SelectItem>
                <SelectItem value="missing">Missing</SelectItem>
                <SelectItem value="pending">Pending</SelectItem>
                <SelectItem value="skipped">Skipped</SelectItem>
              </SelectContent>
            </Select>
            <Select value={protectionFilter} onValueChange={value => setProtectionFilter(value as ProtectionFilter)}>
              <SelectTrigger><SelectValue placeholder="Protection" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All files</SelectItem>
                <SelectItem value="protected">Protected</SelectItem>
                <SelectItem value="unprotected">Unprotected</SelectItem>
              </SelectContent>
            </Select>
            <Select value={sortMode} onValueChange={value => setSortMode(value as SortMode)}>
              <SelectTrigger><SelectValue placeholder="Sort" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="newest">Newest first</SelectItem>
                <SelectItem value="oldest">Oldest first</SelectItem>
                <SelectItem value="scenario">Scenario</SelectItem>
                <SelectItem value="status">Status</SelectItem>
                <SelectItem value="size_desc">Largest first</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {error && <p className="mt-3 text-xs text-destructive">{error}</p>}
        </div>

        {recordings.length === 0 ? (
          <div className="mt-4 rounded-xl bg-surface px-5 py-10 text-center shadow-sm">
            <Video className="mx-auto h-8 w-8 text-surface-muted-foreground" />
            <h2 className="mt-3 text-sm font-medium text-foreground">No recordings yet</h2>
            <p className="mx-auto mt-1 max-w-xl text-xs text-surface-muted-foreground">
              Use Save Current Replay to save the current OBS replay buffer. It stays unlinked unless you select a run first.
            </p>
          </div>
        ) : visibleRecordings.length === 0 ? (
          <div className="mt-4 rounded-xl bg-surface px-5 py-10 text-center shadow-sm">
            <h2 className="text-sm font-medium text-foreground">No matching recordings</h2>
            <p className="mt-1 text-xs text-surface-muted-foreground">
              {hasFilters ? 'Clear or adjust filters to show more recordings.' : 'No recordings are available.'}
            </p>
          </div>
        ) : (
          <div className="mt-4 overflow-hidden rounded-xl bg-surface shadow-sm">
            <div className="border-b border-surface-border px-5 py-4">
              <h2 className="text-sm font-medium text-foreground">Recording metadata</h2>
              <p className="mt-1 text-xs text-surface-muted-foreground">
                Showing {visibleRecordings.length} of {recordings.length} recordings.
              </p>
            </div>
            <div className="divide-y divide-surface-border">
              {visibleRecordings.map(recording => {
                const canUseVideo = !!recording.videoPath && recording.status !== 'missing'
                const rowBusy = busyId === recording.id
                return (
                  <div key={recording.id} className="grid gap-3 px-5 py-4 xl:grid-cols-[1.2fr_0.45fr_0.65fr_0.55fr_1.25fr_0.85fr]">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-foreground">{recording.scenario || recording.runFileName || 'Unlinked recording'}</div>
                      <div className="mt-1 truncate text-xs text-surface-muted-foreground">{recording.playedAt || recording.runId || formatLabel(recording.linkSource || 'unlinked')}</div>
                      {recording.protected && (
                        <div className="mt-2 inline-flex items-center rounded bg-primary/10 px-1.5 py-0.5 text-[11px] text-primary">
                          <Lock className="mr-1 h-3 w-3" />Protected
                        </div>
                      )}
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Status</div>
                      <div className={`mt-1 inline-flex rounded px-1.5 py-0.5 text-xs font-medium ${statusClass(recording.status)}`}>
                        {formatLabel(recording.status)}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Reason</div>
                      <div className="mt-1 text-sm text-foreground">{formatLabel(recording.keepReason)}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Size</div>
                      <div className="mt-1 text-sm text-foreground">{formatBytes(recording.sizeBytes)}</div>
                    </div>
                    <div className="min-w-0">
                      <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Video path</div>
                      <div className="mt-1 truncate font-mono text-xs text-surface-muted-foreground" title={recording.videoPath || recording.obsSourcePath || ''}>
                        {recording.videoPath || recording.obsSourcePath || 'Not saved yet'}
                      </div>
                      {recording.lastError && <div className="mt-1 text-xs text-destructive">{recording.lastError}</div>}
                    </div>
                    <div className="flex flex-wrap content-start justify-end gap-1.5">
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
                )
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
