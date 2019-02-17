package wsocket

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

type Connected interface {
	Close()
	SendMap(message map[string]interface{})
	SendString(message string)
	Read() ([]byte, bool)
	IsClosed() bool
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

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Client is a middleman between the websocket connection and the hub.
type websocketClient struct {
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	sendChannel chan []byte
	// Channel for receive meessages
	receiveChannel chan []byte
	// Used to wait for go routines end before close whole websocketClient
	syncWriter sync.WaitGroup
	syncReader sync.WaitGroup
	// Used to thread safe the close method
	m sync.Mutex

	context    context.Context
	cancelFunc context.CancelFunc

	isClosed bool
}

// readPump ensures only one reader per connection.
func (c *websocketClient) readPump() {

	defer func() {
		c.syncReader.Done()
		c.Close()
		log.Traceln("Close ws readpump")

	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Errorf("Normal disconnect from host: %v", err)
			} else {
				log.Errorf("Unexpected websocket error: %v", err)
			}
			return
		}
		if c.isClosed {
			return
		}

		if messageType == websocket.TextMessage {
			ioutil.WriteFile("test.json", message, 0)
			c.receiveChannel <- message
		}
	}
}

// Close the web socket client to free all resources and stop and wait for goroutines
func (c *websocketClient) Close() {
	// Make sure we make close method thread safe
	c.m.Lock()

	if c.isClosed == true {
		// Make sure subsequent calls to close just returns
		c.m.Unlock()
		return
	}
	c.isClosed = true
	//  Wait for the routines to stop
	c.m.Unlock()

	// The sender
	close(c.sendChannel)
	c.syncWriter.Wait()

	c.cancelFunc()

	close(c.receiveChannel)

	c.conn.Close()
	c.syncReader.Wait()

	log.Errorf("Closing websocket")

}

// IsClosed returns true if the connection closed
func (c *websocketClient) IsClosed() bool {
	c.m.Lock()
	defer c.m.Unlock()
	return c.isClosed
}

func (c *websocketClient) SendMap(message map[string]interface{}) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.isClosed {
		return
	}
	jsonString, err := json.Marshal(message)
	if err != nil {
		return
	}

	c.sendChannel <- jsonString

}

func (c *websocketClient) SendString(message string) {
	c.m.Lock()
	defer c.m.Unlock()

	if c.isClosed {
		return
	}

	c.sendChannel <- []byte(message)

}

// Read the next message
func (c *websocketClient) Read() ([]byte, bool) {
	select {
	case message, ok := <-c.receiveChannel:
		if ok {
			return message, true
		}
	case <-c.context.Done():
		return nil, false
	}
	return nil, false
}

// writePump pumps messages to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *websocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.syncWriter.Done()
		c.Close()
		log.Traceln("Close ws writepump")
	}()
	for {
		select {
		case message, ok := <-c.sendChannel:
			if c.isClosed {
				return
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}

			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// var w io.WriteCloser
			if w, err := c.conn.NextWriter(websocket.TextMessage); err != nil {
				return
			} else {
				if _, err := w.Write(message); err != nil {
					return
				}

				log.Tracef("msg->%s", string(message))

				if err := w.Close(); err != nil {
					return
				}
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
func ConnectWS(ip string, path string, ssl bool) Connected {
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

	ctx, cancel := context.WithCancel(context.Background())

	client := &websocketClient{conn: c, sendChannel: make(chan []byte, 256), receiveChannel: make(chan []byte, 2),
		isClosed: false, context: ctx, cancelFunc: cancel}

	// Do write and read operations in own go routines
	client.syncWriter.Add(1)
	go client.writePump()
	client.syncReader.Add(1)
	go client.readPump()

	return client
}

func init() {

	log = logrus.WithField("prefix", "hassclient")

}
