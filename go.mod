module wede

go 1.25.6

require (
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gorilla/websocket v1.5.3
	github.com/reearth/ygo v1.29.0
	github.com/vul-os/vulos-relay v0.0.0-00010101000000-000000000000
)

// Sovereign public tunnel: wede embeds the Vulos Relay agent from the sibling
// vulos-relay checkout instead of shelling out to a third-party frp binary.
replace github.com/vul-os/vulos-relay => ../vulos-relay

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/time v0.10.0 // indirect
)
