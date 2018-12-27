package client

import (
	"bytes"
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 131072
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type websocketClient struct {
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	SendChannel chan []byte
	// Channel for receive data
	ReceiveChannel chan []byte
	// Set if websocket cant revocer and need to be reconnected
	Fatal bool
	// Used to wait for go routines end before close whole websocketClient
	syncRoutines sync.WaitGroup
}

// readPump ensures only one reader per connection.
func (c *websocketClient) readPump() {
	c.syncRoutines.Add(1)
	defer func() {
		c.syncRoutines.Done()
		c.Close(true)
		log.Printf("Close ws readpump")

	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Unexpected close error: %v", err)
			} else {
				log.Printf("Error reading websocket: %v", err)
			}

			return
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))

		c.ReceiveChannel <- message

	}
}

// Close the web socket client to free all resources and stop and wait for goroutines
func (c *websocketClient) Close(fatal bool) {
	c.Fatal = fatal
	if c.ReceiveChannel == nil || c.SendChannel == nil {
		return
	}
	// Close the connection and ignore errors
	c.conn.Close()

	// Close the channels
	close(c.ReceiveChannel)
	c.ReceiveChannel = nil
	close(c.SendChannel)
	c.SendChannel = nil

	//  Wait for the routines to stop
	c.syncRoutines.Wait()
	log.Printf("Closing websocket")
}

func (c *websocketClient) SendMap(message map[string]interface{}) {

	jsonString, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshal message: %s", err)
		return
	}

	c.SendChannel <- jsonString

}

func (c *websocketClient) SendString(message string) {

	c.SendChannel <- []byte(message)

}

// writePump pumps messages to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *websocketClient) writePump() {
	c.syncRoutines.Add(1)
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close(true)
		c.syncRoutines.Done()
	}()
	for {
		select {
		case message, ok := <-c.SendChannel:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			// Add queued messages to the current websocket message.
			n := len(c.SendChannel)
			for i := 0; i < n; i++ {
				w.Write(<-c.SendChannel)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ConnectWS connects to Web Socket
func ConnectWS(ip string, path string, ssl bool) *websocketClient {
	var scheme string = "ws"
	if ssl == true {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: ip, Path: path}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Print("dial:", err)
		return nil
	}

	client := &websocketClient{conn: c, SendChannel: make(chan []byte, 256), ReceiveChannel: make(chan []byte), Fatal: false}

	// Do write and read operations in own go routines
	go client.writePump()
	go client.readPump()

	return client
}
