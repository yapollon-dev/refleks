import { Button, Modal } from '@/shared/components'
import type { RecordingRecord } from '@/shared/types'
import { Trash2 } from 'lucide-react'
import { formatBytes, recordingTitle } from '../lib/recordingFormat'

type DeleteRecordingModalProps = {
  recording: RecordingRecord | null
  isDeleting?: boolean
  onClose: () => void
  onConfirm: (recording: RecordingRecord) => void | Promise<void>
}

export function DeleteRecordingModal({ recording, isDeleting = false, onClose, onConfirm }: DeleteRecordingModalProps) {
  const title = recording ? recordingTitle(recording) : ''
  const sizeBytes = recording ? (recording.sizeBytes || recording.rawSizeBytes || recording.trimmedSizeBytes || 0) : 0

  return (
    <Modal isOpen={!!recording} onClose={onClose} title="Delete Recording" width="min(560px, calc(100vw - 32px))" height="auto" closeOnOutsideClick={!isDeleting} closeOnEscapeKey={!isDeleting}>
      {recording && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <div className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
              <Trash2 className="h-5 w-5" />
            </div>
            <div className="min-w-0 space-y-1">
              <p className="text-sm font-medium text-foreground">Delete this replay video?</p>
              <p className="text-sm text-surface-muted-foreground">
                This removes only the recording metadata and video file. The linked Kovaak&apos;s run stays in your history.
              </p>
            </div>
          </div>

          <div className="min-w-0 overflow-hidden rounded-lg bg-surface-subtle px-3 py-2 text-xs">
            <div className="break-words font-medium text-foreground" title={title}>{title}</div>
            <div className="mt-1 break-all font-mono leading-relaxed text-surface-muted-foreground" title={recording.videoPath || recording.rawVideoPath || recording.trimmedVideoPath || ''}>
              {recording.videoPath || recording.rawVideoPath || recording.trimmedVideoPath || 'No video file path'}
            </div>
            {sizeBytes > 0 && (
              <div className="mt-1 text-surface-muted-foreground">
                {formatBytes(sizeBytes)}
              </div>
            )}
          </div>

          <div className="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={onClose} disabled={isDeleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={() => void onConfirm(recording)} disabled={isDeleting}>
              {isDeleting ? 'Deleting...' : 'Delete Recording'}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  )
}
