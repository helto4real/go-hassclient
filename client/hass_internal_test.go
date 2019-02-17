package client

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	h "github.com/helto4real/go-hassclient/internal/test"
	ws "github.com/helto4real/go-hassclient/internal/wsocket"
)

var (
	port           int
	cancelFunc     func()
	connectSuccess atomic.Value
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Set default to return connect success
	connectSuccess.Store(true)

	if *h.IntegrationFlag {
		port, cancelFunc = SetupFakeWebServer()

	}

	os.Exit(m.Run())

	if *h.IntegrationFlag {
		cancelFunc()
	}
}

func TestConnectReconnect(t *testing.T) {
	hass := newHassClient()

	oldGetConnected := getConnected
	getConnected = fakeConnectWS
	defer func() { getConnected = oldGetConnected }()

	t.Run("NormalConnect",
		func(*testing.T) {
			c := hass.connectWithReconnect()
			h.NotEquals(t, nil, c)
		})

	if *h.IntegrationFlag {
		// Only run the reconnect tests with integration flag on
		t.Run("NoConnect",
			func(*testing.T) {
				connectSuccess.Store(false)
				defer func() { connectSuccess.Store(true) }()

				go func() {
					time.Sleep(time.Second * 1)
					connectSuccess.Store(true)
				}()
				c := hass.connectWithReconnect()
				h.NotEquals(t, nil, c)
			})
	}
}

func TestDisconnectAndReconnect(t *testing.T) {
	if !*h.IntegrationFlag {
		t.Skip()
		return
	}
	fake := newFakeConnected()
	fakePoster := newFakePoster()
	hass := newHassClient()
	// hass.wsClient = fake
	hass.poster = fakePoster

	oldGetConnected := getConnected
	getConnected = fakeConnectWS
	defer func() { getConnected = oldGetConnected }()

	go hass.Start("host", false, "token")
	defer hass.Stop()

	go func() { fake.doNothingChannel <- true }()

	time.Sleep(time.Second * 1)

}

func TestIntegrations(t *testing.T) {
	if !*h.IntegrationFlag {
		t.Skip()
		return
	}
	hass := newHassClient()

	t.Run("HttpPost",
		func(*testing.T) {
			b := hass.HassHTTPPostAPI("http://127.0.0.1:4002", []byte("Hello world"))
			h.Equals(t, true, b)
		})

	t.Run("HttpPostFail",
		func(*testing.T) {
			b := hass.HassHTTPPostAPI("http://127.0.0.1:4003", []byte("Hello world"))
			h.NotEquals(t, true, b)
		})
}

func SetupFakeWebServer() (int, func()) {

	listener, _ := net.Listen("tcp", ":4002")
	port := listener.Addr().(*net.TCPAddr).Port

	server := http.Server{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

func serveWS(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "POST done")
}

func fakeConnectWS(host, path string, ssl bool) ws.Connected {
	connSuccess := connectSuccess.Load().(bool)
	if connSuccess {
		return newFakeConnected()
	}
	return nil
}

func newFakeConnected() *fakeConnected {

	return &fakeConnected{doNothingChannel: make(chan bool, 1)}

}

type fakeConnected struct {
	doNothingChannel chan bool
}

func (a *fakeConnected) Close() {
	close(a.doNothingChannel)
}
func (a *fakeConnected) SendMap(message map[string]interface{}) {
	panic("Not implemented")
}
func (a *fakeConnected) SendString(message string) {
	panic("Not implemented")
}
func (a *fakeConnected) Read() ([]byte, bool) {
	<-a.doNothingChannel
	return nil, false
}
func (a *fakeConnected) IsClosed() bool {
	panic("Not implemented")
}

func newFakePoster() *fakeHassAPIPoster {
	return &fakeHassAPIPoster{}
}

type fakeHassAPIPoster struct {
}

func (a *fakeHassAPIPoster) HassHTTPPostAPI(url string, data []byte) bool {
	panic("Not implemented")
}
