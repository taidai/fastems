package appapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket 实时推送：/ws 端点每 2 秒推送当前 snapshot（节点树），
// 前端可订阅它替代轮询。这是应用层 WebSocket 能力的落地。

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// serveWS 处理 WebSocket 连接：升级后周期性推送 snapshot，直到客户端断开。
func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request, identity Identity) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		nodes, err := h.visibleNodes(identity)
		if err == nil {
			if err := conn.WriteJSON(map[string]any{"nodes": nodes, "at": time.Now().UTC()}); err != nil {
				return // 客户端断开
			}
		}
		select {
		case <-ticker.C:
		case <-r.Context().Done():
			return
		}
	}
}
