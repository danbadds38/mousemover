// Package assets embeds the tray icons.
package assets

import _ "embed"

// ActiveICO is shown while nudging is enabled.
//
//go:embed active.ico
var ActiveICO []byte

// IdleICO is shown while nudging is disabled.
//
//go:embed idle.ico
var IdleICO []byte
