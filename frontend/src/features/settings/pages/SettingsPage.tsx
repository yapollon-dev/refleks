import {
  WelcomeModalSession,
  buildManualWelcomePresentation,
  buildWelcomeSeenSettingsUpdate,
  buildWelcomeSettingsUpdate,
  type WelcomePresentation,
} from '@/features/welcome'
import { Button, Checkbox, Input, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/components'
import { setAvailableUpdate, useAvailableUpdate, usePersistedState, useStore } from '@/shared/hooks'
import {
  EXTERNAL_LINKS,
  FONTS,
  MISSING_VALUE,
  STORAGE_KEYS,
  THEMES,
  checkForUpdates,
  downloadAndInstallUpdate,
  getSettings,
  getVersion,
  openURL,
  quitApp,
  selectRecordingDirectory,
  setAutostart,
  setFont,
  setTheme,
  testRecordingConnection,
  updateSettings,
  type Font,
  type Theme,
} from '@/shared/lib'
import type { RecordingRuntimeStatus, RecordingSettings, Settings, UpdateInfo } from '@/shared/types'
import { ChevronDown, ChevronUp, Download, RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { ClearCacheModal } from '../components/ClearCacheModal'
import { ResetSettingsModal } from '../components/ResetSettingsModal'
import { SettingsField } from '../components/SettingsField'
import { SettingsSection } from '../components/SettingsSection'

const themeOptions = THEMES.map(t => ({
  label: t
    .split('-')
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' '),
  value: t,
}))
const fontOptions = FONTS.map(f => ({ label: f.label, value: f.id }))
const sessionGapOptions = [5, 10, 15, 20, 30, 45, 60, 90, 120].map(m => ({
  label: `${m} minutes`,
  value: String(m),
}))
const recordingPolicyOptions = [
  { label: 'Every completed run', value: 'every_run' },
  { label: 'New PBs only', value: 'new_pb' },
  { label: 'PBs and ties', value: 'pb_and_ties' },
  { label: 'Local top three', value: 'top_three' },
]

function parseScenarioList(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
}

function formatScenarioList(value?: string[]): string {
  return Array.isArray(value) ? value.join(', ') : ''
}

export function SettingsPage() {
  const setSessionGap = useStore(s => s.setSessionGap)
  const setSessionNotes = useStore(s => s.setSessionNotes)
  const availableUpdate = useAvailableUpdate()

  const [settings, setSettings] = useState<Settings | null>(null)
  const [showAdvanced, setShowAdvanced] = usePersistedState(STORAGE_KEYS.settingsShowAdvanced, false)
  const saveQueueRef = useRef(Promise.resolve())
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  const [currentVersion, setCurrentVersion] = useState<string>('')
  const [update, setUpdate] = useState<UpdateInfo | null>(null)
  const [checking, setChecking] = useState<boolean>(false)
  const [checkError, setCheckError] = useState<string>('')
  const [downloading, setDownloading] = useState<boolean>(false)
  const [downloadError, setDownloadError] = useState<string>('')
  const [isResetOpen, setIsResetOpen] = useState(false)
  const [isClearCacheOpen, setIsClearCacheOpen] = useState(false)
  const [welcomePresentation, setWelcomePresentation] = useState<WelcomePresentation | null>(null)
  const [recordingStatus, setRecordingStatus] = useState<RecordingRuntimeStatus | null>(null)
  const [testingRecordingConnection, setTestingRecordingConnection] = useState(false)

  useEffect(() => {
    getSettings().then(setSettings).catch(() => { })
    getVersion()
      .then(v => setCurrentVersion(v))
      .catch(() => setCurrentVersion(''))
  }, [])

  useEffect(() => {
    if (!availableUpdate) return
    setUpdate(previous => previous ?? availableUpdate)
  }, [availableUpdate])

  const queueSettingsSave = (next: Settings) => {
    setIsSaving(true)
    const run = saveQueueRef.current.then(async () => {
      await updateSettings(next)
      setSessionGap(next.sessionGapMinutes)
      setSessionNotes(next.sessionNotes ?? {})
      setHasUnsavedChanges(false)
    })
      .catch((error: unknown) => {
        console.error('Save error:', error)
        alert('Failed to save settings')
        throw error
      })
      .finally(() => {
        setIsSaving(false)
      })

    saveQueueRef.current = run.catch(() => { })
    return run
  }

  const updateField = <K extends keyof Settings>(key: K, value: Settings[K], persist = false) => {
    setSettings(prev => {
      if (!prev) return null
      const next = { ...prev, [key]: value }
      if (persist) {
        void queueSettingsSave(next)
      } else {
        setHasUnsavedChanges(true)
      }
      return next
    })
  }

  const updateRecordingField = <K extends keyof RecordingSettings>(key: K, value: RecordingSettings[K], persist = false) => {
    setSettings(prev => {
      if (!prev) return null
      const next = {
        ...prev,
        recording: {
          ...prev.recording,
          [key]: value,
        },
      }
      if (persist) {
        void queueSettingsSave(next)
      } else {
        setHasUnsavedChanges(true)
      }
      return next
    })
  }

  const handleSelectRecordingDirectory = async () => {
    try {
      const dir = await selectRecordingDirectory()
      if (dir) {
        updateRecordingField('recordingDir', dir)
      }
    } catch (e) {
      console.error('select recording directory error:', e)
      alert('Failed to select recording folder')
    }
  }

  const handleTestRecordingConnection = async () => {
    if (!settings) return
    setTestingRecordingConnection(true)
    try {
      await queueSettingsSave(settings)
      const next = await testRecordingConnection()
      setRecordingStatus(next)
    } catch (e) {
      setRecordingStatus(prev => ({
        enabled: settings.recording.enabled,
        recordingDir: settings.recording.recordingDir,
        metadataPath: prev?.metadataPath || '',
        totalRecordings: prev?.totalRecordings || 0,
        totalSizeBytes: prev?.totalSizeBytes || 0,
        storageLimitBytes: prev?.storageLimitBytes || 0,
        minFreeSpaceBytes: prev?.minFreeSpaceBytes || 0,
        freeSpaceBytes: prev?.freeSpaceBytes || 0,
        connectionStatus: 'error',
        replayBufferStatus: 'unknown',
        lastError: (e as Error)?.message || 'Failed to test OBS connection',
      }))
    } finally {
      setTestingRecordingConnection(false)
    }
  }

  const handleAutostartChange = async (enabled: boolean) => {
    try {
      await setAutostart(enabled)
      updateField('autostartEnabled', enabled)
    } catch (e) {
      console.error('setAutostart error:', e)
      alert('Failed to update autostart: ' + (e as Error)?.message)
    }
  }

  const handleThemeChange = (value: string) => {
    const theme = value as Theme
    setTheme(theme)
    updateField('theme', theme, true)
  }

  const handleFontChange = (value: string) => {
    const font = value as Font
    setFont(font)
    updateField('font', font, true)
  }

  const handleCheckUpdate = async () => {
    setChecking(true)
    setCheckError('')
    try {
      const info = await checkForUpdates()
      setUpdate(info)
      setAvailableUpdate(info)
    } catch (e) {
      setCheckError((e as Error)?.message || 'Failed to check for updates')
    } finally {
      setChecking(false)
    }
  }

  const handleDownloadInstall = async () => {
    setDownloading(true)
    setDownloadError('')
    try {
      await downloadAndInstallUpdate(update?.latestVersion ?? '')
    } catch (e) {
      setDownloadError((e as Error)?.message || 'Failed to download update')
      setDownloading(false)
    }
    // On success the app quits — no need to reset state
  }

  const handleOpenWelcome = () => {
    if (!settings) return

    const nextPresentation = buildManualWelcomePresentation(settings, currentVersion)
    if (!nextPresentation) return

    setWelcomePresentation(nextPresentation)
  }

  const handleWelcomeConfirm = async ({ anonymousEnabled, mouseTrackingEnabled }: { anonymousEnabled: boolean, mouseTrackingEnabled: boolean | null }) => {
    if (!settings || !welcomePresentation) return

    const next = buildWelcomeSeenSettingsUpdate(
      buildWelcomeSettingsUpdate(settings, { anonymousEnabled, mouseTrackingEnabled }),
      welcomePresentation.currentVersion,
    )
    setSettings(next)
    try {
      await queueSettingsSave(next)
    } catch {
      // queueSettingsSave already surfaced the failure to the user.
    }
  }

  const handleAnonymousChange = (enabled: boolean) => {
    setSettings(prev => {
      if (!prev) return null
      const next = {
        ...prev,
        anonymousEnabled: enabled,
      }
      void queueSettingsSave(next)
        .catch(() => { })
      return next
    })
  }

  const handleEnterCommit = (input?: HTMLInputElement) => {
    if (settings) {
      void queueSettingsSave(settings)
      input?.blur()
    }
  }

  const handleInputKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return
    e.preventDefault()
    handleEnterCommit(e.currentTarget)
  }

  const handleReset = async () => {
    try {
      const current = await getSettings()
      setSettings(current)
      setHasUnsavedChanges(false)
      setTheme(current.theme)
      if (current.font) setFont(current.font)
      setSessionGap(current.sessionGapMinutes)
      setSessionNotes(current.sessionNotes ?? {})
    } catch (e) {
      console.error('Reset error:', e)
    }
  }

  if (!settings) {
    return (
      <div className="flex h-full flex-col overflow-hidden text-sm">
        <div className="p-5 text-surface-muted-foreground">Loading settings...</div>
      </div>
    )
  }

  const recording = settings.recording

  return (
    <div className="flex flex-1 flex-col overflow-hidden text-sm">
      <div className="sticky top-0 z-10 bg-canvas/95 px-5 py-4 backdrop-blur">
        <div className="space-y-0.5">
          <h1 className="text-lg font-semibold text-foreground">Settings</h1>
          <p className="text-xs text-surface-muted-foreground">General behavior, privacy, appearance, and advanced integration options.</p>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-5">
        <div className="space-y-4">
          <SettingsSection title="Updates" description="Check for the latest version, reopen the welcome screen, and review the current release.">
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-sm text-surface-muted-foreground">
                Current version: <span className="font-mono text-foreground">{currentVersion || MISSING_VALUE}</span>
              </span>
              <Button onClick={handleCheckUpdate} disabled={checking} variant="outline" size="sm">
                {checking ? <RefreshCw className="h-4 w-4 animate-spin" /> : 'Check for Updates'}
              </Button>
              <Button onClick={handleOpenWelcome} disabled={!currentVersion.trim()} variant="outline" size="sm">
                Read Welcome Again
              </Button>
              {checkError && <span className="text-sm text-destructive">{checkError}</span>}
              {update && !update.hasUpdate && (
                <span className="text-sm text-surface-muted-foreground">You're on the latest version!</span>
              )}
            </div>
            {update?.hasUpdate && (
              <div className="space-y-3 rounded-xl bg-surface p-4 shadow-sm">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-foreground">Version {update.latestVersion} available</span>
                </div>
                <p className="text-xs text-surface-muted-foreground">
                  You&apos;re on {update.currentVersion || currentVersion || MISSING_VALUE}. Click <strong>Install Update</strong> to download in the background — the app will close and the installer will launch automatically.
                </p>
                <div className="flex flex-wrap items-center gap-2">
                  <Button onClick={handleDownloadInstall} disabled={downloading} variant="default" size="sm">
                    {downloading
                      ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Downloading...</>
                      : <><Download className="mr-1.5 h-4 w-4" />Install Update</>}
                  </Button>
                  <Button onClick={() => openURL(EXTERNAL_LINKS.changelog)} variant="outline" size="sm">
                    View Changelog
                  </Button>
                  {downloadError && <span className="text-sm text-destructive">{downloadError}</span>}
                </div>
              </div>
            )}
          </SettingsSection>

          <div className="grid gap-4 xl:grid-cols-2">
            <div className="space-y-4">
              <SettingsSection title="General" description="Core folders and session behavior.">
                <SettingsField label="KovaaK's Install Folder" description="Path to the KovaaK's install folder used to locate FPSAimTrainer/stats and FPSAimTrainer/performances">
                  <Input
                    type="text"
                    value={settings.kovaaksInstallDir}
                    onChange={e => updateField('kovaaksInstallDir', e.target.value)}
                    onKeyDown={handleInputKeyDown}
                    className="w-full max-w-xl"
                  />
                </SettingsField>

                <SettingsField label="Start with KovaaK's" description="Automatically launch RefleK's when you start KovaaK's, RefleK's will also start with Windows" checkbox>
                  <Checkbox
                    checked={!!settings.autostartEnabled}
                    onCheckedChange={v => handleAutostartChange(v === true)}
                  />
                </SettingsField>

                <SettingsField label="Mouse Tracking" description="Record mouse movement during scenarios (Windows only)" checkbox>
                  <Checkbox
                    checked={!!settings.mouseTrackingEnabled}
                    onCheckedChange={v => updateField('mouseTrackingEnabled', v === true, true)}
                  />
                </SettingsField>

                <SettingsField label="Session Gap" description="Minutes of inactivity before starting a new session">
                  <Select value={String(settings.sessionGapMinutes)} onValueChange={v => updateField('sessionGapMinutes', parseInt(v, 10), true)}>
                    <SelectTrigger className="h-8 w-max min-w-[8rem] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {sessionGapOptions.map(option => (
                        <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsField>
              </SettingsSection>

              <SettingsSection title="Privacy" description="Control whether runs are uploaded and whether identifying environment data is scrubbed before sync.">
                <SettingsField label="Run Sync" description="Upload completed runs to the RefleK's Index." checkbox>
                  <Checkbox
                    checked={settings.runSyncEnabled !== false}
                    onCheckedChange={v => updateField('runSyncEnabled', v === true, true)}
                  />
                </SettingsField>

                <SettingsField label="Anonymous Mode" description="Remove Steam ID and Steam persona name from run environment data before sync uploads." checkbox>
                  <Checkbox
                    checked={settings.anonymousEnabled === true}
                    onCheckedChange={v => handleAnonymousChange(v === true)}
                  />
                </SettingsField>
              </SettingsSection>
            </div>

            <div className="space-y-4">
              <SettingsSection title="Recording" description="Local OBS replay-buffer settings. This can test OBS connection and replay-buffer status.">
                <SettingsField label="Auto Recording" description="Enable run-linked replay saving when later recording is available." checkbox>
                  <Checkbox
                    checked={!!recording.enabled}
                    onCheckedChange={v => updateRecordingField('enabled', v === true, true)}
                  />
                </SettingsField>

                <div className="grid gap-3 sm:grid-cols-[1fr_7rem]">
                  <SettingsField label="OBS Host">
                    <Input
                      type="text"
                      value={recording.obsHost}
                      onChange={e => updateRecordingField('obsHost', e.target.value)}
                      onKeyDown={handleInputKeyDown}
                      className="w-full font-mono"
                    />
                  </SettingsField>
                  <SettingsField label="OBS Port">
                    <Input
                      type="number"
                      value={recording.obsPort}
                      onChange={e => updateRecordingField('obsPort', parseInt(e.target.value, 10) || recording.obsPort)}
                      onKeyDown={handleInputKeyDown}
                      min={1}
                      max={65535}
                      className="w-full text-center"
                    />
                  </SettingsField>
                </div>

                <SettingsField label="OBS Password" description="Stored locally in settings and masked in the UI.">
                  <Input
                    type="password"
                    value={recording.obsPassword || ''}
                    onChange={e => updateRecordingField('obsPassword', e.target.value || undefined)}
                    onKeyDown={handleInputKeyDown}
                    className="w-full max-w-sm"
                    placeholder="Optional"
                  />
                </SettingsField>

                <div className="rounded-lg bg-surface-subtle px-3 py-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => void handleTestRecordingConnection()} disabled={testingRecordingConnection}>
                      {testingRecordingConnection ? <><RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />Testing...</> : 'Test OBS Connection'}
                    </Button>
                    <span className="text-xs text-surface-muted-foreground">
                      Connection: <span className="font-medium text-foreground">{recordingStatus?.connectionStatus || 'not checked'}</span>
                    </span>
                    <span className="text-xs text-surface-muted-foreground">
                      Replay buffer: <span className="font-medium text-foreground">{recordingStatus?.replayBufferStatus || 'not checked'}</span>
                    </span>
                  </div>
                  {(recordingStatus?.obsVersion || recordingStatus?.obsWebSocketVersion) && (
                    <p className="mt-2 text-xs text-surface-muted-foreground">
                      OBS {recordingStatus.obsVersion || 'unknown'} · WebSocket {recordingStatus.obsWebSocketVersion || 'unknown'}
                    </p>
                  )}
                  {recordingStatus?.lastError && (
                    <p className="mt-2 text-xs text-destructive">{recordingStatus.lastError}</p>
                  )}
                </div>

                <SettingsField label="Recording Folder" description="Default: $HOME/.refleks/recordings">
                  <div className="flex flex-wrap gap-2">
                    <Input
                      type="text"
                      value={recording.recordingDir}
                      onChange={e => updateRecordingField('recordingDir', e.target.value)}
                      onKeyDown={handleInputKeyDown}
                      className="min-w-[16rem] flex-1 font-mono"
                    />
                    <Button type="button" variant="outline" size="sm" onClick={() => void handleSelectRecordingDirectory()}>
                      Browse
                    </Button>
                  </div>
                </SettingsField>

                <SettingsField label="Default Save Policy" description="Policy evaluation.">
                  <Select value={recording.savePolicy} onValueChange={v => updateRecordingField('savePolicy', v as RecordingSettings['savePolicy'], true)}>
                    <SelectTrigger className="h-8 w-max min-w-[12rem] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {recordingPolicyOptions.map(option => (
                        <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsField>

                <SettingsField label="Always-save Scenarios" description="Comma-separated scenario names. These override the global policy unless also listed as never-save.">
                  <Input
                    type="text"
                    value={formatScenarioList(recording.alwaysSaveScenarios)}
                    onChange={e => updateRecordingField('alwaysSaveScenarios', parseScenarioList(e.target.value))}
                    onKeyDown={handleInputKeyDown}
                    placeholder="Example: Smoothbot, Air Voltaic"
                    className="w-full"
                  />
                </SettingsField>

                <SettingsField label="Never-save Scenarios" description="Comma-separated scenario names. These take precedence over all other auto-save policies.">
                  <Input
                    type="text"
                    value={formatScenarioList(recording.neverSaveScenarios)}
                    onChange={e => updateRecordingField('neverSaveScenarios', parseScenarioList(e.target.value))}
                    onKeyDown={handleInputKeyDown}
                    placeholder="Example: Warmup scenario"
                    className="w-full"
                  />
                </SettingsField>

                <div className="grid gap-3 sm:grid-cols-2">
                  <SettingsField label="Storage Limit (GB)" description="Cleanup keeps unprotected non-PB recordings under this limit.">
                    <Input
                      type="number"
                      value={recording.storageLimitGb}
                      onChange={e => updateRecordingField('storageLimitGb', parseInt(e.target.value, 10) || recording.storageLimitGb)}
                      onKeyDown={handleInputKeyDown}
                      min={1}
                      className="w-24 text-center"
                    />
                  </SettingsField>
                  <SettingsField label="Minimum Free Space (GB)" description="New saves are blocked if they would drop the recording drive below this free-space floor.">
                    <Input
                      type="number"
                      value={recording.minFreeSpaceGb}
                      onChange={e => updateRecordingField('minFreeSpaceGb', parseInt(e.target.value, 10) || recording.minFreeSpaceGb)}
                      onKeyDown={handleInputKeyDown}
                      min={1}
                      className="w-24 text-center"
                    />
                  </SettingsField>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <SettingsField label="Auto Connect" description="Connect to OBS automatically so the replay buffer can be kept ready." checkbox>
                    <Checkbox
                      checked={!!recording.autoConnect}
                      onCheckedChange={v => updateRecordingField('autoConnect', v === true, true)}
                    />
                  </SettingsField>
                  <SettingsField label="Auto-start Replay Buffer" description="Start OBS replay buffer automatically before recording runs." checkbox>
                    <Checkbox
                      checked={!!recording.autoStartReplayBuffer}
                      onCheckedChange={v => updateRecordingField('autoStartReplayBuffer', v === true, true)}
                    />
                  </SettingsField>
                </div>

                <SettingsField label="Auto Cleanup" description="After a successful save, delete oldest eligible recordings if storage limits require it." checkbox>
                  <Checkbox
                    checked={!!recording.autoCleanup}
                    onCheckedChange={v => updateRecordingField('autoCleanup', v === true, true)}
                  />
                </SettingsField>
              </SettingsSection>

              <SettingsSection title="Appearance" description="Visual preferences for the interface.">
                <SettingsField label="Theme" description="Color theme for the application">
                  <Select value={settings.theme} onValueChange={handleThemeChange}>
                    <SelectTrigger className="h-8 w-max min-w-[8rem] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {themeOptions.map(option => (
                        <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsField>

                <SettingsField label="Font" description="Font family for the interface">
                  <Select value={settings.font || FONTS[0].id} onValueChange={handleFontChange}>
                    <SelectTrigger className="h-8 w-max min-w-[8rem] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {fontOptions.map(option => (
                        <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsField>
              </SettingsSection>

              <SettingsSection title="Advanced" description="Integration and data retention options.">
                <button
                  onClick={() => setShowAdvanced(!showAdvanced)}
                  className="flex items-center gap-1.5 text-sm text-surface-muted-foreground transition-colors hover:text-foreground"
                >
                  {showAdvanced ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                  {showAdvanced ? 'Hide advanced settings' : 'Show advanced settings'}
                </button>

                {showAdvanced && (
                  <div className="space-y-4 pt-2">
                    <div className="space-y-3">
                      <div className="text-xs font-medium uppercase tracking-wide text-surface-muted-foreground">Steam</div>
                      <div className="space-y-4">
                        <SettingsField label="Steam Install Directory">
                          <Input
                            type="text"
                            value={settings.steamInstallDir}
                            onChange={e => updateField('steamInstallDir', e.target.value)}
                            onKeyDown={handleInputKeyDown}
                            className="w-full max-w-xl"
                          />
                        </SettingsField>

                        <SettingsField label="Steam ID Override" description="Leave empty to auto-detect">
                          <Input
                            type="text"
                            value={settings.steamIdOverride || ''}
                            onChange={e => updateField('steamIdOverride', e.target.value || undefined)}
                            onKeyDown={handleInputKeyDown}
                            placeholder="76561198000000000"
                            className="w-full max-w-xs font-mono"
                          />
                        </SettingsField>

                        <SettingsField label="Persona Name Override" description="Leave empty to auto-detect">
                          <Input
                            type="text"
                            value={settings.personaNameOverride || ''}
                            onChange={e => updateField('personaNameOverride', e.target.value || undefined)}
                            onKeyDown={handleInputKeyDown}
                            placeholder="Display name"
                            className="w-full max-w-xs"
                          />
                        </SettingsField>
                      </div>
                    </div>

                    <div className="space-y-3 pt-4">
                      <div className="text-xs font-medium uppercase tracking-wide text-surface-muted-foreground">Mouse Traces</div>
                      <div className="space-y-4">
                        <SettingsField label="Buffer Duration" description="Minutes of mouse data to keep in memory">
                          <Input
                            type="number"
                            value={settings.mouseBufferMinutes}
                            onChange={e => updateField('mouseBufferMinutes', parseInt(e.target.value, 10) || 5)}
                            onKeyDown={handleInputKeyDown}
                            min={1}
                            max={60}
                            className="w-20 text-center"
                          />
                        </SettingsField>

                        <SettingsField label="Recent Runs Window (Days)" description="Only runs from the last N days are loaded and shown">
                          <Input
                            type="number"
                            value={settings.recentRunsDays}
                            onChange={e => {
                              const next = parseInt(e.target.value, 10)
                              updateField('recentRunsDays', Number.isFinite(next) && next > 0 ? next : settings.recentRunsDays)
                            }}
                            onKeyDown={handleInputKeyDown}
                            min={1}
                            max={3650}
                            className="w-24 text-center"
                          />
                        </SettingsField>

                        <SettingsField label="Recent Runs Minimum Count" description="If the day window has too few runs, include older runs until this minimum is reached">
                          <Input
                            type="number"
                            value={settings.recentRunsMinCount}
                            onChange={e => {
                              const next = parseInt(e.target.value, 10)
                              updateField('recentRunsMinCount', Number.isFinite(next) && next > 0 ? next : settings.recentRunsMinCount)
                            }}
                            onKeyDown={handleInputKeyDown}
                            min={1}
                            max={50000}
                            className="w-24 text-center"
                          />
                        </SettingsField>
                      </div>
                    </div>
                  </div>
                )}
              </SettingsSection>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3 pt-1">
            <Button variant="ghost" size="sm" onClick={() => setIsResetOpen(true)}>
              Reset
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setIsClearCacheOpen(true)}>
              Clear Cache
            </Button>
            <span className="text-xs text-surface-muted-foreground">
              {isSaving ? 'Saving settings...' : hasUnsavedChanges ? 'Unsaved changes' : 'All changes saved'}
            </span>
            <div className="flex-1" />
            <Button variant="default" size="sm" onClick={() => handleEnterCommit()} disabled={isSaving || !settings}>
              {isSaving ? 'Saving...' : 'Save'}
            </Button>
            <Button variant="destructive" size="sm" onClick={() => quitApp()}>
              Quit App
            </Button>
          </div>
        </div>
      </div>

      <ResetSettingsModal isOpen={isResetOpen} onClose={() => setIsResetOpen(false)} onReset={handleReset} />
      <ClearCacheModal isOpen={isClearCacheOpen} onClose={() => setIsClearCacheOpen(false)} />
      {welcomePresentation && (
        <WelcomeModalSession
          presentation={welcomePresentation}
          onConfirm={handleWelcomeConfirm}
          onDismissed={() => setWelcomePresentation(null)}
        />
      )}
    </div>
  )
}
