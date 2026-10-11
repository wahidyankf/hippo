// Package hostprobe contains only the external host-query wire protocol and client.
// It has no application, renderer or parent fixture dependency.
package hostprobe

import "slices"

// HelperEnvironment marks only the external collector helper.
const HelperEnvironment = "PUBLIC_TUI_HOST_PROBE_HELPER"

// HelperEntry is the exact existing testing entry used by native query wrappers.
const HelperEntry = "TestBDDPublicTUIHostProbeHelper"

// Request preserves the existing nonce-bound process/query wire fields.
type Request struct {
	Nonce       string   `json:"Nonce"`
	Executable  string   `json:"Executable"`
	Name        string   `json:"Name"`
	RaceOptions string   `json:"RaceOptions"`
	PID         int      `json:"PID"`
	ParentPID   int      `json:"ParentPID"`
	Arguments   []string `json:"Arguments"`
}

// Response carries external query output or refusal.
type Response struct {
	Output string `json:"Output"`
	Error  string `json:"Error"`
}

const sysctlTool = "sysctl"

var commands = [][]string{
	{"ps", "-A", "-o", "%cpu="},
	{"memory_pressure", "-Q"},
	{"vm_stat"},
	{sysctlTool, "-n", "vm.swapusage"},
	{sysctlTool, "-n", "kern.memorystatus_vm_pressure_level"},
	{sysctlTool, "-n", "vm.compressor_available"},
	{sysctlTool, "-n", "vm.compressor_bytes_used"},
}

// Commands returns an owned copy of the seven production collector queries.
func Commands() [][]string {
	result := make([][]string, len(commands))
	for index, command := range commands {
		result[index] = slices.Clone(command)
	}
	return result
}

// CommandIndex rejects every query outside the exact production whitelist.
func CommandIndex(request Request) int {
	for index, command := range commands {
		if command[0] == request.Name && slices.Equal(command[1:], request.Arguments) {
			return index
		}
	}
	return -1
}
