package client_test

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	c "github.com/helto4real/go-hassclient/client"
	h "github.com/helto4real/go-hassclient/internal/test"
)

// var h.IntegrationFlag *bool = flag.Bool("h.IntegrationFlag", false, "run h.IntegrationFlag tests")

func TestIntegrationWebSocket(t *testing.T) {
	fake := newFakeConnected()
	fakePoster := newFakePoster()
	hass := c.NewHassClientFakeConnection(fake, fakePoster)

	go func() {
		// Just to clear all stuff
		for {
			message, ok := <-hass.GetHassChannel()
			if !ok {
				return
			}
			switch v := message.(type) {
			case c.HassCallServiceEvent:
				atomic.AddInt64(&fake.nrOfEvents, 1)
			case c.HassEntity:
				atomic.AddInt64(&fake.nrOfEntities, 1)
			default:
				fmt.Printf("I don't know about type %T!\n", v)
			}
		}
	}()
	go hass.Start("ws://fake", false, "anytoken")
	defer hass.Stop()

	state := <-hass.GetStatusChannel()
	h.Equals(t, true, state)

	t.Run("GetEntity",
		func(*testing.T) {
			entity, ok := hass.GetEntity("group.all_lights")
			h.Equals(t, true, ok)
			h.Equals(t, "group.all_lights", entity.ID)
			h.Equals(t, "group.all_lights", entity.Name)
			h.Equals(t, "on", entity.New.State)
			h.Equals(t, "", entity.Old.State)
		})

	t.Run("GetEntityNoExists",
		func(*testing.T) {
			entity, ok := hass.GetEntity("group.not_exists")
			h.Equals(t, false, ok)
			h.Equals(t, "", entity.ID)
		})

	t.Run("SetEntity",
		func(*testing.T) {
			ok := hass.SetEntity(&c.HassEntity{})
			h.Equals(t, true, ok)
			h.Equals(t, 1, fakePoster.nrOfPostCalls)
			h.Equals(t, false, strings.Contains(fakePoster.url,
				"/homeassistant/api/states/"))
			h.Equals(t, true, strings.Contains(fakePoster.url,
				"/api/states/"))
			h.NotEquals(t, true, strings.Contains(fakePoster.url,
				"https"))
		})

	t.Run("Test Events",
		func(*testing.T) {
			entity, ok := hass.GetEntity("binary_sensor.vardagsrum_pir")
			h.Equals(t, true, ok)
			h.Equals(t, "binary_sensor.vardagsrum_pir", entity.ID)
			h.Equals(t, "on", entity.New.State)
			nrOfEntities := atomic.LoadInt64(&fake.nrOfEntities)
			nrOfEvents := atomic.LoadInt64(&fake.nrOfEvents)
			h.Equals(t, int64(19), nrOfEntities)
			h.Equals(t, int64(0), nrOfEvents)
			// Simulate a new event turning the state to off (event.json)
			go fake.SimulateNewEntityEvent()
			go fake.SimulateCallServiceEvent()
			time.Sleep(time.Second)
			entity, ok = hass.GetEntity("binary_sensor.vardagsrum_pir")
			nrOfEntities = atomic.LoadInt64(&fake.nrOfEntities)
			nrOfEvents = atomic.LoadInt64(&fake.nrOfEvents)
			h.Equals(t, "off", entity.New.State)
			h.Equals(t, int64(20), nrOfEntities)
			h.Equals(t, int64(1), nrOfEvents)
			h.Equals(t, true, ok)
		})

	t.Run("CallService",
		func(*testing.T) {
			hass.CallService("light/turn_on", map[string]string{"state": "on"})
			time.Sleep(time.Second)
			nrOfCallService := atomic.LoadInt64(&fake.nrOfCallService)
			h.Equals(t, int64(1), nrOfCallService)
		})
}

func TestHassEntityString(t *testing.T) {
	entity := c.NewHassEntity("id1", "name1", c.HassEntityState{
		State: "Old state"}, c.HassEntityState{
		State: "New state",
		Attributes: map[string]interface{}{
			"attr1": "attrvalue",
		}})

	str := entity.String()

	h.Equals(t, true, strings.Contains(str, "name1"))
	h.Equals(t, true, strings.Contains(str, "New state"))
	h.Equals(t, true, strings.Contains(str, "attr1"))
	h.Equals(t, true, strings.Contains(str, "attrvalue"))
}

func TestIntegrationWebSocketSSL(t *testing.T) {
	fake := newFakeConnected()
	fakePoster := newFakePoster()
	hass := c.NewHassClientFakeConnection(fake, fakePoster)

	go func() {
		// Just to clear all stuff
		for {
			if _, ok := <-hass.GetHassChannel(); !ok {
				return
			}
		}
	}()
	go hass.Start("fake", true, "anytoken")
	defer hass.Stop()

	state := <-hass.GetStatusChannel()
	h.Equals(t, true, state)

	t.Run("SetEntity",
		func(*testing.T) {
			ok := hass.SetEntity(&c.HassEntity{})
			h.Equals(t, true, ok)
			h.Equals(t, 1, fakePoster.nrOfPostCalls)
			h.Equals(t, false, strings.Contains(fakePoster.url,
				"/homeassistant/api/states/"))
			h.Equals(t, true, strings.Contains(fakePoster.url,
				"/api/states/"))
			h.Equals(t, true, strings.Contains(fakePoster.url,
				"https"))
		})
}
func TestIntegrationWebSocketHassio(t *testing.T) {
	fake := newFakeConnected()
	fakePoster := newFakePoster()
	hass := c.NewHassClientFakeConnection(fake, fakePoster)
	go func() {
		// Just to clear all stuff
		for {
			_, ok := <-hass.GetHassChannel()
			if !ok {
				return
			}
		}
	}()
	go hass.Start("hassio", false, "anytoken")
	defer hass.Stop()

	state := <-hass.GetStatusChannel()
	h.Equals(t, true, state)

	t.Run("SetEntity",
		func(*testing.T) {
			ok := hass.SetEntity(&c.HassEntity{})
			h.Equals(t, true, ok)
			h.Equals(t, 1, fakePoster.nrOfPostCalls)
			h.Equals(t, true, strings.Contains(fakePoster.url,
				"/homeassistant/api/states/"))
		})
}

func newFakeConnected() *fakeConnected {

	fake := fakeConnected{
		start:         true,
		isClosed:      false,
		stringChannel: make(chan string),
		mapChannel:    make(chan map[string]interface{}),
		eventChannel:  make(chan []byte, 2),
	}
	return &fake
}

type fakeConnected struct {
	isClosed        bool
	start           bool
	stringChannel   chan string
	mapChannel      chan map[string]interface{}
	eventChannel    chan []byte
	nrOfEntities    int64
	nrOfEvents      int64
	nrOfCallService int64
}

func (a *fakeConnected) Close() {

}
func (a *fakeConnected) SendMap(message map[string]interface{}) {
	a.mapChannel <- message
}
func (a *fakeConnected) SendString(message string) {
	a.stringChannel <- message
}
func (a *fakeConnected) Read() ([]byte, bool) {

	if a.start {
		// Fake just connected
		resp, _ := ioutil.ReadFile("testdata/auth_required.json")
		a.start = false
		return resp, true
	}
	select {
	case sendString, ok := <-a.stringChannel:
		if !ok {
			return nil, false
		}
		if strings.Contains(sendString, "auth") {
			resp, _ := ioutil.ReadFile("testdata/auth_ok.json")
			return resp, true
		}

	case sendMap, ok := <-a.mapChannel:
		if !ok {
			return nil, false
		}
		msgType, _ := sendMap["type"].(string)
		if msgType == "get_config" {
			resp, _ := ioutil.ReadFile("testdata/result_config.json")
			id := strconv.FormatInt(sendMap["id"].(int64), 10)
			return replaceId(resp, "123456789", id), true
		} else if msgType == "get_states" {
			resp, _ := ioutil.ReadFile("testdata/result_states.json")
			id := strconv.FormatInt(sendMap["id"].(int64), 10)
			return replaceId(resp, "123456789", id), true
		} else if msgType == "subscribe_events" {
			resp, _ := ioutil.ReadFile("testdata/result_msg.json")
			id := strconv.FormatInt(sendMap["id"].(int64), 10)
			return replaceId(resp, "123456789", id), true
		} else if msgType == "call_service" {
			resp, _ := ioutil.ReadFile("testdata/result_msg.json")
			id := strconv.FormatInt(sendMap["id"].(int64), 10)
			atomic.AddInt64(&a.nrOfCallService, 1)
			return replaceId(resp, "123456789", id), true
		}

	case event, ok := <-a.eventChannel:
		if !ok {
			return nil, false
		}
		return event, true
	}

	panic("Unknown input")
}
func (a *fakeConnected) IsClosed() bool {
	return a.isClosed
}

func (a *fakeConnected) SimulateNewEntityEvent() {
	esp, _ := ioutil.ReadFile("testdata/event.json")
	a.eventChannel <- esp
}
func (a *fakeConnected) SimulateCallServiceEvent() {
	esp, _ := ioutil.ReadFile("testdata/service_event.json")
	a.eventChannel <- esp
}
func replaceId(response []byte, old string, new string) []byte {
	res := string(response)
	replaced := strings.Replace(res, old, new, -1)
	return []byte(replaced)
}

func newFakePoster() *fakeHassAPIPoster {
	return &fakeHassAPIPoster{}
}

type fakeHassAPIPoster struct {
	nrOfPostCalls int
	url           string
}

func (a *fakeHassAPIPoster) HassHTTPPostAPI(url string, data []byte) bool {
	a.nrOfPostCalls = a.nrOfPostCalls + 1
	a.url = url
	return true
}
