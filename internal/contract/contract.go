// Package contract pins the plugin<->core protocol version.
//
// The OpenCode plugin refuses to talk to a core that prints a different
// value, so the contract evolves only through explicit versions.
package contract

// Version is the Regulus plugin<->core contract.
const Version = "regulus.contract/v1"

// Supported lists every plugin<->core contract this core speaks.
// Version negotiation accepts only these; anything else refuses.
var Supported = []string{Version}

// Supports reports whether v is a supported plugin<->core contract.
func Supports(v string) bool {
	for _, s := range Supported {
		if v == s {
			return true
		}
	}
	return false
}
