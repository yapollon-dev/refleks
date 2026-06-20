# Auto Recording

RefleK's can save OBS replay-buffer clips after completed Kovaak's runs.

The automatic trigger is a completed `Stats.csv` import. Cancelled or restarted attempts are not recorded unless Kovaak's produces a completed stats file for them.

RefleK's only links a recording to a run when OBS confirms that a new replay was saved for that recording attempt. If the app cannot confidently confirm a new replay, the save fails instead of linking an uncertain or older clip.

## Current Recording Limitation

RefleK's currently saves the full OBS replay-buffer output. It does not yet trim the video to the exact start and end of the Kovaak's scenario.

This means:

* Short scenarios may include footage from before the run.
* The saved clip length depends on the replay-buffer duration configured in OBS.
* If a scenario is longer than the OBS replay buffer, the beginning of the run will not be available.
* RefleK's cannot recover footage that has already left the OBS replay buffer.

Configure the OBS replay buffer so it is longer than the longest scenario you expect to play, plus some additional time for the completed-run file to be detected and processed.

Exact scenario-length trimming will be added separately.

## OBS Setup

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

## Troubleshooting

* **Authentication failed:** Verify the OBS WebSocket password in Settings.
* **Connection failed or was lost:** Start OBS, enable the WebSocket server and verify the host and port.
* **Replay buffer inactive:** Start it manually or enable Auto-start Replay Buffer.
* **OBS did not confirm a new replay:** Confirm that replay-buffer saving works directly inside OBS.
* **Recording is missing the beginning:** Increase the OBS replay-buffer duration.
* **Recording contains footage before the scenario:** This is expected until exact scenario-length trimming is added.
* **Saved replay file is missing:** Check the OBS output folder, configured RefleK's recording folder, permissions and antivirus rules.
* **Insufficient disk space:** Free disk space, lower the minimum free-space setting, or run cleanup.
* **Storage limit exceeded:** Increase the storage limit, enable automatic cleanup, or delete older unprotected recordings.
* **Recording marked missing:** Restore the file to its original path and refresh recording files.

## Privacy And Password Storage

Recording metadata and videos remain on the local machine unless the user moves or shares them.

The OBS password is stored locally and protected using the current Windows user account. It is not returned to the frontend after being saved.

Passwords protected for one Windows user or computer may not be usable after moving the settings file to another account or machine.
