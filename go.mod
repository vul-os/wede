module wede

go 1.25.6

toolchain go1.25.12

require (
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gorilla/websocket v1.5.3
	github.com/reearth/ygo v1.29.0
	github.com/vul-os/ephor v0.4.0
)

// Sovereign public tunnel: wede embeds the Ephor agent
// (github.com/vul-os/ephor/tunnel/agent) instead of shelling out to a
// third-party frp binary. Ephor is a plain versioned dependency resolved from
// the module proxy — no replace directive, no sibling checkout.

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/time v0.10.0 // indirect
)
