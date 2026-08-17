package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/auth/jwt"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 70 * time.Second
	pingPeriod = 30 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Mobile apps have no Origin; a future web client should tighten this.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Heartbeater lets the read pump bump presence without importing the domain.
type Heartbeater interface {
	HeartbeatUser(ctx context.Context, userID uuid.UUID) error
}

// Handler upgrades GET /ws?token=<access-jwt> and pumps events.
func Handler(hub *Hub, tokens jwt.Manager, hearts Heartbeater, log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := tokens.ParseAccess(c.Query("token"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false, "message": "Invalid or expired token",
				"error": "UNAUTHORIZED", "data": nil,
			})
			return
		}
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return // upgrader already wrote the error
		}

		cl := &client{userID: claims.UserID, conn: conn, send: make(chan Event, 32)}
		hub.register(cl)
		_ = hearts.HeartbeatUser(c.Request.Context(), claims.UserID)
		log.Info("ws connected", zap.String("user_id", claims.UserID.String()))

		// Queue hello BEFORE the pumps start: the channel is buffered and
		// nothing can close it yet, so this can never race with unregister.
		cl.send <- Event{Type: EventHello}

		go writePump(cl)
		go readPump(hub, cl, hearts, log)
	}
}

func writePump(c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case event, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteJSON(event); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func readPump(hub *Hub, c *client, hearts Heartbeater, log *zap.Logger) {
	defer func() {
		hub.unregister(c)
		_ = c.conn.Close()
		log.Info("ws disconnected", zap.String("user_id", c.userID.String()))
	}()
	c.conn.SetReadLimit(1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &frame) == nil && frame.Type == "presence.ping" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = hearts.HeartbeatUser(ctx, c.userID)
			cancel()
		}
	}
}
