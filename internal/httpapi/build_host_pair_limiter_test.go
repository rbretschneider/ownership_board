package httpapi

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newBuildHostPairLimiter(func() time.Time { return now })
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.allow("127.0.0.1") == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 5 {
		t.Fatal(successes.Load())
	}
	now = now.Add(time.Minute)
	if e := l.allow("127.0.0.1"); e != nil {
		t.Fatal(e)
	}
}

func TestPairLimiterBoundAndCanonicalIPs(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newBuildHostPairLimiter(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if e := l.allow("::ffff:127.0.0.1"); e != nil {
			t.Fatal(e)
		}
	}
	if e := l.allow("127.0.0.1"); e == nil {
		t.Fatal("mapped address got a second quota")
	}
	for i := 1; i < 4096; i++ {
		if e := l.allow(fmt.Sprintf("10.0.%d.%d", i/256, i%256)); e != nil {
			t.Fatal(i, e)
		}
	}
	if len(l.entries) != 4096 {
		t.Fatal(len(l.entries))
	}
	if e := l.allow("192.0.2.1"); e == nil {
		t.Fatal("capacity ignored")
	}
	if len(l.entries) != 4096 {
		t.Fatal(len(l.entries))
	}
	now = now.Add(time.Minute)
	if e := l.allow("192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	if len(l.entries) != 1 {
		t.Fatal(len(l.entries))
	}
}
