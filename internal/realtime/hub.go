// Package realtime is the WebSocket fan-out hub. The server pushes small
// "changed" events; clients refetch via REST (which stays the source of truth
// and owns per-viewer redaction). One hub instance per process.
package realtime

import (
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Event names.
const (
	EventHello = "hello"
	// A message (from a person or from Chaos) landed in this conversation.
	EventMessagesChanged = "messages.changed" // payload: conversation_id
	// The conversation list itself moved — new conversation, rename, someone
	// joined, a decision resolved.
	EventConversationsChanged = "conversations.changed"
	// Chaos started composing. Purely cosmetic: the client shows "Chaos is
	// thinking…" without holding a request open.
	EventChaosThinking = "chaos.thinking"
)

type Event struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// Notifier is the port the domain service publishes through.
type Notifier interface {
	NotifyUsers(userIDs []uuid.UUID, event Event)
	IsConnected(userID uuid.UUID) bool
}

type client struct {
	userID uuid.UUID
	conn   *websocket.Conn
	send   chan Event
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*client]struct{}
	log     *zap.Logger
}

var _ Notifier = (*Hub)(nil)

func NewHub(log *zap.Logger) *Hub {
	return &Hub{clients: map[uuid.UUID]map[*client]struct{}{}, log: log}
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = map[*client]struct{}{}
	}
	h.clients[c.userID][c] = struct{}{}
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[c.userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
	close(c.send)
}

// NotifyUsers queues an event for every live connection of the given users.
// Slow consumers are skipped (they'll catch up via the fallback poll).
func (h *Hub) NotifyUsers(userIDs []uuid.UUID, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, id := range userIDs {
		for c := range h.clients[id] {
			select {
			case c.send <- event:
			default:
			}
		}
	}
}

// IsConnected reports whether the user has at least one live socket — used to
// decide who needs a push notification instead.
func (h *Hub) IsConnected(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// Connections reports how many sockets are live (ops/debug).
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, set := range h.clients {
		n += len(set)
	}
	return n
}

// NoopNotifier satisfies Notifier when the hub is disabled (tests).
type NoopNotifier struct{}

func (NoopNotifier) NotifyUsers([]uuid.UUID, Event) {}
func (NoopNotifier) IsConnected(uuid.UUID) bool     { return false }
