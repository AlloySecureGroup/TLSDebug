# TLSDebug Desktop

Cross-platform Wails desktop interface for TLSDebug with a workflow inspired
by Fiddler Classic.

## Included

- Start and stop the bundled TLSDebug proxy
- Live session list with filtering
- Request, response, header, and raw inspectors
- One-click Root CA installation with the native Windows, macOS, or Linux
  authorization prompt
- Save captures as portable `.tlsdebug.json` files
- Import saved captures for offline review
- Configurable proxy port, monitor port, data directory, and proxy binary

## Prerequisites

- Go 1.23 or newer
- Node.js 20 or newer
- Platform requirements from the
  [Wails installation guide](https://wails.io/docs/gettingstarted/installation)
  (WebView2 on Windows, WebKitGTK on Linux, Xcode Command Line Tools on macOS)

The scripts download the pinned Wails CLI through `go run`; a global Wails
installation is not required.

## Run in development

macOS or Linux:

```bash
cd gui
chmod +x scripts/*.sh
./scripts/dev.sh
```

Windows PowerShell:

```powershell
cd gui
.\scripts\dev.ps1
```

The script builds `../tlsproxy.go`, points the GUI at the resulting binary,
installs frontend dependencies through Wails, and starts live development.
GUI-launched proxy and monitor ports bind to `127.0.0.1` so captured traffic
is not exposed to other devices on the network.

## Build

Build on each target operating system so Wails can use its native webview.

macOS or Linux:

```bash
cd gui
./scripts/build.sh
```

Windows:

```powershell
cd gui
.\scripts\build.ps1
```

Output is written to `gui/build/bin`. The scripts place the matching
`tlsproxy` executable beside the desktop application (inside the app bundle on
macOS).

## First capture

1. Click **Start**. This starts the proxy, using the repository CA pair when
   available or generating a per-user pair in the data directory.
2. Click **Install Root CA** and approve the operating-system prompt.
3. Configure the client being inspected to use `127.0.0.1:8080` as its HTTP
   and HTTPS proxy.
4. Generate traffic and select a session to inspect it.

The CA private key grants the ability to issue certificates trusted by that
machine. The GUI stores it in the configured data directory. Do not share it,
and use TLSDebug only with systems and traffic you are authorized to inspect.

If the TLSDebug repository root contains both `proxy-ca.crt` and
`proxy-ca.key`, the GUI validates that pair and copies it into the configured
data directory before starting the proxy. The repository pair takes precedence
over a previously generated GUI CA. Set `TLSDEBUG_ROOT` when launching from a
nonstandard working directory.
