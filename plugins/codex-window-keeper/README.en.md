# Codex Window Keeper

[中文](README.md) | English

A CPA plugin that tracks the usage windows actually returned for each Codex OAuth credential. After it has observed a gating window exhausted and all observed exhausted gates recover, it sends one short Responses request pinned to that exact credential.

> The plugin does not assume that a successful response activates a quota window. It compares usage observations before and after the message and records the outcome as anchored, fixed, or inconclusive. A fixed window is not retried as if it were message-activated.

## Behavior and limits

- Discovers primary windows, additional limits, and optional Code Review windows from each account's usage payload. Plan type is used only to compare the observed window shape; Free/Plus/Pro combinations are not hard-coded.
- Uses CPA host auth-list/auth-get callbacks for Codex OAuth identity, `auth_index`, account ID, and plan. Credential JSON and access tokens are not persisted.
- Sends through CPA's model callback with `forced_provider=codex` and the exact `AuthID`. Success requires a complete SSE stream with `response.completed` and output text.
- Another observed exhausted gating window blocks sending. Authentication/config errors pause the account; after fixing the cause, resume it in the management page.
- Polling state, attempts, windows, per-account overrides, and encrypted management key are kept in one `keeper.sqlite` database. Startup clears leases and probes accounts with unfinished attempts.
- The upstream API has no idempotency key. If CPA is terminated after the upstream accepted a message but before the local success commit, the plugin probes before resuming; the rare ambiguous external-commit window cannot be eliminated completely.
- The plugin does not reset quotas, toggle credentials, or claim fixed windows move when a message is sent.

## Setup

Requires CLIProxyAPI `v7.3.9+` with plugin ABI and host model-stream callbacks. The plugin starts disabled.

Example config (put `data_dir` on persistent storage):

~~~yaml
plugins:
  enabled: true
  configs:
    codex-window-keeper:
      data_dir: data/codex-window-keeper
      enabled: false
      model: gpt-5.4
      reasoning_effort: low
      prompt: Reply with exactly OK.
      include_additional: fill_gaps
      poll_interval_seconds: 20
      activation_skew_seconds: 3
~~~

Open the CPA management menu **额度窗口**:

1. If the page warns that its management key is missing, open **Adjust** and enter the CPA management URL and a key allowed to call the management API. The plugin uses CPA's `/v0/management/api-call` route to read Codex usage by `auth_index`.
2. Confirm the account rows, plan comparison, and observed windows, then enable the global switch. Windows not observed before first enable are not retroactively sent.
3. **Re-read usage** only probes. **Send according to rules** still obeys the global switch, account pause, and quota gates; it is not a quota bypass.
4. After repairing authentication/configuration, use **Resume account**.

The default management URL is `http://127.0.0.1:8317`. Change it if CPA uses another local port. The management key is encrypted with AES-GCM in SQLite; `keeper.key` is local key material alongside the database, not a second database. Persist and protect both files with permissions restricted to the CPA service user.

## Build and test

~~~bash
cd plugins/codex-window-keeper
make test
make build
~~~

`make build` creates a c-shared library for the current system and requires CGO. Cross-compilation also requires a matching C cross-compiler. Release tags use `codex-window-keeper-v<version>`.

## Management endpoints

- Resource: `GET /v0/resource/plugins/codex-window-keeper/app`
- Health: `GET /v0/management/codex-window-keeper/health`
- Settings: `GET/PUT /v0/management/codex-window-keeper/settings`
- CPA URL/key: `PUT /v0/management/codex-window-keeper/connection`
- Accounts, overrides, probes, activation, and resume: `/v0/management/codex-window-keeper/accounts...`
- Attempt log: `GET /v0/management/codex-window-keeper/attempts`

The plugin does not start a replacement server; CPA serves the UI and API in-process.
