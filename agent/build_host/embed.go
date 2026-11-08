// Package buildhostassets contains the runtime and fixed installer payloads.
package buildhostassets

import "embed"

// Agent is the canonical Python runtime.
//
//go:embed build_host.py
var Agent []byte

// Requirements locks the runtime dependency distributions.
//
//go:embed requirements.txt
var Requirements []byte

// Templates are platform-specific installers, without deployment credentials.
//
//go:embed install/build-host.sh install/build-host.ps1
var Templates embed.FS
