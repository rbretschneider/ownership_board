// Package install renders fixed, anonymous build worker downloads.
package install

import (
	"encoding/base64"
	"fmt"
	assets "github.com/rbretschneider/ownership_board/agent/build_host"
	"strings"
)

// Render uses embedded bytes only: no request or deployment data enters a script.
func Render(kind string) ([]byte, error) {
	if kind != "sh" && kind != "ps1" {
		return nil, fmt.Errorf("unknown installer format")
	}
	template, err := assets.Templates.ReadFile("install/build-host." + kind)
	if err != nil {
		return nil, err
	}
	return []byte(strings.NewReplacer("@@AGENT_BASE64@@", base64.StdEncoding.EncodeToString(assets.Agent), "@@REQUIREMENTS_BASE64@@", base64.StdEncoding.EncodeToString(assets.Requirements)).Replace(string(template))), nil
}
