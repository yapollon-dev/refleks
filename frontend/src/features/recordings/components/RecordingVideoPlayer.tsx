import { Button } from '@/shared/components'
import { WindowFullscreen, WindowUnfullscreen } from '@wails/runtime'
import { Maximize2, Minimize2, Pause, Play, RotateCcw, Volume2, VolumeX } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type CSSProperties, type ChangeEvent } from 'react'

type RecordingVideoPlayerProps = {
  src: string
  title: string
  onError?: () => void
}

function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0:00'
  const rounded = Math.floor(seconds)
  const hours = Math.floor(rounded / 3600)
  const minutes = Math.floor((rounded % 3600) / 60)
  const secs = rounded % 60
  if (hours > 0) {
    return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  }
  return `${minutes}:${secs.toString().padStart(2, '0')}`
}

export function RecordingVideoPlayer({ src, title, onError }: RecordingVideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const isFullscreenRef = useRef(false)
  const [isPlaying, setIsPlaying] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [duration, setDuration] = useState(0)
  const [currentTime, setCurrentTime] = useState(0)
  const [volume, setVolume] = useState(1)
  const [muted, setMuted] = useState(false)

  const progressPercent = useMemo(() => {
    if (!duration) return 0
    return Math.min(100, Math.max(0, (currentTime / duration) * 100))
  }, [currentTime, duration])

  const volumePercent = muted ? 0 : Math.round(volume * 100)

  useEffect(() => {
    isFullscreenRef.current = isFullscreen
  }, [isFullscreen])

  useEffect(() => {
    setIsPlaying(false)
    setDuration(0)
    setCurrentTime(0)
  }, [src])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && isFullscreenRef.current) {
        event.preventDefault()
        exitFullscreen()
      }
      const activeTag = document.activeElement?.tagName
      const activeControl = activeTag === 'INPUT' || activeTag === 'BUTTON' || activeTag === 'SELECT' || activeTag === 'TEXTAREA'
      if (event.key === ' ' && !activeControl) {
        event.preventDefault()
        void togglePlayback()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      if (isFullscreenRef.current) {
        WindowUnfullscreen()
      }
    }
  }, [])

  const togglePlayback = async () => {
    const video = videoRef.current
    if (!video) return
    if (video.paused) {
      await video.play()
    } else {
      video.pause()
    }
  }

  const replay = async () => {
    const video = videoRef.current
    if (!video) return
    video.currentTime = 0
    await video.play()
  }

  const seek = (event: ChangeEvent<HTMLInputElement>) => {
    const video = videoRef.current
    if (!video) return
    const nextTime = Number(event.target.value)
    video.currentTime = nextTime
    setCurrentTime(nextTime)
  }

  const changeVolume = (event: ChangeEvent<HTMLInputElement>) => {
    const nextVolume = Number(event.target.value)
    const video = videoRef.current
    if (!video) return
    video.volume = nextVolume
    video.muted = nextVolume <= 0
    setVolume(nextVolume)
    setMuted(video.muted)
  }

  const toggleMute = () => {
    const video = videoRef.current
    if (!video) return
    video.muted = !video.muted
    setMuted(video.muted)
  }

  const enterFullscreen = () => {
    WindowFullscreen()
    setIsFullscreen(true)
  }

  const exitFullscreen = () => {
    WindowUnfullscreen()
    setIsFullscreen(false)
  }

  const toggleFullscreen = () => {
    if (isFullscreen) {
      exitFullscreen()
    } else {
      enterFullscreen()
    }
  }

  return (
    <div className={isFullscreen ? 'fixed inset-0 z-[100] bg-black' : 'overflow-hidden rounded-xl bg-black shadow-sm'}>
      <div className="group relative flex h-full min-h-[18rem] items-center justify-center bg-black">
        <video
          ref={videoRef}
          key={src}
          src={src}
          preload="metadata"
          onClick={() => void togglePlayback()}
          onDoubleClick={toggleFullscreen}
          onLoadedMetadata={event => {
            setDuration(event.currentTarget.duration || 0)
            setVolume(event.currentTarget.volume)
            setMuted(event.currentTarget.muted)
          }}
          onTimeUpdate={event => setCurrentTime(event.currentTarget.currentTime)}
          onDurationChange={event => setDuration(event.currentTarget.duration || 0)}
          onPlay={() => setIsPlaying(true)}
          onPause={() => setIsPlaying(false)}
          onEnded={() => setIsPlaying(false)}
          onVolumeChange={event => {
            setVolume(event.currentTarget.volume)
            setMuted(event.currentTarget.muted)
          }}
          onError={onError}
          className={isFullscreen ? 'h-screen w-screen object-contain' : 'block h-auto max-h-[74vh] w-full object-contain'}
        />

        {!isPlaying && (
          <button
            type="button"
            className="absolute left-1/2 top-1/2 inline-flex h-16 w-16 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-white/15 bg-black/55 text-white shadow-2xl backdrop-blur transition hover:bg-black/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            onClick={() => void togglePlayback()}
            title="Play"
          >
            <Play className="ml-1 h-7 w-7" fill="currentColor" />
          </button>
        )}

        <div className="pointer-events-none absolute inset-x-0 top-0 bg-gradient-to-b from-black/80 via-black/35 to-transparent px-5 pb-14 pt-4">
          <div className="truncate text-sm font-semibold text-white drop-shadow">{title}</div>
        </div>

        <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/95 via-black/70 to-transparent px-4 pb-4 pt-16 text-white">
          <input
            type="range"
            min={0}
            max={duration || 0}
            step="0.05"
            value={duration ? Math.min(currentTime, duration) : 0}
            onChange={seek}
            className="recording-player-range w-full"
            style={{ '--range-progress': `${progressPercent}%` } as CSSProperties}
            aria-label="Seek recording"
          />

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Button variant="ghost" size="icon" className="h-9 w-9 text-white hover:bg-white/10 hover:text-white" onClick={() => void togglePlayback()} title={isPlaying ? 'Pause' : 'Play'}>
              {isPlaying ? <Pause className="h-4 w-4" fill="currentColor" /> : <Play className="ml-0.5 h-4 w-4" fill="currentColor" />}
            </Button>
            <Button variant="ghost" size="icon" className="h-9 w-9 text-white hover:bg-white/10 hover:text-white" onClick={() => void replay()} title="Restart">
              <RotateCcw className="h-4 w-4" />
            </Button>

            <div className="min-w-[6.5rem] font-mono text-xs text-white/80">
              {formatTime(currentTime)} / {formatTime(duration)}
            </div>

            <div className="ml-auto flex items-center gap-2">
              <Button variant="ghost" size="icon" className="h-9 w-9 text-white hover:bg-white/10 hover:text-white" onClick={toggleMute} title={muted ? 'Unmute' : 'Mute'}>
                {muted || volume <= 0 ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}
              </Button>
              <input
                type="range"
                min={0}
                max={1}
                step="0.01"
                value={muted ? 0 : volume}
                onChange={changeVolume}
                className="recording-player-range w-24"
                style={{ '--range-progress': `${volumePercent}%` } as CSSProperties}
                aria-label="Recording volume"
              />
              <Button variant="ghost" size="icon" className="h-9 w-9 text-white hover:bg-white/10 hover:text-white" onClick={toggleFullscreen} title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
                {isFullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
