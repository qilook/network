package network

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WsConn struct {
	conn    *websocket.Conn
	session Session
	once    sync.Once
	done    chan struct{}
}

func (ws *WsConn) ServerIO() {
	ws.session.OnConnect(ws)
	go ws.readPump()
	ws.writePump()
}

func (ws *WsConn) Close() {
	ws.once.Do(func() { ws.session.OnDisConnect(); ws.done <- struct{}{}; ws.conn.Close() })
}

func (ws *WsConn) readPump() {
	defer func() { ws.Close() }()
	ws.conn.SetReadDeadline(time.Now().Add(pongWait))
	ws.conn.SetPongHandler(func(string) error { ws.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := ws.conn.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
			break
		}
		if err != nil {
			log.Printf("network read is err:%v", err)
			break
		}
		ws.session.OnMessage(message)

	}
}

func (ws *WsConn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() { ticker.Stop(); ws.Close() }()
	for {
		select {
		case <-ticker.C:
			ws.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("network write ping is err:%v", err)
				return
			}
		case <-ws.done:
			return
		}
	}
}

func (ws *WsConn) Write(b []byte) error {
	ws.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return ws.conn.WriteMessage(websocket.BinaryMessage, b)
}

func (ws *WsConn) LocalAddr() net.Addr {
	return ws.conn.UnderlyingConn().LocalAddr()
}

func (ws *WsConn) RemoteAddr() net.Addr {
	return ws.conn.UnderlyingConn().RemoteAddr()
}

func (ws *WsConn) GetSession() Session {
	return ws.session
}

func NewWsConn(conn *websocket.Conn, sessionCreator func() Session) *WsConn {
	wsConn := &WsConn{conn: conn, session: sessionCreator(), done: make(chan struct{})}
	return wsConn
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WsServer struct {
	sessionCreator func() Session
}

func (server *WsServer) Start(addr string) error {
	return http.ListenAndServe(addr, server)
}

func (server *WsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("network upgrade:%v", err)
		return
	}
	defer c.Close()
	wsConn := NewWsConn(c, server.sessionCreator)
	wsConn.ServerIO()
}

func NewWsServer(sessionCreator func() Session) Server {
	return &WsServer{sessionCreator: sessionCreator}
}
