package domain

import "testing"

func TestSupportsBatches(t *testing.T) {
	for v, want := range map[string]bool{"1.1": true, "1.2": true, "2.0": true, "1.10": true, "1.0": false, "1": false, "": false, "dev": false, "0.9": false} {
		if got := SupportsBatches(v); got != want {
			t.Errorf("SupportsBatches(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestRestyleBatching(t *testing.T) {
	r := Repo{BatchMax: DefaultBatchSize}
	on, size := true, 6
	if err := r.Restyle(RepoStyle{BatchVerify: &on, BatchMax: &size}); err != nil || !r.BatchVerify || r.BatchMax != 6 {
		t.Fatalf("restyle = %+v, %v", r, err)
	}
	for _, bad := range []int{1, 9} {
		if err := r.Restyle(RepoStyle{BatchMax: &bad}); KindOf(err) != KindInvalid {
			t.Fatalf("batch_max %d accepted: %v", bad, err)
		}
	}
	if !(RepoStyle{BatchMax: &size}).ChangesBatching() || (RepoStyle{}).ChangesBatching() {
		t.Fatal("ChangesBatching")
	}
}
