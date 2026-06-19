# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go CLI that reads SMS messages pushed from an Air780E (EigenComm) cellular modem over a USB serial port and forwards them through pluggable notification channels (currently Telegram). The binary is self-contained — no Python/Node runtime needed.

## Commands

```sh
go build -o air780e-sms-forwarder .   # build
go test ./...                          # run all tests
go test ./internal/sms/                # test a single package
go test ./internal/sms/ -run TestDecodePDU -v   # run a single test

go run . ports                         # list/rank serial port candidates
go run . forward                       # read config.json, listen + forward
```

There is no lint config in the repo; use `go vet ./...` and `gofmt`.

## Architecture

Data flows in one direction through a fan-out of channels:

```
serialport.Open → modem.AT (warthog618/modem/at) → +CMT indication handler
                                                         ↓
                              rawLines chan ──┐    events chan (sms.Event)
                                              ↓          ↓
                              forwarder.Run select loop dispatches to []notifier.Notifier
```

- **cmd/** — Cobra root with two subcommands (`forward`, `ports`). `forward` reads runtime configuration from `config.json` only.
- **internal/config/** — `Config` struct + `Default()` + JSON loading from `config.json`. Missing files use defaults; empty string and zero number values fall back to defaults.
- **internal/forwarder/** — `Run()` is the orchestrator: opens the port, builds notifiers, registers AT indication handlers, then runs a `select` loop over `rawLines`, `events`, `ctx.Done()`, and modem-closed.
- **internal/modem/** — Wraps `warthog618/modem/at`. `NewAT` registers `+CMT:`/`+CMTI:`/`+CDS:` unsolicited-result-code handlers; `+CMT:` parses into an `sms.Event`. `InitAir780E` sends the AT init sequence (`E0`, `+CPIN?`, `+CSQ`, `+CMGF=1`, `+CNMI=2,2,0,0,0`).
- **internal/sms/** — Self-contained SMS parser. `ParseCMTIndication` distinguishes text-mode (`+CMT: "<oa>",...`) from PDU-mode (`+CMT: [<alpha>],<length>`) headers. `DecodePDU` parses the TPDU and decodes GSM 7-bit / UCS2 / 8-bit user data. This decoder is intentionally local (not a library) because it's tied to the Air780E `+CMT` format.
- **internal/notifier/** — `Notifier` interface (`Name`, `SendSMS`, `SendRaw`). `Build(cfg)` returns enabled channels; Telegram is added only when both token and chat ID are set. Add new channels by implementing the interface and appending in `Build`.
- **internal/serialport/** — Port discovery and scoring. `Candidates()` ranks ports; Air780E/EigenComm names, USB interface metadata (Linux `/sys/class/tty`), and stable `/dev/serial/by-id/*` symlinks score higher. `if03`/lower data interfaces beat `log`/`ppp`.

## Key behaviors to preserve

- **PDU length validation**: `DecodePDU` rejects a PDU when the actual TPDU length (after skipping the SMSC address) doesn't match the `<length>` from the `+CMT:` header. This guards against shifted/partial serial reads producing a wrong SMS. Don't remove this check.
- **Non-blocking channel sends**: `emitRawLines`/`emitSMSEvent` use `select`/`default` and drop (with a stderr warning) rather than block when the channel buffer is full.
- The default Air780E SMS mode is PDU (`AT+CMGF=0`); init switches it to text mode (`CMGF=1`) but the modem can still push PDU-mode `+CMT`, so both paths must keep working.

## Reference

OpenLuat AT command docs for `CMGF`, `CNMI`, and PDU encoding are linked in README.md.
