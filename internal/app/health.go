package app

import (
	"sync/atomic"
	"time"

	"github.com/meshcore-go/OwlShack/internal/api"
)

// radioActivity records when the radio last carried traffic. It is process-scoped rather than a
// field on backend, because a reload installs a new backend and the radio's history does not stop
// at that boundary. Zero means "not since this process started", which is not the same as "never".
type radioActivity struct{ lastRx, lastTx atomic.Int64 }

func (a *radioActivity) rx() { a.lastRx.Store(time.Now().UnixNano()) }
func (a *radioActivity) tx() { a.lastTx.Store(time.Now().UnixNano()) }

// radioSeen is written by the packet logger, which sees every frame in both directions.
var radioSeen radioActivity

// secsSinceTime is secsSince for a time.Time. A zero time.Time does not have a zero UnixNano — it
// is a large negative number — so the emptiness test has to be IsZero, not the nanos.
func secsSinceTime(t, now time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	secs := int64(now.Sub(t).Seconds())
	return &secs
}

// secsSince renders an age for the wire, nil where there is nothing to measure from, so a monitor
// can tell a silent radio from one that cannot be asked.
func secsSince(nanos int64, now time.Time) *int64 {
	if nanos == 0 {
		return nil
	}
	secs := int64(now.Sub(time.Unix(0, nanos)).Seconds())
	return &secs
}

func (b *backend) Health() api.HealthInfo {
	return b.health(time.Now(), &radioSeen)
}

// health composes the snapshot. now and act are parameters so a test can drive both without a
// clock or a radio.
func (b *backend) health(now time.Time, act *radioActivity) api.HealthInfo {
	info := api.HealthInfo{
		Problems:   []string{},
		Radio:      b.radioHealth(now, act),
		Companions: []api.CompanionHealth{},
		Brokers:    []api.BrokerHealth{},
	}

	if b.db != nil {
		queued, capacity, dropped, lastDrop := b.db.WriterStats()
		info.Database = api.DatabaseHealth{
			WriteQueueLen: queued, WriteQueueCap: capacity, WritesDropped: dropped,
			WritesDroppedLastSecs: secsSinceTime(lastDrop, now),
		}
		// Deliberately not a problem: the count never resets, so flagging it would pin this endpoint
		// to "degraded" for the life of the process after one transient overflow. Problems carries
		// current state; a past loss is reported as an age for the operator to threshold.
	}

	// Reuses Companions() but keeps only the name and peer count: position, channel keys and the
	// pubkey are all things a monitor has no use for and a public endpoint should not publish.
	for _, c := range b.Companions() {
		info.Companions = append(info.Companions,
			api.CompanionHealth{Name: c.Name, PeerCount: c.PeerCount})
	}
	if b.repeater != nil {
		info.Repeater = &api.RepeaterHealth{Name: b.repeater.Name()}
	}

	if brokers, ok := b.MqttStatus(); ok {
		for _, br := range brokers {
			info.Brokers = append(info.Brokers, api.BrokerHealth{
				Name: br.Name, Enabled: br.Enabled, Connected: br.Connected,
				Failing: br.LastError != "", Published: br.Published, Dropped: br.Dropped,
			})
			// Only a broker that is meant to be up counts: a disabled one is not a fault.
			if br.Enabled && !br.Connected {
				info.Problems = append(info.Problems, "mqtt: broker "+br.Name+" is not connected")
			}
		}
	}

	if !info.Radio.Connected {
		info.Problems = append(info.Problems, "radio: modem not connected")
	}
	if len(b.companions) == 0 && b.repeater == nil {
		info.Problems = append(info.Problems, "no companion or repeater node is running")
	}

	return info
}

func (b *backend) radioHealth(now time.Time, act *radioActivity) api.RadioHealth {
	h := api.RadioHealth{
		LastRxSecs: secsSince(act.lastRx.Load(), now),
		LastTxSecs: secsSince(act.lastTx.Load(), now),
	}

	// LastReply is the liveness probe's own signal, and only some transports answer a status query
	// at all; where none can, the field stays null rather than claiming silence.
	if lr, ok := b.stats.(interface{ LastReply() time.Time }); ok {
		if t := lr.LastReply(); !t.IsZero() {
			secs := int64(now.Sub(t).Seconds())
			h.LastReplySecs = &secs
		}
	}

	// false: never poll the board from here. A monitor hits this endpoint on a schedule, and asking
	// the radio a question per scrape would put real traffic on the link to answer "are you well".
	stats, ok := b.radioStats(false)
	if !ok {
		return h // no modem: everything below would be a zero that reads as healthy
	}
	h.Connected = true
	h.Transport = stats.Transport
	h.TxQueueLen = stats.TxQueueLen
	h.TxSent = stats.TxSent
	h.TxFailed = stats.TxFailed
	h.TxDroppedBusy = stats.TxDroppedBusy
	h.TxDroppedQueue = stats.TxDroppedQueue
	h.InboundDroppedNew = stats.InboundDroppedNew
	h.HandlerSlow = stats.HandlerSlow
	h.CRCErrors = stats.CRCErrors
	h.RecvErrors = stats.RecvErrors
	h.DriverErrors = stats.DriverErrors
	h.HwErrors = stats.HwErrors
	h.NoiseFloor = stats.NoiseFloor
	h.BatteryMV = stats.BatteryMV
	h.MCUTempC = stats.MCUTempC
	return h
}
