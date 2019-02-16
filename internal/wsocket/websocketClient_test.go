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
	integration       *bool = flag.Bool("integration", true, "run integration tests")
	port              int
	cancelFunc        func()
	wsConn            *websocket.Conn
	readBytesChannel  chan []byte = make(chan []byte, 1)
	writeBytesChannel chan []byte = make(chan []byte, 2)
)

func TestMain(m *testing.M) {
	flag.Parse()

	if *integration {
		port, cancelFunc = SetupWebSocketFakeServer()

	}

	os.Exit(m.Run())

	if *integration {
		cancelFunc()
	}
}

// TestIntegrationWebSocketReturnSameValue tests if the basic functionality
// works. It uses the mock server to do actual connection and the mock server
// will return same thing that are sent.
func TestIntegrationWebSocketReturnSameValue(t *testing.T) {
	if !*integration {
		t.Skip()
		return
	}
	wClient := ws.ConnectWS(fmt.Sprintf("127.0.0.1:%v", port), "/ws", false)

	t.Run("TestSendString",
		func(*testing.T) {
			wClient.SendString("Hello world!")
			result := string(<-wClient.ReceiveChannel)
			h.Equals(t, string(result), "Hello world!")
		})

	t.Run("TestSendMap",
		func(*testing.T) {
			wClient.SendMap(map[string]interface{}{
				"test":    "hello",
				"integer": 100})
			result := string(<-wClient.ReceiveChannel)
			h.Equals(t, result, "{\"integer\":100,\"test\":\"hello\"}")
		})

	wClient.Close()
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

func ReadNextWSMessage() []byte {
	return <-readBytesChannel
}
func WriteWSMessage(message []byte) {
	writeBytesChannel <- message
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func serveWS(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	wsConn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		panic(err)
	}

	go func() {
		// Read pump
		wsConn.SetReadLimit(maxMessageSize)
		wsConn.SetReadDeadline(time.Now().Add(pongWait))
		wsConn.SetPongHandler(func(string) error { wsConn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				break
			}
			// Just write back anything we get
			writeBytesChannel <- message
		}
	}()

	go func() {
		// Write pump
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case message, ok := <-writeBytesChannel:
				wsConn.SetWriteDeadline(time.Now().Add(writeWait))
				if !ok {
					wsConn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}

				w, err := wsConn.NextWriter(websocket.TextMessage)
				if err != nil {
					return
				}
				w.Write(message)

				if err := w.Close(); err != nil {
					return
				}
			case <-ticker.C:
				wsConn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}
