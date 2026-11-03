// Package sdk is the Go authoring side of nulo's plugin runtime.
//
// A plugin can be built as a C shared library:
//
//	go build -buildmode=c-shared -o myplugin.dll ./myplugin
//
// or as a standalone protocol process:
//
//	go build -o myplugin ./myplugin
//
// The protocol process calls ServeProtocol from main. The shared-library
// package exports the three ABI wrappers shown in examples/plugin. Both modes
// use the same factory: register tools and commands, subscribe to Pi's
// extension events, and call back into the host for session, model, UI and
// persistence actions.
//
// The package does not import the nulo SDK. A plugin is an independent Go
// program, so it can use a different Go version and different dependencies
// from the host.
package sdk
