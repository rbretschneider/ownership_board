package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestHostSignature(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	now := time.Unix(1000, 0)
	body := []byte(`{}`)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, HostSignatureMessage("POST", "/test", "1000", body)))
	for _, tc := range []struct {
		body []byte
		at   time.Time
		ok   bool
	}{{body, now, true}, {[]byte(`{ }`), now, false}, {body, now.Add(300 * time.Second), true}, {body, now.Add(-300 * time.Second), true}, {body, now.Add(301 * time.Second), false}} {
		if VerifyHostSignature(key.Public().(ed25519.PublicKey), "POST", "/test", "1000", sig, tc.body, tc.at) != tc.ok {
			t.Fatal(tc)
		}
	}
}

func TestHostSignatureProtocolVector(t *testing.T) {
	want := "POST\n/api/v1/build-host/heartbeat\n1700000000\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if got := string(HostSignatureMessage("POST", "/api/v1/build-host/heartbeat", "1700000000", []byte(`{}`))); got != want {
		t.Fatalf("%q", got)
	}
	want = "GET\n/api/v1/build-host/status\n1700000000\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := string(HostSignatureMessage("GET", "/api/v1/build-host/status", "1700000000", nil)); got != want {
		t.Fatalf("%q", got)
	}
}
func TestHostSignatureTampering(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	ts := "1000"
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, HostSignatureMessage("POST", "/path", ts, []byte(`{}`))))
	otherSeed := make([]byte, 32)
	otherSeed[0] = 1
	otherKey := ed25519.NewKeyFromSeed(otherSeed)
	for _, test := range []struct {
		method, path, ts, sig string
		key                   ed25519.PublicKey
	}{{"GET", "/path", ts, sig, key.Public().(ed25519.PublicKey)}, {"POST", "/other", ts, sig, key.Public().(ed25519.PublicKey)}, {"POST", "/path", "01000", sig, key.Public().(ed25519.PublicKey)}, {"POST", "/path", ts, sig, otherKey.Public().(ed25519.PublicKey)}, {"POST", "/path", ts, sig + "\n", key.Public().(ed25519.PublicKey)}, {"POST", "/path", ts, "bad", key.Public().(ed25519.PublicKey)}} {
		if VerifyHostSignature(test.key, test.method, test.path, test.ts, test.sig, []byte(`{}`), time.Unix(1000, 0)) {
			t.Fatal(test)
		}
	}
}

// These public deterministic credentials are fixtures, never enrollment identities.
func TestHostSignatureSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/host_signature_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Seed, PublicKey, Method, Path, Timestamp, Body, Message, Signature string }
	// JSON uses snake_case for the public key.
	var entries []map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, v := range entries {
		vectors = append(vectors, struct{ Seed, PublicKey, Method, Path, Timestamp, Body, Message, Signature string }{v["seed"], v["public_key"], v["method"], v["path"], v["timestamp"], v["body"], v["message"], v["signature"]})
	}
	for _, v := range vectors {
		seed, err := hex.DecodeString(v.Seed)
		if err != nil {
			t.Fatal(err)
		}
		key := ed25519.NewKeyFromSeed(seed)
		public, err := base64.StdEncoding.DecodeString(v.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		ts, err := strconv.ParseInt(v.Timestamp, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if string(HostSignatureMessage(v.Method, v.Path, v.Timestamp, []byte(v.Body))) != v.Message {
			t.Fatal("canonical bytes differ")
		}
		if got := base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(v.Message))); got != v.Signature {
			t.Fatal("signature differs")
		}
		if !VerifyHostSignature(public, v.Method, v.Path, v.Timestamp, v.Signature, []byte(v.Body), time.Unix(ts, 0)) {
			t.Fatal("shared signature rejected")
		}
		if VerifyHostSignature(public, v.Method, v.Path, v.Timestamp, v.Signature, []byte(v.Body+" "), time.Unix(ts, 0)) {
			t.Fatal("tampered bytes accepted")
		}
	}
}
