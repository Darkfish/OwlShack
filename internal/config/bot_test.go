package config

import (
	"strings"
	"testing"
)

func cronTrigger(tmpl string) *TriggerConfig {
	t := &TriggerConfig{Type: "cron", Template: "x", Schedule: "*/5 * * * *"}
	t.Template = tmpl
	return t
}

// Validate parses the template against a stub func map, so a function added to the templater and
// not here is rejected on save: the UI reports "invalid template" for a template that works.
func TestValidate_AcceptsEveryTemplateFunction(t *testing.T) {
	for _, tmpl := range []string{
		`{{now.Year}}`,
		`{{date now "15:04"}}`,
		`{{date now "15:04" "Pacific/Auckland"}}`,
		`{{date .Timestamp "2006-01-02"}}`,
		`{{formatPathBytes .PathHashes}}`,
	} {
		if err := cronTrigger(tmpl).Validate(); err != nil {
			t.Errorf("%s should validate: %v", tmpl, err)
		}
	}
}

func TestValidate_RejectsUnknownFunction(t *testing.T) {
	err := cronTrigger(`{{strftime now "%H:%M"}}`).Validate()
	if err == nil {
		t.Fatal("want an error for a function that does not exist")
	}
	if !strings.Contains(err.Error(), "invalid template") {
		t.Errorf("got %v", err)
	}
}

// Mirroring answers an incoming message with the size it arrived on. A scheduled or feed trigger
// answers nothing, so asking it to mirror is a config that cannot mean what it says.
func TestTriggerConfig_MirrorPathHashSizeNeedsAnIncomingMessage(t *testing.T) {
	t.Parallel()
	mirror := uint8(MirrorIncomingPathHashSize)
	twoBytes := uint8(2)
	over := uint8(MaxPathHashSize + 1)

	for _, tc := range []struct {
		name    string
		cfg     TriggerConfig
		wantErr bool
	}{
		{"group mirrors", TriggerConfig{Type: "group", Template: "x", Channels: &ChannelList{{Name: "Public"}}, PathHashSize: &mirror}, false},
		{"dm mirrors", TriggerConfig{Type: "dm", Template: "x", PathHashSize: &mirror}, false},
		{"cron cannot mirror", TriggerConfig{Type: "cron", Template: "x", Schedule: "@every 1h", Contacts: &[]string{"aa"}, PathHashSize: &mirror}, true},
		{"rss cannot mirror", TriggerConfig{Type: "rss", Template: "x", URL: "https://example.org/f.xml", Contacts: &[]string{"aa"}, PathHashSize: &mirror}, true},
		{"cap cannot mirror", TriggerConfig{Type: "cap", Template: "x", URL: "https://example.org/f.xml", Contacts: &[]string{"aa"}, PathHashSize: &mirror}, true},
		{"cron takes a fixed size", TriggerConfig{Type: "cron", Template: "x", Schedule: "@every 1h", Contacts: &[]string{"aa"}, PathHashSize: &twoBytes}, false},
		{"past the maximum is rejected", TriggerConfig{Type: "dm", Template: "x", PathHashSize: &over}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("%s accepted a mirror pathHashSize; it answers no message", tc.cfg.Type)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("rejected a valid config: %v", err)
			}
		})
	}
}
