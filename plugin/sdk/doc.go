// Package sdk is the Go authoring side of nulo's native plugin ABI.
//
// A plugin is a Go main package built as a C shared library:
//
//	go build -buildmode=c-shared -o myplugin.dll ./myplugin
//
// The main package only needs the three ABI wrappers shown in
// examples/plugin. Everything else lives here: register tools and commands,
// subscribe to Pi's extension events, and call back into the host for session,
// model, UI and persistence actions.
//
// The package does not import the nulo SDK. A plugin is an independent Go
// program, so it can use a different Go version and different dependencies
// from the host.
package sdk
