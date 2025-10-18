# Curl_CLI
Interactive terminal UI for crafting HTTP requests (like Postman, but in your terminal) using Bubble Tea.

## Features
- Step-by-step flow: URL → Method → Headers → Body → Confirm → Response
- Responsive layout that adapts to your terminal size
- Scrollable response viewport
- Cancel long-running requests

## Requirements
- Go 1.20+
- bash, curl
- Optional: `jq` or Python for pretty-printing JSON responses

## Build and Run
```sh
go build
./Curl_CLI
```

## Keys
- Enter: Continue/Confirm
- Esc: Back (or cancel while sending)
- q or Ctrl+C: Quit (also cancels while sending)
- ↑/↓/PgUp/PgDn: Scroll response

## Notes
- Make sure `request.sh` is executable: `chmod +x request.sh`
- The app switches to alt-screen; your normal screen returns on exit.
