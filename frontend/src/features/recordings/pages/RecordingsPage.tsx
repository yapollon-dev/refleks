import { Button } from '@/shared/components'
import { getRecordings, getRecordingStatus, testRecordingConnection } from '@/shared/lib'
import type { RecordingRecord, RecordingRuntimeStatus } from '@/shared/types'
import { RefreshCw, Video } from 'lucide-react'
import { useEffect, useState } from 'react'

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

export function RecordingsPage() {
  const [status, setStatus] = useState<RecordingRuntimeStatus | null>(null)
  const [recordings, setRecordings] = useState<RecordingRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState('')

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
    } catch (e) {
      setError((e as Error)?.message || 'Failed to load recordings')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

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

  return (
    <div className="flex flex-1 flex-col overflow-hidden text-sm">
      <div className="sticky top-0 z-10 bg-canvas/95 px-5 py-4 backdrop-blur">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-0.5">
            <h1 className="text-lg font-semibold text-foreground">Recordings</h1>
            <p className="text-xs text-surface-muted-foreground">OBS replay metadata, connection status, and storage controls.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => void handleTestConnection()} disabled={testing} variant="outline" size="sm">
              {testing ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Testing...</> : 'Test OBS'}
            </Button>
            <Button onClick={() => void load()} disabled={loading} variant="outline" size="sm">
              {loading ? <RefreshCw className="h-4 w-4 animate-spin" /> : 'Refresh'}
            </Button>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-5">
        <div className="grid gap-4 md:grid-cols-5">
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Recording</div>
            <div className="mt-2 text-base font-medium text-foreground">
              {status?.enabled ? 'Enabled' : 'Disabled'}
            </div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">OBS</div>
            <div className="mt-2 text-base font-medium text-foreground">{status?.connectionStatus || 'not checked'}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Replay buffer</div>
            <div className="mt-2 text-base font-medium text-foreground">{status?.replayBufferStatus || 'not checked'}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Saved clips</div>
            <div className="mt-2 text-base font-medium text-foreground">{status?.totalRecordings ?? recordings.length}</div>
          </div>
          <div className="rounded-xl bg-surface px-5 py-4 shadow-sm">
            <div className="text-xs uppercase tracking-wide text-surface-muted-foreground">Storage used</div>
            <div className="mt-2 text-base font-medium text-foreground">{formatBytes(status?.totalSizeBytes ?? 0)}</div>
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

        <div className="mt-4 rounded-xl bg-surface px-5 py-10 text-center shadow-sm">
          <Video className="mx-auto h-8 w-8 text-surface-muted-foreground" />
          <h2 className="mt-3 text-sm font-medium text-foreground">
            {recordings.length === 0 ? 'No recordings yet' : `${recordings.length} recording metadata rows`}
          </h2>
          <p className="mx-auto mt-1 max-w-xl text-xs text-surface-muted-foreground">
            This can verify OBS websocket connectivity and replay-buffer state.
          </p>
          {error && <p className="mt-3 text-xs text-destructive">{error}</p>}
        </div>
      </div>
    </div>
  )
}
