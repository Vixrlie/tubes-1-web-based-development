package dbm

import (
	"context"
	"os"
	"testing"
)

func TestDebugBothStates(t *testing.T) {
	// Butuh target-db Postgres (5433) dan MySQL (5435) yang menyala.
	if os.Getenv("DBM_DEBUG_TARGETS") == "" {
		t.Skip("set DBM_DEBUG_TARGETS=1 untuk menjalankan")
	}
	ctx := context.Background()
	for _, raw := range []string{
		"postgres://target:target@127.0.0.1:5433/sample",
		"mysql://target:target@127.0.0.1:5435/sample",
	} {
		tgt, err := OpenTarget(ctx, raw)
		if err != nil {
			t.Fatalf("open %s: %v", raw, err)
		}
		st, err := loadSchemaState(ctx, tgt)
		if err != nil {
			t.Errorf("loadState %s: %v", raw, err)
		} else {
			t.Logf("%s -> tabel=%d fk=%d", raw, len(st.Tables), len(st.FKs))
		}
		tgt.Close()
	}
}
