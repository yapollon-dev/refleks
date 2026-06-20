import { Button, Loading } from '@/shared/components'
import {
  deleteRecording,
  getRecordings,
  refreshRecordingFiles,
  revealRecording,
  setRecordingProtected,
} from '@/shared/lib'
import type { RecordingRecord } from '@/shared/types'
import { ArrowLeft, FolderOpen, Lock, LockOpen, RefreshCw, Trash2, Video } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { RecordingVideoPlayer } from '../components/RecordingVideoPlayer'
import { formatBytes, formatLabel, recordingTitle } from '../lib/recordingFormat'

function recordingVideoURL(id: string): string {
  return `/recording-video/${encodeURIComponent(id)}`
}

function formatDate(value?: string): string {
  if (!value) return 'Unknown'
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? new Date(parsed).toLocaleString() : value
}

function clipStatusLabel(recording: RecordingRecord): string {
  if (recording.trimStatus === 'succeeded') return 'Exact trimmed clip'
  if (recording.trimStatus === 'truncated') return 'Truncated trimmed clip'
  if (recording.trimStatus === 'failed') return 'Full replay fallback'
  if (recording.trimStatus === 'pending') return 'Trim pending'
  return recording.activeVideoKind === 'trimmed' ? 'Trimmed clip' : 'Full replay'
}

export function RecordingDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const recordingId = id ? decodeURIComponent(id) : ''
  const [recording, setRecording] = useState<RecordingRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const records = await getRecordings()
      setRecording(records.find(item => item.id === recordingId) ?? null)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to load recording')
    } finally {
      setLoading(false)
    }
  }, [recordingId])

  useEffect(() => {
    void load()
  }, [load])

  const canUseVideo = !!recording?.videoPath && recording.status !== 'missing'
  const videoURL = useMemo(() => (canUseVideo && recording ? recordingVideoURL(recording.id) : ''), [canUseVideo, recording])

  const handleBack = () => {
    navigate('/recordings')
  }

  const handleReveal = async () => {
    if (!recording) return
    setBusy('reveal')
    setError('')
    try {
      await revealRecording(recording.id)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to reveal recording')
      const records = await refreshRecordingFiles()
      setRecording(records.find(item => item.id === recording.id) ?? null)
    } finally {
      setBusy('')
    }
  }

  const handleProtect = async () => {
    if (!recording) return
    setBusy('protect')
    setError('')
    try {
      setRecording(await setRecordingProtected(recording.id, !recording.protected))
    } catch (e) {
      setError((e as Error)?.message || 'Failed to update recording protection')
    } finally {
      setBusy('')
    }
  }

  const handleRefreshFiles = async () => {
    if (!recording) return
    setBusy('refresh')
    setError('')
    try {
      const records = await refreshRecordingFiles()
      setRecording(records.find(item => item.id === recording.id) ?? null)
    } catch (e) {
      setError((e as Error)?.message || 'Failed to refresh recording files')
    } finally {
      setBusy('')
    }
  }

  const handleDelete = async () => {
    if (!recording) return
    if (!window.confirm(`Delete ${recordingTitle(recording)}? This removes only the recording metadata and video file, not the run.`)) return

    setBusy('delete')
    setError('')
    try {
      await deleteRecording(recording.id)
      navigate('/recordings')
    } catch (e) {
      setError((e as Error)?.message || 'Failed to delete recording')
    } finally {
      setBusy('')
    }
  }

  const handleVideoError = () => {
    setError('Video failed to load. The file may be missing, still locked by another app, or in a format WebView cannot play.')
  }

  if (loading) {
    return (
      <div className="flex h-full min-h-0 items-center justify-center">
        <Loading />
      </div>
    )
  }

  if (!recording) {
    return (
      <div className="flex-1 overflow-auto text-sm">
        <div className="sticky top-0 z-20 bg-canvas px-6 py-4">
          <Button variant="ghost" size="sm" onClick={handleBack}>
            <ArrowLeft className="mr-1.5 h-4 w-4" />
            Back
          </Button>
        </div>
        <div className="flex min-h-[18rem] flex-col items-center justify-center px-6 text-center">
          <Video className="h-8 w-8 text-surface-muted-foreground" />
          <h1 className="mt-3 text-base font-semibold text-foreground">Recording not found</h1>
          <p className="mt-1 max-w-md text-xs text-surface-muted-foreground">
            This recording metadata may have been deleted or cleaned up.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-auto text-sm">
      <div className="sticky top-0 z-20 bg-canvas px-6 py-4">
        <div className="flex min-w-0 flex-wrap items-center gap-2.5">
          <Button variant="ghost" size="sm" onClick={handleBack}>
            <ArrowLeft className="mr-1.5 h-4 w-4" />
            Back
          </Button>
          <h1 className="min-w-0 flex-1 truncate text-lg font-semibold text-foreground">{recordingTitle(recording)}</h1>
          <Button variant="ghost" size="icon" onClick={() => void handleReveal()} disabled={!canUseVideo || busy === 'reveal'} title="Reveal in Explorer">
            <FolderOpen className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="icon" onClick={() => void handleProtect()} disabled={busy === 'protect'} title={recording.protected ? 'Unprotect recording' : 'Protect recording'}>
            {recording.protected ? <LockOpen className="h-4 w-4" /> : <Lock className="h-4 w-4" />}
          </Button>
          <Button variant="ghost" size="icon" onClick={() => void handleRefreshFiles()} disabled={busy === 'refresh'} title="Detect missing file">
            <RefreshCw className={`h-4 w-4 ${busy === 'refresh' ? 'animate-spin' : ''}`} />
          </Button>
          <Button variant="ghost" size="icon" onClick={() => void handleDelete()} disabled={busy === 'delete'} title="Delete recording">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <div className="space-y-4 p-6">
        {error && <p className="rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</p>}

        <div className="rounded-xl bg-surface shadow-sm">
          {videoURL ? (
            <RecordingVideoPlayer src={videoURL} title={recordingTitle(recording)} onError={handleVideoError} />
          ) : (
            <div className="flex min-h-[24rem] flex-col items-center justify-center bg-black px-5 text-center">
              <Video className="h-8 w-8 text-surface-muted-foreground" />
              <p className="mt-3 text-sm font-medium text-foreground">Video file unavailable</p>
              <p className="mt-1 max-w-lg text-xs text-surface-muted-foreground">
                Refresh missing files or reveal the folder to check whether the recording was moved.
              </p>
            </div>
          )}
        </div>

        <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <h2 className="text-sm font-medium text-foreground">Recording</h2>
            <dl className="mt-4 grid gap-3 text-xs sm:grid-cols-2">
              <div>
                <dt className="text-surface-muted-foreground">Scenario</dt>
                <dd className="mt-1 text-sm text-foreground">{recording.scenario || 'Unlinked recording'}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Score</dt>
                <dd className="mt-1 text-sm text-foreground">{recording.score > 0 ? recording.score.toLocaleString(undefined, { maximumFractionDigits: 2 }) : 'None'}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Status</dt>
                <dd className="mt-1 text-sm text-foreground">{formatLabel(recording.status)}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Clip</dt>
                <dd className="mt-1 text-sm text-foreground">{clipStatusLabel(recording)}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Reason</dt>
                <dd className="mt-1 text-sm text-foreground">{formatLabel(recording.keepReason)}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Size</dt>
                <dd className="mt-1 text-sm text-foreground">{formatBytes(recording.sizeBytes)}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Played</dt>
                <dd className="mt-1 text-sm text-foreground">{formatDate(recording.playedAt)}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">Protected</dt>
                <dd className="mt-1 text-sm text-foreground">{recording.protected ? 'Yes' : 'No'}</dd>
              </div>
              <div>
                <dt className="text-surface-muted-foreground">PB at save</dt>
                <dd className="mt-1 text-sm text-foreground">{recording.pbAtSave ? 'Yes' : 'No'}</dd>
              </div>
              {recording.timingSource && (
                <div className="sm:col-span-2">
                  <dt className="text-surface-muted-foreground">Timing source</dt>
                  <dd className="mt-1 break-all text-sm text-foreground">{recording.timingSource}</dd>
                </div>
              )}
              {(recording.trimTruncatedStart || recording.trimTruncatedEnd) && (
                <div className="sm:col-span-2 rounded-lg bg-warning/10 px-3 py-2 text-xs text-warning">
                  This recording is truncated. The OBS replay buffer did not contain the full requested {recording.trimTruncatedStart && recording.trimTruncatedEnd ? 'start or end' : recording.trimTruncatedStart ? 'start' : 'end'} of the clip window.
                </div>
              )}
            </dl>
          </div>

          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <h2 className="text-sm font-medium text-foreground">File</h2>
            <div className="mt-4 space-y-3 text-xs">
              <div>
                <div className="text-surface-muted-foreground">Active video path</div>
                <div className="mt-1 break-all font-mono text-foreground">{recording.videoPath || 'Not saved yet'}</div>
              </div>
              {recording.trimmedVideoPath && (
                <div>
                  <div className="text-surface-muted-foreground">Trimmed clip</div>
                  <div className="mt-1 break-all font-mono text-foreground">{recording.trimmedVideoPath}</div>
                  <div className="mt-0.5 text-surface-muted-foreground">{formatBytes(recording.trimmedSizeBytes || 0)}</div>
                </div>
              )}
              {recording.rawVideoPath && (
                <div>
                  <div className="text-surface-muted-foreground">Full replay</div>
                  <div className="mt-1 break-all font-mono text-foreground">{recording.rawVideoPath}</div>
                  <div className="mt-0.5 text-surface-muted-foreground">
                    {(recording.rawSizeBytes || 0) > 0 ? formatBytes(recording.rawSizeBytes || 0) : 'Deleted after successful trim'}
                  </div>
                </div>
              )}
              <div>
                <div className="text-surface-muted-foreground">Run file</div>
                <div className="mt-1 break-all font-mono text-foreground">{recording.runFilePath || recording.runFileName || 'Unlinked'}</div>
              </div>
              {(recording.scenarioStartAt || recording.requestedClipStartAt || recording.replayTimelineStartAt) && (
                <div>
                  <div className="text-surface-muted-foreground">Timing</div>
                  <div className="mt-1 space-y-1 text-surface-muted-foreground">
                    {recording.scenarioStartAt && <div>Scenario: {formatDate(recording.scenarioStartAt)} to {formatDate(recording.scenarioEndAt)}</div>}
                    {recording.requestedClipStartAt && <div>Requested: {formatDate(recording.requestedClipStartAt)} to {formatDate(recording.requestedClipEndAt)}</div>}
                    {recording.actualClipStartAt && <div>Actual: {formatDate(recording.actualClipStartAt)} to {formatDate(recording.actualClipEndAt)}</div>}
                    {recording.replayTimelineStartAt && <div>Replay: {formatDate(recording.replayTimelineStartAt)} to {formatDate(recording.replayTimelineEndAt)}</div>}
                  </div>
                </div>
              )}
              {recording.lastError && (
                <div>
                  <div className="text-surface-muted-foreground">Last error</div>
                  <div className="mt-1 text-destructive">{recording.lastError}</div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
