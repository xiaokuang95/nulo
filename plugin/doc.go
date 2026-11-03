// Package plugin is nulo's native plugin contract.
//
// Plugins are shared libraries built by plugin authors. The host loads the
// library at runtime and talks to it over a small C ABI carrying JSON. There is
// no JavaScript or Node runtime, and the plugin does not have to be built with
// the same Go version as the host.
//
// The ABI is intentionally tiny:
//
//	NuloPluginInit(hostInfo *char, hostInfoLen int, hostCall uintptr) *char
//	NuloPluginCall(request *char, requestLen int) *char
//	NuloPluginFree(value *char)
//
// Init returns a Registration; Call executes one event, tool or command.
// HostCall is a function pointer the plugin uses for API calls such as
// getActiveTools or sendUserMessage. See package plugin/sdk for the Go side of
// the contract.
package plugin
