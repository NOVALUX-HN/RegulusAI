// Package contract pins the plugin<->core protocol version.
//
// The OpenCode plugin refuses to talk to a core that prints a different
// value, so the contract evolves only through explicit versions.
package contract

// Version is the Regulus plugin<->core contract.
const Version = "regulus.contract/v1"
