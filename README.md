# muse

A real Wails v2 desktop coding-agent harness in Go. Pick **any folder** as the workspace, chat with a model, and let the agent inspect and edit code *in that folder* — streaming tokens, tool-call cards, diff previews, and approve/deny prompts included.

It follows a harness pattern (agent loop → streamed model turn → tool calls → tool results → repeat; a small validated tool registry; ask-before-mutating-actions), built as a native Wails desktop app instead of a plugin/CLI system.

## Two backends (Settings → Backend)

**1. Muse subscription — via the Muse Code CLI (default).** A subscription does not give third-party apps an API key or OAuth flow (verified against dev.meta.ai/docs/muse-code/subscriptions, 2026-11-04) — but it does sign in to Meta's official `muse` CLI, and that CLI is built to be driven headlessly. So this app does exactly that, under the hood:

- You sign in once, in the official CLI: run `muse` in a terminal and complete browser sign-in with your subscription. The credential stays inside that CLI; this app never reads, mints, or stores it (no reverse-engineered key mint — deliberately not implemented).
- For each message this app spawns `muse exec --json --workspace <your folder> --prompt-file <tmp> --disable-approval` (plus `--model`, `--max-model-steps`, and `--session-id` to resume the CLI session), parses the JSONL stream (`run.output.delta`, `run.terminal.*`), and renders it in the chat. The CLI process exit code and stderr surface as errors.
- In this mode the CLI is the agent: it owns its tools, its OS sandbox, and permissions. Headless runs cannot answer its approval prompts, so prompts are disabled (`--disable-approval`) while its sandbox stays on; `--yolo` (sandbox off) is never passed. This app's own tools/approval modal are not in this path — pick the workspace carefully.

**2. Model API key — OpenAI-compatible endpoint.** A **Meta Model API key** (pay-as-you-go) from the Model API dashboard — default preset `https://api.meta.ai/v1`, model `muse-spark-1.3` — or **any OpenAI-compatible endpoint** (OpenRouter, a local vLLM/Ollama-compatible server, etc.) via Base URL + model + key in Settings. In this mode this app's own agent loop, six confined tools, and approve/deny modal apply.

Key resolution order: key saved in Settings, then env `MUSE_API_KEY`, `META_API_KEY`, `MODEL_API_KEY`, `OPENAI_API_KEY`. The key is never logged. It is stored plaintext in `settings.json` under your OS config dir (e.g. `%AppData%\muse-harness\settings.json`) — readable by any local process; it is masked (last 4 only) in the UI, not encrypted.

Note: streamed tool-calling on Meta's Chat Completions endpoint has not been live-verified without a real key. If Meta rejects `tools` there, the provider surface (`internal/provider`) is the one place to switch transport.

## Setup

Prerequisites: Go, Node/npm, and the Wails v2 CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`), plus WebView2 on Windows.

```powershell
cd D:\Projects\muse
muse            # once, in a terminal: sign in with your Muse subscription (CLI backend)
# API backend instead: $env:META_API_KEY = "<your key>"   # or paste it in Settings
wails dev      # develop (hot reload, regenerates frontend bindings)
wails build    # produce build\bin\muse.exe
```

Then: **Choose workspace…** → pick the project folder you want worked on → New chat → ask. Nothing about generation is confined to this app's own folder.

## Tools (all confined to the chosen workspace)

| Tool | Mutating | Approval |
|---|---|---|
| `list_files` | no | auto |
| `read_file` | no | auto |
| `search` (RE2 regex, skips .git/node_modules/dist, binaries) | no | auto |
| `write_file` | yes | approve/deny, with diff preview |
| `edit_file` (old_string must match exactly once) | yes | approve/deny, with diff preview |
| `run_command` (`cmd /C`, cwd = workspace, 120s default / 600s max timeout, output capped) | yes | approve/deny |

Paths are resolved through a single scope guard: absolute paths and `../` escapes are rejected, the longest existing prefix is canonicalized and re-checked, and the UI shows workspace-relative paths. On Windows canonicalization uses `GetFinalPathNameByHandle`, so directory **junctions** (`mklink /J`, which needs no admin privilege) and other reparse points that point outside the workspace are rejected for read/list/write/edit just like symlink escapes — covered by a junction regression test (`TestScopeJunctionEscape`). `search` likewise resolve-checks every directory it would descend and every file before reading it. Turning on **Auto-approve** in Settings skips prompts for the session (default off).

Honest limit: this is path confinement plus a trust prompt, **not an OS sandbox**. An approved `run_command` can still read/write anything your OS account can. Check the exact command in the approval modal.

Known limits / external blockers (unchanged, not faked):

- **No live API smoke test yet:** streamed tool-calling against `https://api.meta.ai/v1/chat/completions` has not been verified with a real key (none available here). If Meta rejects `tools` there, `internal/provider` is the single switch point.
- **Subscription = CLI delegation, not a login in this app:** there is no in-app "Sign in with Meta" (no public third-party OAuth exists); the subscription is used only by driving the official signed-in `muse` CLI, as documented at the top. If that CLI is not installed/signed in, the CLI backend fails with its stderr; use `muse usage` in a terminal to check allowance (this machine currently reports `no_allowance`).
- **API key at rest:** stored plaintext in `settings.json` (masked to last 4 in the UI). DPAPI/credential-store wrapping is deferred.
- **No OS sandbox:** confinement is the path checks above plus approval prompts only; see the honest limit above.
- **GUI coverage:** the frontend is compiled (`tsc` + `vite`) as part of `wails build`; it has not been interactively exercised beyond that build.

## Architecture

- `internal/musecli` — the subscription backend: spawns `muse exec --json` with the prompt in a temp file (never as a shell argument), translates JSONL (`run.output.delta` → chat deltas, `run.terminal.*` → done/error, session stream id → stored for `--session-id` resume), wraps the Windows `muse.cmd` shim, and caps errors to the stderr tail. Parser covered by unit tests from real CLI 1.4.14 `--provider echo` output.
- `internal/provider` — `Provider` interface + one OpenAI-compatible implementation (API backend): SSE streaming from `POST {BaseURL}/chat/completions`, tool-call deltas reassembled by index, typed HTTP errors (status + body snippet, never the key). `GET /models` backs the Settings model dropdown.
- `internal/agent` — sequential multi-turn loop (max steps from Settings, default 25). Context uses a stable prefix (fixed system prompt + deterministically sorted tools) with append-only history for automatic prefix-cache hits; token pressure at 80% of budget triggers one-shot checkpoint summarization (same prefix replayed + instruction at the tail) that replaces the oldest turns with a single summary, keeping the newest 4 turns verbatim. Plain group truncation remains as a fallback when summarization fails. Cancellation via context (Stop button).
- `internal/tools` — six validated tools; validation failures come back as `error: …` tool results so the model can self-correct, never panics. Exit codes are reported as `[exit code: N]` result text.
- `internal/session`, `internal/config` — one JSON file per chat plus `settings.json` under the OS user config dir (`%AppData%\muse-harness\`), atomic temp+rename saves after every step.
- `app.go` — thin Wails adapter only: bound methods (`GetSettings`, `SaveSettings`, `PickWorkspace`, `NewSession`, `ListSessions`, `LoadSession`, `DeleteSession`, `SendMessage`, `CancelRun`, `ResolveApproval`, `FetchModels`), an approval broker (5-minute timeout = deny), and events `chat:delta`, `chat:tool`, `approval:request`, `chat:done`, `chat:error`.
- `frontend/` — vanilla-TypeScript single page (Wails vanilla-ts template): sidebar sessions, workspace picker, streaming chat, tool-call cards with red/green diffs, approval modal, settings panel, dark editor-like theme.

Deliberately dropped versus the reference harness design (not stubbed): plugin architecture, CLI/JSON-RPC/ACP modes, parallel tool execution (sequential here for diff/ordering correctness), patch/image tools, OS sandboxes, JSONL/zstd session event logs, subagents/teams/skills.

## Verify

```powershell
go vet ./...
go test ./...    # path confinement + symlink and Windows junction/reparse-point escape, unique-match edit, search/read, command exit, truncation grouping, SSE tool-call reassembly
wails build
```
