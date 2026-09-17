package store

import (
	"path/filepath"
	"testing"
)

func TestStore_UpgradeFailoverAndOpenHop(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"pre-feature", "upstream-v15", "failover-v15"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "upgrade.db")
			db, err := openWritableDB(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, migration := range migrations[:14] {
				if err := migration(ctx, db); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, `
				INSERT INTO settings (id, connection) VALUES (1, 'tcp://radio:5000');
				INSERT INTO companions (id, name) VALUES (1, 'backup');
				INSERT INTO triggers (id, companion_id, type, template) VALUES (1, 1, 'group', 'reply');
				PRAGMA user_version = 14;
			`); err != nil {
				t.Fatal(err)
			}
			var wantPattern, wantToken string
			var wantTimeout int64
			switch source {
			case "upstream-v15":
				if _, err := db.ExecContext(ctx, `
					ALTER TABLE settings ADD COLUMN modem_token TEXT;
					UPDATE settings SET modem_token = 'saved-token';
					PRAGMA user_version = 15;
				`); err != nil {
					t.Fatal(err)
				}
				wantToken = "saved-token"
			case "failover-v15":
				if _, err := db.ExecContext(ctx, `
					ALTER TABLE triggers ADD COLUMN failover_pattern TEXT NOT NULL DEFAULT '';
					ALTER TABLE triggers ADD COLUMN failover_timeout INTEGER NOT NULL DEFAULT 0;
					UPDATE triggers SET failover_pattern = '^pong$', failover_timeout = 5;
					PRAGMA user_version = 15;
				`); err != nil {
					t.Fatal(err)
				}
				wantPattern, wantTimeout = "^pong$", 5
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				st, err := Open(ctx, path)
				if err != nil {
					t.Fatalf("Open: %v", err)
				}
				defer st.Close()
				var version int
				if err := st.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
					t.Fatal(err)
				}
				if version != 16 {
					t.Fatalf("schema version = %d, want 16", version)
				}
				settings, err := st.Settings.Get(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if settings.Connection == nil || *settings.Connection != "tcp://radio:5000" {
					t.Fatalf("connection was not preserved: %+v", settings)
				}
				if wantToken == "" {
					if settings.ModemToken != nil {
						t.Fatalf("new modem token = %q, want NULL", *settings.ModemToken)
					}
				} else if settings.ModemToken == nil || *settings.ModemToken != wantToken {
					t.Fatal("existing modem token was not preserved")
				}
				trigger, err := st.Triggers.Get(ctx, 1)
				if err != nil {
					t.Fatal(err)
				}
				if trigger.Template != "reply" || trigger.FailoverPattern != wantPattern || trigger.FailoverTimeout != wantTimeout {
					t.Fatalf("trigger settings after upgrade: %+v", trigger)
				}
				if err := st.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
