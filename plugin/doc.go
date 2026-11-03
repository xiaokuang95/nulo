// Package plugin is nulo's plugin contract.
//
// A plugin can be hosted in one of two ways:
//
//   - SourceLibrary: an in-process shared library loaded through the C ABI
//     below. This is the lowest-latency mode.
//   - SourceProtocol: a separate executable speaking newline-delimited JSON
//     over stdin/stdout. The host starts the process, sends an init frame,
//     and then exchanges Invoke/HostCall frames. This mode is language-neutral
//     and does not require Node.
//
// The shared-library ABI is intentionally tiny:
//
//	NuloPluginInit(hostInfo *char, hostInfoLen int, hostCall uintptr) *char
//	NuloPluginCall(request *char, requestLen int) *char
//	NuloPluginFree(value *char)
//
// Init returns a Registration; Call executes one event, tool or command.
// HostCall is a function pointer the plugin uses for API calls such as
// getActiveTools or sendUserMessage. See package plugin/sdk for the Go side of
// both modes.
package plugin
