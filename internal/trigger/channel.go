package trigger

import (
	"context"
	"log/slog"
	"regexp"
	"sync"

	"github.com/meshcore-go/OwlShack/internal/config"
	"github.com/meshcore-go/OwlShack/internal/logging"
	meshcore "github.com/meshcore-go/meshcore-go"
	"github.com/meshcore-go/meshcore-go/node"
)

type ChannelTrigger struct {
	cfg      config.TriggerConfig
	botName  string
	node     *node.Node
	patterns []*regexp.Regexp
	channels map[string]bool // channel names this trigger listens on; nil = all
	log      *slog.Logger

	mu       sync.Mutex
	callback Callback
}

func NewChannelTrigger(botName string, cfg config.TriggerConfig, n *node.Node, channels []*meshcore.ChannelEntry, log *slog.Logger) (*ChannelTrigger, error) {
	patterns, err := compilePatterns(cfg.Match)
	if err != nil {
		return nil, err
	}

	var channelFilter map[string]bool
	if len(channels) > 0 {
		channelFilter = make(map[string]bool, len(channels))
		for _, ch := range channels {
			channelFilter[ch.Name] = true
		}
	}

	return &ChannelTrigger{
		cfg:      cfg,
		botName:  botName,
		node:     n,
		patterns: patterns,
		channels: channelFilter,
		log:      log.With("trigger", "channel"),
	}, nil
}

// Start only stores the callback: group text arrives via the companion's persistent GrpTxt handler, because node.OnPacket cannot deregister.
func (t *ChannelTrigger) Start(_ context.Context, callback Callback) error {
	t.mu.Lock()
	t.callback = callback
	t.mu.Unlock()
	return nil
}

// Stop clears the callback so HandleGroupText is a no-op even if the dispatcher still holds this trigger.
func (t *ChannelTrigger) Stop() error {
	t.mu.Lock()
	t.callback = nil
	t.mu.Unlock()
	return nil
}

// HandleGroupText is invoked by the companion's persistent GrpTxt handler for every received group message.
func (t *ChannelTrigger) HandleGroupText(pkt *meshcore.Packet) {
	t.mu.Lock()
	cb := t.callback
	t.mu.Unlock()
	if cb == nil {
		return // stopped or not yet started
	}

	msg, ch, err := t.node.DecryptGroupText(pkt)
	if err != nil {
		t.log.Log(context.Background(), logging.LevelTrace, "group decrypt failed", "error", err)
		return
	}

	t.log.Log(context.Background(), logging.LevelTrace, "group message received",
		"channel", ch.Name, "sender", msg.Sender,
		"text", msg.Text, "snr", pkt.SNR, "rssi", pkt.RSSI)

	// Our own sends are heard back over the air; matching them would let a bot trigger on itself and loop.
	if msg.Sender == t.botName {
		t.log.Log(context.Background(), logging.LevelTrace, "own message, skipping trigger",
			"channel", ch.Name)
		return
	}

	if t.channels != nil && !t.channels[ch.Name] {
		t.log.Log(context.Background(), logging.LevelTrace, "channel not matched, skipping",
			"received", ch.Name, "listening", t.channelNames())
		return
	}

	captures := t.matchesAny(msg.Text)
	if captures == nil {
		t.log.Log(context.Background(), logging.LevelTrace, "no pattern matched",
			"channel", ch.Name, "text", msg.Text, "patterns", t.patternStrings())
		return
	}

	t.log.Log(context.Background(), logging.LevelTrace, "trigger matched", "captures", captures)

	cb(Event{
		Type:    "channel",
		BotName: t.botName,
		Data: map[string]any{
			"Sender":       msg.Sender,
			"Channel":      ch.Name,
			"ChannelEntry": ch,
			"Message":      msg.Text,
			"Match":        captures,
			"Timestamp":    msg.Timestamp,
			"SNR":          pkt.SNR,
			"RSSI":         pkt.RSSI,
			"Hops":         pkt.PathHashCount(),
			"PathHashes":   pkt.PathHashes(),
			"PathHashSize": pkt.PathHashSize(),
		},
	})
}

func (t *ChannelTrigger) matchesAny(text string) map[string]string {
	captures := matchCaptures(t.patterns, text)
	t.log.Log(context.Background(), logging.LevelTrace, "regex check",
		"text", text, "patterns", t.patternStrings(), "matched", captures != nil)
	return captures
}

func (t *ChannelTrigger) channelNames() []string {
	names := make([]string, 0, len(t.channels))
	for name := range t.channels {
		names = append(names, name)
	}
	return names
}

func (t *ChannelTrigger) patternStrings() []string {
	return patternStrings(t.patterns)
}

var _ Trigger = (*ChannelTrigger)(nil)
