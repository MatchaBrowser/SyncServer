package events

import (
	"testing"
	"time"
)

func received(s *Subscription) (int64, bool) {
	select {
	case v := <-s.Notifications():
		return v, true
	case <-time.After(10 * time.Millisecond):
		return 0, false
	}
}

func isClosed(s *Subscription) bool {
	select {
	case <-s.Closed():
		return true
	default:
		return false
	}
}

func TestPublish(t *testing.T) {
	h := NewHub()
	mac := h.Subscribe("account", "mac")
	mini := h.Subscribe("account", "mini")
	stranger := h.Subscribe("other account", "studio")

	h.Publish("account", "mac", 10)
	if _, ok := received(mac); ok {
		t.Error("the device that pushed must not be notified")
	}
	if v, ok := received(mini); !ok || v != 10 {
		t.Errorf("other device: %d, %v", v, ok)
	}
	if _, ok := received(stranger); ok {
		t.Error("another account must not be notified")
	}

	// Several pushes before a read: the latest version only.
	h.Publish("account", "mac", 11)
	h.Publish("account", "mac", 13)
	h.Publish("account", "mac", 12)
	if v, ok := received(mini); !ok || v != 13 {
		t.Errorf("coalesced notification: %d, %v; want 13", v, ok)
	}
	if _, ok := received(mini); ok {
		t.Error("only one notification must be pending")
	}
}

func TestClose(t *testing.T) {
	h := NewHub()
	mac := h.Subscribe("account", "mac")
	mini := h.Subscribe("account", "mini")
	studio := h.Subscribe("account", "studio")
	other := h.Subscribe("other account", "mac")

	h.CloseDevice("account", "mini")
	if !isClosed(mini) || isClosed(mac) || isClosed(studio) {
		t.Error("CloseDevice must close the streams of that device only")
	}
	h.CloseOthers("account", "mac")
	if !isClosed(studio) || isClosed(mac) {
		t.Error("CloseOthers must keep the stream of the current device")
	}
	h.CloseAccount("account")
	if !isClosed(mac) || isClosed(other) {
		t.Error("CloseAccount must close the streams of that account only")
	}
	if h.Len() != 1 {
		t.Errorf("%d streams open, want 1", h.Len())
	}

	// A closed stream gets nothing, and closing again is harmless.
	h.Publish("account", "x", 5)
	h.Unsubscribe(mac)
	h.CloseAll()
	if !isClosed(other) || h.Len() != 0 {
		t.Error("CloseAll must close every stream")
	}
}
