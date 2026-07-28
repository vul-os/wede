module wede

go 1.25.6

toolchain go1.25.12

require (
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gorilla/websocket v1.5.3
	github.com/reearth/ygo v1.29.0
	github.com/vul-os/ephor v0.0.0-00010101000000-000000000000
)

// Sovereign public tunnel: wede embeds the Ephor agent
// (github.com/vul-os/ephor/tunnel/agent) instead of shelling out to a
// third-party frp binary.
//
// STOPGAP — this replace exists only because Ephor has no consumable tag yet.
// Ephor's published tags v0.1.0, v0.2.0 and v0.3.0 were all cut while its
// go.mod still declared `module github.com/vul-os/vulos-relay`, so the proxy
// serves them under the OLD path and `go get github.com/vul-os/ephor@v0.3.0`
// does NOT resolve. No tag has been cut since the module was renamed.
//
// Nothing here needs a sibling checkout to BUILD: the agent's packages are
// committed under /vendor, and vendor mode never consults the replace target.
// The replace is what `go mod tidy` / `go mod vendor` resolve against, so those
// two commands (and only those) need ../ephor on disk.
//
// TO RETIRE, once Ephor cuts any tag after the rename (say v0.4.0):
//   1. delete this replace directive
//   2. go get github.com/vul-os/ephor@v0.4.0
//   3. go mod tidy && go mod vendor
//   4. go build ./... && go test ./... -count=1
// That leaves a plain versioned require with real go.sum hashes, and the
// vendored tree becomes proxy-verifiable.
replace github.com/vul-os/ephor => ../ephor

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/time v0.10.0 // indirect
)
