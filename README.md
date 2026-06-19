# Air780E SMS Forwarder

Small Go program for reading SMS messages from an Air780E USB serial port.

It can:

- configure the serial port with `go.bug.st/serial`
- initialize SMS text-mode push with AT commands through `github.com/warthog618/modem/at`
- print AT command responses and raw SMS-related modem indications
- parse simple `+CMT` text and PDU SMS events
- forward parsed SMS messages to Telegram and expose Telegram inline-keyboard modem controls

## Quick test on this Mac

Create `config.json` in the current working directory:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

```sh
go run . forward
```

Then send an SMS to the SIM card. You should see raw modem lines like:

```text
+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"
Your code is 123456
```

The Air780E may also report SMS in PDU mode. This has been tested with output like:

```text
+CMT:,24
07915892200417F5240BA14180889969F100006260918052200005E8329BFD06
```

The program decodes simple GSM 7-bit and UCS2 SMS PDU payloads.

## Libraries

The low-level serial work is delegated to `go.bug.st/serial`, so the program no longer shells out to `stty` or opens `/dev/tty*` manually. AT command request/response handling and unsolicited result code dispatch are handled by `github.com/warthog618/modem/at`.

The SMS PDU decoder remains local because it is small and tied to the Air780E `+CMT` format documented by OpenLuat.

## Serial message format

The init sequence uses:

```text
AT+CMGF=1
AT+CNMI=2,2,0,0,0
```

That asks the module to push new SMS messages directly to the serial port in text mode. In that mode, OpenLuat documents the received-message format as:

```text
+CMT: "<oa>",[<alpha>],<scts>[,<tooa>,<fo>,<pid>,<dcs>,<sca>,<tosca>,<length>]
<data>
```

Air780E's default SMS mode is PDU mode (`AT+CMGF=0`). In PDU mode, the same push notification is:

```text
+CMT: [<alpha>],<length>
<pdu>
```

For PDU mode, `<length>` is the TPDU length only. It does not include the SMS center address at the start of the PDU. The parser checks that length before decoding, so a shifted or partial serial read should be rejected instead of producing a wrong SMS.

Reference docs:

- https://docs.openluat.com/air780e/at/app/Command_List/SMS/CMGF/
- https://docs.openluat.com/air780e/at/app/Command_List/SMS/CNMI/
- https://docs.openluat.com/air780e/at/app/Command_List/SMS/PDU%E7%9F%AD%E4%BF%A1%E7%BC%96%E7%A0%81%E6%A0%BC%E5%BC%8F%E4%BB%8B%E7%BB%8D/

To listen without sending the init AT commands:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "init_modem": false
}
```

If port `0000000000013` does not show `OK` or SMS output, try:

```sh
go run . forward
```

Change `port` in `config.json` before retrying.

## Port discovery

The program gets the general serial port list from `go.bug.st/serial`. On Linux, it also prefers stable `/dev/serial/by-id/*` entries and checks `/sys/class/tty` USB metadata. Air780E / EigenComm ports are ranked above generic serial ports, and lower data interfaces are preferred over `log` or `ppp` style interfaces.

List candidates:

```sh
go run . ports
```

Run with automatic discovery:

```sh
go run . forward
```

For long-running Linux deployment, prefer the stable symlink shown by `ports` when available:

```sh
go run . forward
```

Set `port` in `config.json` to the stable symlink path.

## Telegram Control

Telegram is the program's push and control surface. `telegram_token` and `telegram_chat` are required for `go run . forward`; `telegram_chat` must be an int64 chat ID and is also the only authorized chat allowed to use the bot controls.

Configuration is read only from `config.json` in the current working directory. Missing files use built-in defaults, and empty string or zero number values in JSON fall back to defaults.

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "baud": 115200,
  "configure_port": true,
  "init_modem": true,
  "telegram_raw": false,
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

```sh
go run . forward
```

Open the bot chat and send `/start` or `/menu` to show the inline keyboard. The bot uses long polling and deletes any existing webhook before polling.

The menu provides status queries, SMS history queries, device controls, help, and an OpenLuat AT documentation link. Button-triggered AT commands are serialized before they are sent to the modem.

Available controls include:

- status summary, signal quality, network registration, operator, SIM status, and module information
- unread SMS, all SMS, and SMS storage queries
- current function mode, re-enable SMS push, and reset with confirmation

To forward every raw line as well:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "telegram_raw": true,
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

## Build

```sh
go build -o air780e-sms-forwarder .
```

The resulting binary does not need Python or Node.js.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
