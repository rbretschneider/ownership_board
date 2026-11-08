package sqlite

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildJobsMigrationPreservesHosts(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "upgrade.db"))
	if e := m.Migrate(15); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO build_hosts(name,os,arch,public_key,created_at,running_job_ids,free_disk_bytes) VALUES('Builder','linux','amd64','key','2026-01-01T00:00:00.000Z','[1,42]',100)`); e != nil {
		t.Fatal(e)
	}
	before := dumpTable(t, db, "build_hosts")
	if e := m.Migrate(16); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, dumpTable(t, db, "build_hosts")) {
		t.Fatal("upgrade changed hosts")
	}
	if e := m.Migrate(15); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, dumpTable(t, db, "build_hosts")) {
		t.Fatal("downgrade changed hosts")
	}
}
