# Auto Recording

RefleK's can save OBS replay-buffer clips after completed Kovaak's runs.

The automatic trigger is a completed `Stats.csv` import. Cancelled or restarted attempts are not recorded unless Kovaak's produces a completed stats file for them.

RefleK's only links a recording to a run when OBS confirms that a new replay was saved for that recording attempt. If the app cannot confidently confirm a new replay, the save fails instead of linking an uncertain or older clip.

## Exact Run Clipping

RefleK's saves the OBS replay buffer first, then trims it to the verified Kovaak's run window when enough timing data is available.

For run-linked auto saves, RefleK's waits until the configured post-roll has elapsed before asking OBS to save the replay buffer. This is required because OBS cannot save footage that has not happened yet.

The clip window is:

* The completed scenario start.
* The completed scenario end.
* The configured pre-roll before the start.
* The configured post-roll after the end.

The default pre-roll is 2 seconds. The default post-roll is 3 seconds. Both are configurable in Settings.

RefleK's stores the timing source used for each recording. Current Kovaak's files normally provide `Challenge Start` as a time of day and the Stats filename as the completed run time. RefleK's maps those values onto the OBS replay file timeline using the OBS save request time and the probed replay duration.

If exact boundaries cannot be established, RefleK's keeps the full replay and marks the trim failure in metadata. It does not label that recording as an exact run clip.

If the replay buffer does not contain the full requested clip window, RefleK's trims the available portion and marks the recording as truncated. A truncated recording is not presented as a full exact clip.

RefleK's requires working `ffmpeg` and `ffprobe` binaries for trimming. The Windows installer can install app-owned FFmpeg under the RefleK's install folder, so users do not need to add FFmpeg to `PATH`.

FFmpeg lookup order is:

1. A custom FFmpeg path configured in Settings.
2. The bundled app-owned FFmpeg folder.
3. FFmpeg available through the system `PATH`.

RefleK's validates the selected FFmpeg toolchain against the current trimming pipeline, including MP4 probing, MP4 input, `libx264`, AAC audio, optional audio mapping and `+faststart`. If FFmpeg is missing, broken or fails trimming, the full replay remains linked and playable with a clear warning.

Configure the OBS replay buffer so it is longer than:

```text
longest expected scenario
+ completed Stats.csv detection delay
+ delayed OBS replay-save confirmation/processing time
+ pre-roll
+ post-roll
```

RefleK's cannot recover footage that has already left the OBS replay buffer.

## OBS Setup

The Windows installer has an optional OBS Studio setup component. It is off by default. When selected, RefleK's uses the official OBS installer instead of repackaging OBS. If OBS is already detected, the optional setup is skipped.

Portable or custom OBS installations may not be detected by the installer, but they are still usable. Configure the OBS host, port and password in RefleK's Settings and use Test OBS Connection to verify WebSocket access.

1. Open OBS.
2. Open OBS Settings and enable the replay buffer.
3. Configure a replay-buffer duration long enough for the scenarios you play.
4. Enable the OBS WebSocket server. OBS 28 and newer include WebSocket v5.
5. Note the host, port and password. RefleK's defaults to `127.0.0.1` and port `4455`.
6. In RefleK's, open Settings and enable Auto Recording.
7. Enter the OBS connection details and test the connection.

OBS must be running and reachable for recordings to work.

When Auto Connect and Auto-start Replay Buffer are enabled, RefleK's attempts to prepare the replay buffer when the app starts and when recording settings are updated.

Testing the OBS connection only checks authentication, OBS version and replay-buffer status. It does not start the replay buffer. Use the separate start action or start it manually in OBS.

The dependency status in Settings distinguishes:

* OBS installed or missing.
* OBS connection details saved in RefleK's.
* OBS WebSocket verified by the last successful connection test.
* Replay buffer active or inactive.
* FFmpeg installed, working, missing or broken.

Saved host and port settings alone do not mean WebSocket is configured. RefleK's only marks WebSocket as verified after a successful connection/authentication/status check.

## Recording Folder

The default recording folder is:

```text
$HOME/.refleks/recordings
```

You can change this folder in Settings.

Recording metadata is stored in:

```text
$HOME/.refleks/recordings.json
```

If the recording folder is on another drive, RefleK's copies the saved replay into the configured folder when a direct move is not possible.

For run-linked recordings, metadata distinguishes:

* The OBS source replay path.
* The copied full replay in the RefleK's recording folder.
* The trimmed clip when trimming succeeds.

After a successful trim, RefleK's deletes the OBS source replay. Settings decide whether the copied full replay is also kept or deleted. The in-app player uses the trimmed clip when available and falls back to the full replay when trimming failed or has not completed.

## Save Policies

Never-save scenario rules take precedence over every other automatic policy.

Always-save scenario rules override the selected global policy unless the scenario is also in the never-save list.

Available policies:

* **Every completed run:** Save every completed imported run.
* **New PBs:** Save only scores that are new local personal bests.
* **New and tied PBs:** Save new personal bests and scores tied with the current local best.
* **Local top-three scores:** Save runs that place within the local top three for that scenario.

Manual replay saving remains available as a separate action. A manual recording is not automatically linked to the latest completed run. It remains unlinked unless the user explicitly selects a run.

## Recording Status

OBS connection checks are temporary. RefleK's connects, reads the required information and then closes the connection.

A successful status therefore means that the **last OBS connection check succeeded**. It does not mean that RefleK's currently maintains an active connection to OBS.

The Recording Manager distinguishes between:

* Successfully saved recordings.
* Pending recording attempts.
* Failed recording attempts.
* Recordings whose video files are missing.

Only actual saved recordings are included in the saved-clip count.

## Storage And Cleanup

Settings include:

* A maximum recording storage limit.
* A minimum amount of free disk space.
* Optional automatic cleanup.

RefleK's checks disk space before completing a save. A save may be rejected if it would leave less free space than the configured minimum.

Cleanup preview shows which recordings would be removed before deletion.

By default, cleanup excludes:

* Protected recordings.
* Recordings that were personal bests when saved.
* Missing or unavailable files.
* The recording that was just saved.

Deleting a recording does not delete its linked Kovaak's run.

## Recording Manager

Use the Recording Manager to:

* View the result of the last OBS connection check.
* View replay-buffer status.
* View successfully saved, pending, failed and missing recordings.
* Filter and sort recordings.
* Detect missing files.
* Open videos.
* Reveal videos in Explorer.
* Protect or unprotect recordings.
* Delete recordings without deleting their linked runs.
* Save the current OBS replay manually.
* Preview and run storage cleanup.

The Recording Manager updates automatically when recording metadata or status changes.

Run details show recordings linked by the canonical run ID. Filename and path matching may be used only for older recording metadata created before run IDs were available.

## Failed And Duplicate Saves

RefleK's keeps one automatic recording attempt for each completed run.

Repeated import events for the same run do not create multiple recording entries. A retry updates the existing attempt for that run.

A recording fails without linking a video when:

* OBS does not confirm a newly saved replay.
* OBS returns an older or uncertain replay path.
* OBS disconnects during the save.
* Authentication fails.
* The replay buffer is inactive and automatic starting is disabled.
* The video file cannot be found or moved.
* Storage or free-space limits prevent the save.

Correct run-to-recording linking takes priority over saving a clip when the result is uncertain.

A recording can still be saved as a full replay with a trim warning when:

* Kovaak's timing fields are missing or ambiguous.
* `ffmpeg` or `ffprobe` is unavailable or fails capability validation.
* Trimming fails validation.

In those cases, the recording remains linked to the run but is not presented as an exact trimmed clip.

## Troubleshooting

* **Authentication failed:** Verify the OBS WebSocket password in Settings.
* **Connection failed or was lost:** Start OBS, enable the WebSocket server and verify the host and port.
* **Replay buffer inactive:** Start it manually or enable Auto-start Replay Buffer.
* **OBS did not confirm a new replay:** Confirm that replay-buffer saving works directly inside OBS.
* **Recording is marked truncated:** Increase the OBS replay-buffer duration so it includes the full scenario, detection delay, pre-roll and post-roll.
* **Recording saved as full replay; trim failed:** Install or repair `ffmpeg`/`ffprobe`, then check the recording error text.
* **Recording contains footage before the scenario:** Confirm trimming succeeded. If trimming failed, RefleK's plays the full replay fallback.
* **Saved replay file is missing:** Check the OBS output folder, configured RefleK's recording folder, permissions and antivirus rules.
* **Insufficient disk space:** Free disk space, lower the minimum free-space setting, or run cleanup.
* **Storage limit exceeded:** Increase the storage limit, enable automatic cleanup, or delete older unprotected recordings.
* **Recording marked missing:** Restore the file to its original path and refresh recording files.

## Privacy And Password Storage

Recording metadata and videos remain on the local machine unless the user moves or shares them.

The OBS password is stored locally and protected using the current Windows user account. It is not returned to the frontend after being saved.

Passwords protected for one Windows user or computer may not be usable after moving the settings file to another account or machine.

## Third-Party Dependency Notes

The Windows installer can include a pinned gyan.dev FFmpeg GPLv3 build for replay trimming. RefleK's itself is GPLv3 and the installer includes FFmpeg version information, build configuration, license notices and corresponding-source instructions under `THIRD_PARTY_NOTICES/ffmpeg`.

The FFmpeg staging script verifies that those packaging artifacts are present for the pinned package. It is a packaging checklist, not a legal determination.

OBS Studio is not redistributed inside RefleK's. If selected, the installer downloads the official OBS installer, verifies the pinned SHA-256 and Authenticode signer and launches it interactively. OBS cancellation or setup failure leaves RefleK's installed; complete OBS setup later from OBS Studio or RefleK's Settings guidance.
