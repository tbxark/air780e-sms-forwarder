# Robustness, Correctness, and CI Fix Plan

## Goals

- Fix long-running service robustness first: reconnect serial sessions after disconnects and decouple Telegram HTTP latency from modem event handling.
- Fix correctness bugs in serial port ranking and PDU parsing while preserving existing PDU length validation and text/PDU `+CMT` support.
- Clean up low-risk performance, dead config/code, documentation drift, truncation helpers, and CI coverage.
- Keep changes minimal and testable; avoid adding durable storage unless explicitly requested.

## P0: Forwarder Runtime Robustness

1. Refactor `internal/forwarder/forwarder.go` so `Run` validates config and starts Telegram polling once, then enters a reconnect loop for the serial session.
2. Add a reconnectable AT executor in `forwarder` that implements `telegrambot.Executor` and can swap the current `*at.AT` instance on each reconnect.
3. Split the serial lifecycle into a helper that resolves the port, opens it, creates `rawLines`/`events`, constructs `modem.NewAT`, optionally runs `InitAir780E`, sets the executor, runs the select loop, then clears the executor and closes the port on exit.
4. On `atModem.Closed()`, log and enqueue a watchdog alert, close the old port, then retry instead of returning from `Run`.
5. For `cfg.Port == ""`, call `serialport.AutoDetect()` on each retry so reconnect can pick up a re-enumerated USB device; for an explicit `port`, retry the same path.
6. Use bounded retry backoff, for example initial 2s and max 30s, reset after a successful connected session; return `nil` only when `ctx` is cancelled.
7. Treat open/init failures as retryable: log, close any partially opened port, wait via the same backoff, and retry until context cancellation.

## P0: Telegram Send Queue

1. Introduce a small `telegramClient` interface in `forwarder` covering `SendSMS`, `SendRaw`, and `SendWatchdogAlert`, implemented by `*telegrambot.Service`.
2. Add a `telegramSender` worker with buffered queues so the modem select loop only enqueues work and never performs Telegram HTTP calls directly.
3. Use separate handling for important events and raw lines: SMS/watchdog alerts should use a larger buffered queue and context-aware enqueue; optional raw forwarding should use a smaller best-effort queue and may be dropped with a warning when congested.
4. Serialize actual Telegram API sends in the worker, log send failures, and stop cleanly when the parent context is cancelled.
5. Route `events`, `rawLines`, and serial-closed watchdog alerts through the sender.

## P0: Serial Port Ranking Correctness

1. Replace repeated marker scoring with a helper that adds the Air780E/EigenComm/Luat marker score at most once per source string.
2. Update `ScorePortName` so names like `air780e` do not receive both `air780` and `air780e` points.
3. Extract Linux USB metadata scoring into a pure helper, for example `scoreLinuxTTYInfo(info linuxTTYUSBInfo)`, and count manufacturer/product/interface marker evidence once instead of once per field.
4. Add tests for `air780` vs `air780e`, multiple markers in one name, and Linux metadata containing repeated markers across fields.

## P1: PDU Parser and GSM7 Performance

1. Keep the current TPDU length validation exactly before decoding TPDU fields.
2. Make `DecodePDU` MTI-aware from `firstOctet & 0x03`.
3. Preserve current SMS-DELIVER parsing for incoming `+CMT` messages: originator address, PID, DCS, 7-byte SCTS, UDL, UD.
4. Add SMS-SUBMIT parsing support or explicit internal helper coverage for TP-VPF offsets: parse MR, destination address, PID, DCS, then skip validity period according to TP-VPF bits (`00` none, `10` relative 1 byte, `01` enhanced 7 bytes, `11` absolute 7 bytes) before UDL/UD.
5. Return a clear unsupported-MTI error for TPDU types not handled, rather than producing shifted text.
6. Add tests with at least one PDU fixture containing a relative validity period and one unsupported/invalid path if applicable.
7. Move the GSM7 default alphabet table out of `gsm7Rune` into a package-level `[128]rune` so decoding does not allocate per character.

## P1: Telegram Formatting and Truncation Cleanup

1. Remove the unused `title` parameter from `formatCommandResult` and update its call site/tests.
2. Consolidate truncation constants and helpers in `internal/telegrambot`, keeping distinct semantic limits such as full Telegram message max, SMS body preview max, watchdog reason max, and CMGL item max.
3. Replace duplicate truncation logic in `truncateText`, `appendHTMLChunk`, and `escapeAndTruncate` with shared helpers and a single truncation suffix constant.
4. Optimize HTML escaping with a whole-string fast path: escape once and return immediately when within limit; fall back to safe rune/entity-aware truncation only when needed.
5. Keep existing HTML parse-mode behavior and add/update tests for escaping, suffix insertion, and max-length enforcement.

## P2: Dead Config, Watchdog Readability, Docs

1. Remove `ConfigurePort` from `config.Config`, `Default()`, and the deprecated warning in `forwarder.Run`.
2. Rely on Go JSON decoding ignoring unknown fields, so existing local `config.json` files containing `configure_port` continue loading without special compatibility code.
3. Update `README.md` to remove `configure_port` from examples and mention reconnect behavior plus queued Telegram forwarding.
4. Update `CLAUDE.md` architecture: remove stale `internal/notifier` references and describe the current Telegram service, reconnect loop, event queues, and executor serialization.
5. Keep `serialWatchdogCommand = ""` because `modem.Command("")` sends a bare `AT`; add a concise comment or rename to make that explicit rather than changing it to `"AT"`.

## P2: CI and Test Coverage

1. Add `.github/workflows/ci.yml` using `actions/checkout` and `actions/setup-go` with `go-version-file: go.mod`.
2. Run non-mutating checks only: `gofmt -l .` with failure on output, `go vet ./...`, and `go test ./...`.
3. Do not reuse the current `Makefile lint` target in CI because it mutates dependencies/source via `go get`, `go mod tidy`, and fixer tools.
4. Add focused forwarder tests around pure seams introduced by the refactor: reconnect loop retries after a closed-session sentinel, respects context cancellation, re-runs autodetect for empty ports, and resets/caps backoff.
5. If extracting watchdog state is still small, test failures/alerted/recovered transitions with an injected probe result rather than sleeping on real tickers.

## Verification

1. Run `gofmt` on changed Go files.
2. Run `go test ./...`.
3. Run `go vet ./...`.
4. Spot-check specific packages while developing: `go test ./internal/sms -run TestDecodePDU -v`, `go test ./internal/serialport -v`, and `go test ./internal/forwarder -v` after adding forwarder tests.

## Acceptance Criteria

- A serial disconnect no longer terminates the process; the service keeps Telegram polling alive and retries serial connection with backoff.
- Slow/failing Telegram sends no longer block AT indication handling directly.
- Port scores are deterministic and do not inflate because of overlapping marker substrings or repeated Linux metadata fields.
- PDU decoding either parses TP-VPF-aware SMS-SUBMIT layouts correctly or rejects unsupported layouts explicitly; existing SMS-DELIVER fixtures still pass.
- `configure_port` is removed from code/docs without breaking existing config files that still include the unknown JSON key.
- CI covers formatting, vet, and tests without mutating the working tree.
