package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"net/netip"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type BuildHostDisk struct {
	Mount         string `json:"mount"`
	Total         int64  `json:"total"`
	Free          int64  `json:"free"`
	IsBuildVolume bool   `json:"is_build_volume"`
}
type BuildHostGPU struct {
	Name          string `json:"name"`
	Driver        string `json:"driver"`
	OpenGLVersion string `json:"opengl_version"`
}
type BuildHostFacts struct {
	OS                 BuildHostOS       `json:"os"`
	OSVersion          string            `json:"os_version"`
	Arch               string            `json:"arch"`
	Hostname           string            `json:"hostname"`
	LANIP              string            `json:"lan_ip"`
	CPUModel           string            `json:"cpu_model"`
	Cores              int64             `json:"cores"`
	Threads            int64             `json:"threads"`
	RAMBytes           int64             `json:"ram_bytes"`
	Disks              []BuildHostDisk   `json:"disks"`
	GPUs               []BuildHostGPU    `json:"gpus"`
	Toolchains         map[string]string `json:"toolchains"`
	InteractiveSession bool              `json:"interactive_session"`
	BuildUserIsAdmin   bool              `json:"build_user_is_admin"`
	AgentVersion       string            `json:"agent_version"`
}

// DecodeStrictObject rejects ambiguous JSON before decoding a fixed schema.
func DecodeStrictObject(raw []byte, out any) error {
	bad := func() error { return Invalidf("invalid report object") }
	if !utf8.Valid(raw) {
		return bad()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil || t == nil {
			return bad()
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return bad()
					}
					s, ok := k.(string)
					if !ok || seen[s] {
						return bad()
					}
					seen[s] = true
					if e = walk(); e != nil {
						return e
					}
				}
			case '[':
				for d.More() {
					if e := walk(); e != nil {
						return e
					}
				}
			default:
				return bad()
			}
			_, e = d.Token()
			return e
		}
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return bad()
	}
	if e := walk(); e != nil {
		return bad()
	}
	if _, e := d.Token(); e != io.EOF {
		return bad()
	}
	if !exactReportFields(raw, reflect.TypeOf(out)) {
		return bad()
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return bad()
	}
	return nil
}
func reportText(s string, max int, required bool) bool {
	return (!required || s != "") && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}
func ParseBuildHostFacts(raw []byte) (BuildHostFacts, error) {
	var f BuildHostFacts
	bad := func() (BuildHostFacts, error) { return f, Invalidf("invalid machine facts") }
	if len(raw) > 65536 || DecodeStrictObject(raw, &f) != nil {
		return bad()
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	for _, k := range []string{"os", "os_version", "arch", "hostname", "lan_ip", "cpu_model", "cores", "threads", "ram_bytes", "disks", "gpus", "toolchains", "interactive_session", "build_user_is_admin", "agent_version"} {
		if _, ok := fields[k]; !ok {
			return bad()
		}
	}
	if f.OS != "linux" && f.OS != "windows" && f.OS != "macos" || !buildArchPattern.MatchString(f.Arch) || !reportText(f.Hostname, 255, true) || !reportText(f.OSVersion, 256, false) || !reportText(f.CPUModel, 512, false) || !reportText(f.AgentVersion, 64, true) || f.Cores < 1 || f.Threads < f.Cores || f.RAMBytes < 0 {
		return bad()
	}
	if f.LANIP != "" {
		if ip, e := netip.ParseAddr(f.LANIP); e != nil || ip.Zone() != "" {
			return bad()
		}
	}
	if len(f.Disks) > 128 || len(f.GPUs) > 32 || len(f.Toolchains) > 64 {
		return bad()
	}
	mounts := map[string]bool{}
	for _, v := range f.Disks {
		if !reportText(v.Mount, 1024, true) || mounts[v.Mount] || v.Total < 0 || v.Free < 0 || v.Free > v.Total {
			return bad()
		}
		mounts[v.Mount] = true
	}
	for _, v := range f.GPUs {
		if !reportText(v.Name, 256, true) || !reportText(v.Driver, 256, false) || !reportText(v.OpenGLVersion, 256, false) {
			return bad()
		}
	}
	// Nested fixed-schema objects require every field, including zero-valued fields.
	for key, required := range map[string][]string{"disks": {"mount", "total", "free", "is_build_volume"}, "gpus": {"name", "driver", "opengl_version"}} {
		var arr []map[string]json.RawMessage
		json.Unmarshal(fields[key], &arr)
		for _, obj := range arr {
			for _, k := range required {
				if _, ok := obj[k]; !ok {
					return bad()
				}
			}
		}
	}
	for k, v := range f.Toolchains {
		if !capabilityPattern.MatchString(k) || !reportText(v, 256, true) {
			return bad()
		}
	}
	return f, nil
}

var desktopGL = regexp.MustCompile(`^([0-9]+)\.([0-9]+)(?:\s.*)?$`)

func SuggestedBuildHostCapabilities(f BuildHostFacts) []string {
	out := []string{}
	for _, k := range []string{"msvc", "dotnet", "cmake", "ninja", "gcc", "clang", "qt", "xcode", "python", "docker"} {
		if f.Toolchains[k] != "" {
			out = append(out, k)
		}
	}
	if f.InteractiveSession {
		for _, g := range f.GPUs {
			m := desktopGL.FindStringSubmatch(g.OpenGLVersion)
			if m == nil {
				continue
			}
			major, e := strconv.ParseUint(m[1], 10, 32)
			minor, e2 := strconv.ParseUint(m[2], 10, 32)
			if e == nil && e2 == nil && (major > 3 || major == 3 && minor >= 3) {
				out = append(out, "gl")
				break
			}
		}
	}
	slices.Sort(out)
	return out
}

// encoding/json also matches field names case-insensitively; the wire schema does not.
func exactReportFields(raw []byte, t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	switch t.Kind() {
	case reflect.Struct:
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		for key, value := range obj {
			ft, ok := fields[key]
			if !ok || !exactReportFields(value, ft) {
				return false
			}
		}
	case reflect.Slice:
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) != nil {
			return false
		}
		for _, v := range arr {
			if !exactReportFields(v, t.Elem()) {
				return false
			}
		}
	}
	return true
}
