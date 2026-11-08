package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const validHostFacts = `{"os":"linux","os_version":"","arch":"amd64","hostname":"builder","lan_ip":"","cpu_model":"","cores":2,"threads":4,"ram_bytes":0,"disks":[],"gpus":[],"toolchains":{},"interactive_session":false,"build_user_is_admin":false,"agent_version":"1"}`

func TestBuildHostFactsValidation(t *testing.T) {
	if _, err := ParseBuildHostFacts([]byte(validHostFacts)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `null`, validHostFacts + `{}`, `{"os":"linux","os":"windows"}`, `{"toolchains":{"cmake":null}}`} {
		if _, err := ParseBuildHostFacts([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var obj map[string]any
	json.Unmarshal([]byte(validHostFacts), &obj)
	for key := range obj {
		copyObj := map[string]any{}
		for k, v := range obj {
			copyObj[k] = v
		}
		delete(copyObj, key)
		raw, _ := json.Marshal(copyObj)
		if _, err := ParseBuildHostFacts(raw); err == nil {
			t.Errorf("missing %s accepted", key)
		}
	}
}
func TestSuggestedBuildHostCapabilities(t *testing.T) {
	for _, tc := range []string{"msvc", "dotnet", "cmake", "ninja", "gcc", "clang", "qt", "xcode", "python", "docker", "other"} {
		f := BuildHostFacts{Toolchains: map[string]string{tc: "1"}}
		got := SuggestedBuildHostCapabilities(f)
		want := 1
		if tc == "other" {
			want = 0
		}
		if len(got) != want {
			t.Errorf("%s: %v", tc, got)
		}
	}
	for _, test := range []struct {
		v           string
		session, gl bool
	}{{"3.3", true, true}, {"4.6 Vendor", true, true}, {"3.2", true, false}, {"OpenGL ES 4.0", true, false}, {"3.3", false, false}, {"3.3oops", true, false}} {
		f := BuildHostFacts{InteractiveSession: test.session, GPUs: []BuildHostGPU{{OpenGLVersion: test.v}}}
		if (len(SuggestedBuildHostCapabilities(f)) > 0) != test.gl {
			t.Errorf("%+v", test)
		}
	}
}
func TestBuildHostOnline(t *testing.T) {
	now := time.Now()
	seen := now.Add(-180 * time.Second)
	h := BuildHost{Status: BuildHostApproved, LastSeenAt: &seen}
	if h.Online(now) {
		t.Fatal("boundary online")
	}
	seen = seen.Add(time.Millisecond)
	if !h.Online(now) {
		t.Fatal("recent offline")
	}
	h.Status = BuildHostPending
	if h.Online(now) {
		t.Fatal("pending online")
	}
}

func TestFactsRejectCaseAliases(t *testing.T) {
	raw := strings.Replace(validHostFacts, `"cores":2`, `"cores":2,"Cores":3`, 1)
	if _, e := ParseBuildHostFacts([]byte(raw)); e == nil {
		t.Fatal("case alias accepted")
	}
}

func TestFactsSchemaBoundaries(t *testing.T) {
	for _, os := range []string{"linux", "windows", "macos"} {
		raw := strings.Replace(validHostFacts, `"linux"`, `"`+os+`"`, 1)
		if _, e := ParseBuildHostFacts([]byte(raw)); e != nil {
			t.Fatal(os, e)
		}
	}
	var obj map[string]json.RawMessage
	json.Unmarshal([]byte(validHostFacts), &obj)
	for k := range obj {
		for _, v := range []string{`null`, `[]`, `{}`, `"wrong"`, `1`, `true`} {
			copyObj := map[string]json.RawMessage{}
			for key, value := range obj {
				copyObj[key] = value
			}
			copyObj[k] = json.RawMessage(v)
			raw, _ := json.Marshal(copyObj)
			_, e := ParseBuildHostFacts(raw)
			// Some substitutions are legitimate for the corresponding type.
			valid := (v == `[]` && (k == "disks" || k == "gpus")) || (v == `{}` && k == "toolchains") || (v == `1` && (k == "cores" || k == "ram_bytes")) || (v == `true` && (k == "interactive_session" || k == "build_user_is_admin")) || (v == `"wrong"` && (k == "os_version" || k == "arch" || k == "hostname" || k == "cpu_model" || k == "agent_version"))
			if (e == nil) != valid {
				t.Errorf("%s=%s error %v", k, v, e)
			}
		}
	}
	cases := map[string][]string{
		"os": {`"bsd"`}, "arch": {`"AMD64"`, `""`}, "hostname": {`""`, `"a\nb"`}, "lan_ip": {`"localhost"`, `"fe80::1%eth0"`, `"1.2.3.999"`}, "cores": {`0`, `-1`, `1.1`, `1e2`, `9223372036854775808`}, "threads": {`1`}, "ram_bytes": {`-1`, `9223372036854775808`}, "agent_version": {`""`},
		"disks": {`[{"mount":"/","total":1,"free":2,"is_build_volume":true}]`, `[{"mount":"/","total":1,"free":0}]`, `[{"mount":"/","total":1,"free":0,"is_build_volume":false,"extra":0}]`, `[{"mount":"/","mount":"x","total":1,"free":0,"is_build_volume":false}]`, `[{"mount":"/","total":1,"free":0,"is_build_volume":false},{"mount":"/","total":1,"free":0,"is_build_volume":true}]`},
		"gpus":  {`[{"name":"gpu","driver":""}]`, `[{"name":"","driver":"","opengl_version":""}]`}, "toolchains": {`{"Bad":"1"}`, `{"cmake":""}`, `{"cmake":"1","cmake":"2"}`},
	}
	for k, values := range cases {
		for _, v := range values {
			copyObj := map[string]json.RawMessage{}
			for key, value := range obj {
				copyObj[key] = value
			}
			copyObj[k] = json.RawMessage(v)
			raw, _ := json.Marshal(copyObj)
			if _, e := ParseBuildHostFacts(raw); e == nil {
				t.Errorf("accepted %s=%s", k, v)
			}
		}
	}
	for k, max := range map[string]int{"hostname": 255, "os_version": 256, "cpu_model": 512, "agent_version": 64} {
		copyObj := map[string]json.RawMessage{}
		for key, value := range obj {
			copyObj[key] = value
		}
		for _, n := range []int{max, max + 1} {
			copyObj[k], _ = json.Marshal(strings.Repeat("a", n))
			raw, _ := json.Marshal(copyObj)
			_, e := ParseBuildHostFacts(raw)
			if (e == nil) != (n == max) {
				t.Errorf("%s length %d: %v", k, n, e)
			}
		}
	}
	for _, n := range []int{65536, 65537} {
		raw := []byte(validHostFacts[:1] + strings.Repeat(" ", n-len(validHostFacts)) + validHostFacts[1:])
		_, e := ParseBuildHostFacts(raw)
		if (e == nil) != (n == 65536) {
			t.Fatal(n, e)
		}
	}
	if _, e := ParseBuildHostFacts(append([]byte(validHostFacts[:1]), append([]byte{0xff}, []byte(validHostFacts[1:])...)...)); e == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestFactsCollectionLimits(t *testing.T) {
	f, e := ParseBuildHostFacts([]byte(validHostFacts))
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []int{128, 129} {
		f.Disks = []BuildHostDisk{}
		for i := 0; i < n; i++ {
			f.Disks = append(f.Disks, BuildHostDisk{Mount: fmt.Sprintf("/%d", i), Total: 1, Free: 0})
		}
		raw, _ := json.Marshal(f)
		_, e := ParseBuildHostFacts(raw)
		if (e == nil) != (n == 128) {
			t.Fatal(n, e)
		}
	}
	f.Disks = []BuildHostDisk{}
	for _, n := range []int{32, 33} {
		f.GPUs = []BuildHostGPU{}
		for i := 0; i < n; i++ {
			f.GPUs = append(f.GPUs, BuildHostGPU{Name: "GPU"})
		}
		raw, _ := json.Marshal(f)
		_, e := ParseBuildHostFacts(raw)
		if (e == nil) != (n == 32) {
			t.Fatal(n, e)
		}
	}
	f.GPUs = []BuildHostGPU{}
	for _, n := range []int{64, 65} {
		f.Toolchains = map[string]string{}
		for i := 0; i < n; i++ {
			f.Toolchains[fmt.Sprintf("tool_%d", i)] = "1"
		}
		raw, _ := json.Marshal(f)
		_, e := ParseBuildHostFacts(raw)
		if (e == nil) != (n == 64) {
			t.Fatal(n, e)
		}
	}
}
func TestBuildHostConnectivityStates(t *testing.T) {
	now := time.Unix(1000, 0)
	seen := now.Add(-time.Second)
	for _, status := range []BuildHostStatus{BuildHostPending, BuildHostApproved, BuildHostRevoked} {
		for _, enabled := range []bool{false, true} {
			for _, last := range []*time.Time{nil, &seen} {
				h := BuildHost{Status: status, Enabled: enabled, LastSeenAt: last}
				want := status == BuildHostApproved && last != nil
				if h.Online(now) != want {
					t.Fatal(h)
				}
			}
		}
	}
}
