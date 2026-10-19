// Package events is the hub of the event streams (SYNC.md § 4.6): it tells
// the devices of an account that another device applied changes, and closes
// their streams when they are revoked (§ 4.9). It is in memory: the server is
// a single instance.
package events

import (
	"sync"
	"time"
)

// DefaultPingInterval is the period of the ": ping" comments (SYNC.md § 4.6).
const DefaultPingInterval = 30 * time.Second

// Subscription is the event stream of one device.
type Subscription struct {
	AccountID string
	DeviceID  string
	// notify carries the last version applied by another device. It holds
	// one value: a slow reader gets the latest version, not a queue.
	notify    chan int64
	closed    chan struct{}
	closeOnce sync.Once
}

// Notifications receives the versions to announce.
func (s *Subscription) Notifications() <-chan int64 {
	return s.notify
}

// Closed is closed when the hub ends the stream: device revoked, account
// deleted, server stopping.
func (s *Subscription) Closed() <-chan struct{} {
	return s.closed
}

func (s *Subscription) close() {
	s.closeOnce.Do(func() { close(s.closed) })
}

type Hub struct {
	// PingInterval is the period of the ": ping" comments.
	PingInterval time.Duration

	mu       sync.Mutex
	accounts map[string]map[*Subscription]struct{}
}

func NewHub() *Hub {
	return &Hub{PingInterval: DefaultPingInterval, accounts: make(map[string]map[*Subscription]struct{})}
}

// Subscribe opens the stream of a device.
func (h *Hub) Subscribe(accountID, deviceID string) *Subscription {
	s := &Subscription{AccountID: accountID, DeviceID: deviceID, notify: make(chan int64, 1), closed: make(chan struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.accounts[accountID]
	if subs == nil {
		subs = make(map[*Subscription]struct{})
		h.accounts[accountID] = subs
	}
	subs[s] = struct{}{}
	return s
}

// Unsubscribe forgets a stream that ended on its own (client gone, token
// expired).
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(s)
}

func (h *Hub) removeLocked(s *Subscription) {
	subs := h.accounts[s.AccountID]
	delete(subs, s)
	if len(subs) == 0 {
		delete(h.accounts, s.AccountID)
	}
	s.close()
}

// Publish tells the streams of the account, except those of the device that
// applied the changes, that version is available.
func (h *Hub) Publish(accountID, fromDeviceID string, version int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.accounts[accountID] {
		if s.DeviceID == fromDeviceID {
			continue
		}
		select {
		case s.notify <- version:
		default:
			// A version not read yet: keep the larger one.
			select {
			case pending := <-s.notify:
				version = max(version, pending)
			default:
			}
			s.notify <- version
		}
	}
}

// CloseDevice ends the streams of a device: revoked, logged out, or
// reconnected with new tokens.
func (h *Hub) CloseDevice(accountID, deviceID string) {
	h.closeWhere(accountID, func(s *Subscription) bool { return s.DeviceID == deviceID })
}

// CloseOthers ends the streams of every device of the account but keepID:
// change of password, recovery.
func (h *Hub) CloseOthers(accountID, keepID string) {
	h.closeWhere(accountID, func(s *Subscription) bool { return s.DeviceID != keepID })
}

// CloseAccount ends every stream of the account: account deleted.
func (h *Hub) CloseAccount(accountID string) {
	h.closeWhere(accountID, func(*Subscription) bool { return true })
}

// CloseAll ends every stream: the server stops.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, subs := range h.accounts {
		for s := range subs {
			s.close()
		}
	}
	h.accounts = make(map[string]map[*Subscription]struct{})
}

func (h *Hub) closeWhere(accountID string, match func(*Subscription) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.accounts[accountID] {
		if match(s) {
			h.removeLocked(s)
		}
	}
}

// Len is the number of open streams.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, subs := range h.accounts {
		n += len(subs)
	}
	return n
}
