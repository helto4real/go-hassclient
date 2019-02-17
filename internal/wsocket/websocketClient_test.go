package wsocket_test

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	h "github.com/helto4real/go-hassclient/internal/test"
	ws "github.com/helto4real/go-hassclient/internal/wsocket"
)

var (
	// h.IntegrationFlag   *bool = flag.Bool("h.IntegrationFlag", false, "run h.IntegrationFlag tests")
	port          int
	cancelFunc    func()
	clientServers map[wsClientServer]bool = map[wsClientServer]bool{}
)

func TestMain(m *testing.M) {
	flag.Parse()

	if *h.IntegrationFlag {
		port, cancelFunc = SetupWebSocketFakeServer()

	}

	os.Exit(m.Run())

	if *h.IntegrationFlag {
		cancelFunc()
	}
}

// TestIntegrationWebSocketReturnSameValue tests if the basic functionality
// works. It uses the mock server to do actual connection and the mock server
// will return same thing that are sent.
func TestIntegrationWebSocket(t *testing.T) {
	if !*h.IntegrationFlag {
		t.Skip()
		return
	}

	t.Run("TestSendString",
		func(*testing.T) {
			wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)
			defer wClient.Close()

			wClient.SendString("Hello world!")
			result, _ := wClient.Read()
			h.Equals(t, string(result), "Hello world!")
		})

	t.Run("TestSendStringMultiple",
		func(*testing.T) {
			wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)
			defer wClient.Close()
			wClient.SendString("Hello world!")
			wClient.SendString("Hello world again!")
			result, _ := wClient.Read()
			h.Equals(t, "Hello world!", string(result))
			result, _ = wClient.Read()
			h.Equals(t, "Hello world again!", string(result))
		})

	t.Run("TestSendMap",
		func(*testing.T) {
			wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)
			defer wClient.Close()
			wClient.SendMap(map[string]interface{}{
				"test":    "hello",
				"integer": 100})
			result, _ := wClient.Read()
			h.Equals(t, string(result), "{\"integer\":100,\"test\":\"hello\"}")
		})

	t.Run("TestNotConnected",
		func(*testing.T) {

			wClient := ws.ConnectWS("127.0.0.2:1111", "/ws", true)

			h.Equals(t, nil, wClient)
		})

	t.Run("TestClientGracefulDisconnect",
		func(*testing.T) {
			wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)
			go wClient.SendString("close")

			time.Sleep(time.Second * 1)
			_, ok := wClient.Read()
			h.Equals(t, false, ok)
			fmt.Println(wClient.IsClosed())
			h.Equals(t, true, wClient.IsClosed())
			wClient.Close()
		})

	t.Run("TestClientHardDisconnect",
		func(*testing.T) {
			wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)

			wClient.Close()

			_, ok := wClient.Read()

			h.Equals(t, false, ok)
		})
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 862144
)

//
func SetupWebSocketFakeServer() (int, func()) {

	listener, _ := net.Listen("tcp", ":4001")
	port := listener.Addr().(*net.TCPAddr).Port

	server := http.Server{}
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(w, r)
	})
	go func() {
		err := http.Serve(listener, nil)
		if err != nil {
			panic(err)
		}
	}()

	time.Sleep(time.Second * 2)
	return port, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		// wsConn.Close()

		err := server.Shutdown(ctx)
		if err != nil {
			cancel()
		}
		time.Sleep(time.Second * 1)

	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type wsClientServer struct {
	readBytesChannel  chan []byte
	writeBytesChannel chan []byte
	wsConn            *websocket.Conn
}

func (a *wsClientServer) ReadNextWSMessage() []byte {
	return <-a.readBytesChannel
}
func (a *wsClientServer) WriteWSMessage(message []byte) {
	a.writeBytesChannel <- message
}

func (a *wsClientServer) Start() {
	go func() {
		defer a.wsConn.Close()
		// Read pump
		a.wsConn.SetReadLimit(maxMessageSize)
		a.wsConn.SetReadDeadline(time.Now().Add(pongWait))
		a.wsConn.SetPongHandler(func(string) error { a.wsConn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

		for {
			_, message, err := a.wsConn.ReadMessage()
			if err != nil {
				return
			}
			// Just write back anything we get
			a.writeBytesChannel <- message
		}
	}()

	go func() {
		// Write pump
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		defer a.wsConn.Close()

		for {
			select {
			case message, ok := <-a.writeBytesChannel:
				a.wsConn.SetWriteDeadline(time.Now().Add(writeWait))
				if !ok {
					a.wsConn.WriteMessage(websocket.CloseMessage, []byte{})

					return
				}
				if string(message) == "close" {
					a.wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Peer closing down.."))
					time.Sleep(time.Second)

				} else {
					w, err := a.wsConn.NextWriter(websocket.TextMessage)
					if err != nil {
						return
					}
					w.Write(message)

					if err := w.Close(); err != nil {
						return
					}
				}

			case <-ticker.C:
				a.wsConn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := a.wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}

func serveWS(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}

	wsClient := wsClientServer{
		readBytesChannel:  make(chan []byte, 1),
		writeBytesChannel: make(chan []byte, 2),
		wsConn:            wsConn,
	}
	clientServers[wsClient] = true

	wsClient.Start()

}
