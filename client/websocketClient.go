package client

import (
	"bytes"
	"context"
	"encoding/json"
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
	maxMessageSize = 862144
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Client is a middleman between the websocket connection and the hub.
type websocketClient struct {
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	SendChannel chan []byte
	// Channel for receive data
	ReceiveChannel chan []byte
	// Used to wait for go routines end before close whole websocketClient
	syncRoutines sync.WaitGroup
	// Used to thread safe the close method
	m sync.Mutex

	context        context.Context
	cancelWSClient context.CancelFunc
	isClosed       bool
}

// readPump ensures only one reader per connection.
func (c *websocketClient) readPump() {
	c.syncRoutines.Add(1)
	defer func() {
		c.syncRoutines.Done()
		c.Close()
		log.Traceln("Close ws readpump")

	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()

		select {
		case <-c.context.Done():
			return
		default:
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Errorf("Unexpected close error: %v", err)
				} else {
					log.Errorf("Error reading websocket: %v", err)
				}

				return
			}
			message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
			log.Tracef("<-msg\n%s", string(message))
			c.ReceiveChannel <- message
		}

	}
}

// Close the web socket client to free all resources and stop and wait for goroutines
func (c *websocketClient) Close() {
	// Make sure we make close method thread safe
	c.m.Lock()
	defer c.m.Unlock()

	if c.isClosed == true {
		// Make sure subsequent calls to close just returns
		return
	}
	c.isClosed = true
	c.cancelWSClient()
	//  Wait for the routines to stop
	c.syncRoutines.Wait()

	close(c.ReceiveChannel)
	close(c.SendChannel)
	c.conn.Close()

	log.Tracef("Closing websocket")

}

func (c *websocketClient) SendMap(message map[string]interface{}) {

	jsonString, err := json.Marshal(message)
	if err != nil {
		log.Errorf("Error marshal message: %s", err)
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
		c.syncRoutines.Done()
		c.Close()
		log.Traceln("Close ws writepump")
	}()
	for {
		select {
		case <-c.context.Done():
			return
		case message, ok := <-c.SendChannel:
			if c.isClosed {
				return
			}
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
			log.Tracef("msg->%s", string(message))
			// Add queued messages to the current websocket message.
			n := len(c.SendChannel)
			for i := 0; i < n; i++ {
				_, err = w.Write(<-c.SendChannel)
				if err != nil {
					return
				}
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
	var scheme = "ws"
	if ssl == true {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: ip, Path: path}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Error("dial:", err)
		return nil
	}

	context, cancelWSClient := context.WithCancel(context.Background())

	client := &websocketClient{conn: c, SendChannel: make(chan []byte, 256), ReceiveChannel: make(chan []byte, 2),
		isClosed: false, context: context, cancelWSClient: cancelWSClient}

	// Do write and read operations in own go routines
	go client.writePump()
	go client.readPump()

	return client
}
