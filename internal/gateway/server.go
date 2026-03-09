package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/agent"

	"github.com/gorilla/websocket"
)

type Server struct {
	host    string
	port    int
	agent   *agent.Agent
	httpSrv *http.Server
}

func NewServer(host string, port int, model string) *Server {
	s := &Server{
		host:  host,
		port:  port,
		agent: agent.New(model),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/ws", s.ws)
	s.httpSrv = &http.Server{
		Addr:              host + ":" + itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	log.Printf("adkbot gateway listening on ws://%s:%d/ws", s.host, s.port)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			_ = conn.WriteJSON(Response{Type: "error", Error: err.Error()})
			continue
		}

		switch msg.Type {
		case "chat":
			reply, err := s.agent.Run(r.Context(), msg.Content)
			if err != nil {
				_ = conn.WriteJSON(Response{Type: "error", Error: err.Error()})
				continue
			}
			_ = conn.WriteJSON(Response{Type: "chat", Data: map[string]string{"reply": reply}})
		case "tool":
			var args map[string]any
			if len(msg.Args) > 0 {
				_ = json.Unmarshal(msg.Args, &args)
			}
			res, err := s.agent.RunTool(r.Context(), msg.Name, args)
			if err != nil {
				_ = conn.WriteJSON(Response{Type: "error", Error: err.Error()})
				continue
			}
			_ = conn.WriteJSON(Response{Type: "tool", Data: res})
		case "tools.list":
			_ = conn.WriteJSON(Response{Type: "tools.list", Data: s.agent.ToolNames()})
		default:
			_ = conn.WriteJSON(Response{Type: "error", Error: "unsupported message type"})
		}
	}
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
