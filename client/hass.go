package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"time"

	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

// HomeAssistant interface represents Home Assistant
type HomeAssistant interface {
	// Start daemon only use in main
	Start(host string, ssl bool, token string) bool
	// Stop daemon only use in main
	Stop()
	GetEntity(entity string) (*HassEntity, bool)
	SetEntity(entity *HassEntity) bool
	CallService(service string, serviceData map[string]string)
	GetHassChannel() chan interface{}
	GetStatusChannel() chan bool
	GetConfig() *HassConfig
}

// HomeAssistantPlatform implements integration with Home Assistant
type HomeAssistantPlatform struct {
	wsClient          *websocketClient
	wsID              int64
	getStateID        int64
	getConfigID       int64
	cancelHassLoop    context.CancelFunc
	context           context.Context
	token             string
	HassChannel       chan interface{}
	HassStatusChannel chan bool
	list              List
	host              string
	ssl               bool
	stopped           bool
	httpClient        *http.Client
	HassConfig        *HassConfig
}

// ServiceDataItem is used for a convenient way to provide service data in a variadic function CallService
type ServiceDataItem struct {
	Name  string
	Value string
}

// NewHassClient creates a new instance of the Home Assistant client
func NewHassClient() *HomeAssistantPlatform {
	context, cancelHassLoop := context.WithCancel(context.Background())
	return &HomeAssistantPlatform{
		wsID:              1,
		context:           context,
		cancelHassLoop:    cancelHassLoop,
		HassChannel:       make(chan interface{}, 10),
		HassStatusChannel: make(chan bool, 2),
		list:              NewEntityList(),
		stopped:           false,
		HassConfig:        &HassConfig{},
		httpClient:        &http.Client{}}
}

func (a *HomeAssistantPlatform) GetConfig() *HassConfig {
	return a.HassConfig
}

func (a *HomeAssistantPlatform) GetHassChannel() chan interface{} {
	return a.HassChannel
}

func (a *HomeAssistantPlatform) GetStatusChannel() chan bool {
	return a.HassStatusChannel
}

// Start the Home Assistant Client
func (a *HomeAssistantPlatform) Start(host string, ssl bool, token string) bool {
	a.token = token
	a.host = host
	a.ssl = ssl
	a.wsClient = a.connectWithReconnect()

	for {
		select {
		case <-a.context.Done():
			return false
		case message, mc := <-a.wsClient.ReceiveChannel:

			if !mc {
				if a.stopped {
					return true // Return if stopped
				}

				// Delay 5 seconds before reconnecting
				if a.delay(5) {
					return true
				}

				a.wsClient = a.connectWithReconnect()
				if a.wsClient == nil {
					log.Println("Ending service discovery")
					return false
				}
			}
			// s := string(message)
			// fmt.Println(s)
			var result Result
			err := json.Unmarshal(message, &result)
			if err != nil {
				log.Error(err)
			} else {
				go a.handleMessage(result)
			}

		}

	}

}
func (a *HomeAssistantPlatform) delay(seconds time.Duration) bool {

	select {
	case <-time.After(seconds * time.Second):
		return false
	case <-a.context.Done():
		return true
	}
}

// Stop the Home Assistant client
func (a *HomeAssistantPlatform) Stop() {
	a.stopped = true
	a.cancelHassLoop()
	a.wsClient.Close()
	close(a.HassChannel)
	close(a.HassStatusChannel)
}

func (a *HomeAssistantPlatform) connectWithReconnect() *websocketClient {

	var client *websocketClient

	for {
		if a.host == "hassio" {
			client = ConnectWS(a.host, "/homeassistant/websocket", false)

		} else {
			client = ConnectWS(a.host, "/api/websocket", a.ssl)
		}

		if client == nil {
			a.HassStatusChannel <- false
			log.Println("Fail to connect, reconnecting to Home Assistant in 30 seconds...")
			// Fail to connect wait to connect again for 5 seconds
			if a.delay(5) {
				return nil
			}

		} else {
			return client
		}
	}
}

// GetEntity returns the entity
func (a *HomeAssistantPlatform) GetEntity(entity string) (*HassEntity, bool) {
	return a.list.GetEntity(entity)
}

// SetEntity sets the entity to the map
func (a *HomeAssistantPlatform) SetEntity(entity *HassEntity) bool {
	var scheme = "http"
	if a.ssl == true {
		scheme = "https"
	}
	var u url.URL
	if a.host == "hassio" {
		u = url.URL{Scheme: scheme, Host: a.host, Path: "/homeassistant/api/states/" + url.PathEscape(entity.ID)}
	} else {
		u = url.URL{Scheme: scheme, Host: a.host, Path: "/api/states/" + url.PathEscape(entity.ID)}
	}

	stateData := SetStateData{State: entity.New.State, Attributes: entity.New.Attributes}
	b, err := json.Marshal(stateData)

	if err != nil {
		log.Errorf("Failed to marshal state data from entity %s, data : %v", entity, err)
		return false
	}

	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(b))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.token)

		resp, err := a.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 201 {
				return true
			}
		}
	}
	return false

}

//CallService makes a service call through the Home Assistant API
func (a *HomeAssistantPlatform) CallService(service string, serviceData map[string]string) {
	a.wsID = a.wsID + 1

	s := map[string]interface{}{
		"id":           a.wsID,
		"type":         "call_service",
		"domain":       "homeassistant",
		"service":      service,
		"service_data": serviceData}

	a.wsClient.SendMap(s)

}

// Send a generic message to Home Assistant websocket API
func (a *HomeAssistantPlatform) sendMessage(messageType string) {
	a.wsID = a.wsID + 1
	s := map[string]interface{}{
		"id":   a.wsID,
		"type": messageType}

	if messageType == "get_states" {
		a.getStateID = a.wsID
	} else if messageType == "get_config" {
		a.getConfigID = a.wsID
	}
	a.wsClient.SendMap(s)

}

func (a *HomeAssistantPlatform) subscribeEventsStateChanged() {
	a.wsID = a.wsID + 1
	s := map[string]interface{}{
		"id":   a.wsID,
		"type": "subscribe_events"} //"event_type": "state_changed"

	a.wsClient.SendMap(s)

}

// func (a *HomeAssistantPlatform) subscribeEventsCallService() {
// 	a.wsID = a.wsID + 1
// 	s := map[string]interface{}{
// 		"id":         a.wsID,
// 		"type":       "subscribe_events",
// 		"event_type": "call_service"}

// 	a.wsClient.SendMap(s)

// }

func (a *HomeAssistantPlatform) handleMessage(message Result) {

	if message.MessageType == "auth_required" {
		//	log.Print("message->: ", message)
		log.Debugln("Authorizing with Home Assistant...")

		a.wsClient.SendString("{\"type\": \"auth\",\"access_token\": \"" + a.token + "\"}")
	} else if message.MessageType == "auth_ok" {
		//		log.Print("message->: ", message)
		log.Debugln("Autorization ok...")
		a.sendMessage("get_config")

	} else if message.MessageType == "result" {

		if message.Id == a.getStateID {
			log.Debugf("Got all states, getting events [%v]", message.Id)
			results := message.Result.([]interface{})
			for _, data := range results {
				item := data.(map[string]interface{})
				lastUpdated, _ := time.Parse(time.RFC3339, item["last_updated"].(string))
				lastChanged, _ := time.Parse(time.RFC3339, item["last_changed"].(string))
				new := HassEntityState{
					LastUpdated: lastUpdated,
					LastChanged: lastChanged,
					State:       item["state"].(string),
					Attributes:  item["attributes"].(map[string]interface{})}

				old := HassEntityState{}
				newHassEntity := NewHassEntity(item["entity_id"].(string), item["entity_id"].(string), old, new)
				a.list.SetEntity(newHassEntity)
				a.HassChannel <- *newHassEntity

			}

			//			a.subscribeEventsCallService()
			a.subscribeEventsStateChanged()
			log.Info("Home Assistant integration ready!")
			a.HassStatusChannel <- true
		} else if message.Id == a.getConfigID {

			result := message.Result.(map[string]interface{})
			a.HassConfig.Latitude = result["latitude"].(float64)
			a.HassConfig.Longitude = result["longitude"].(float64)
			a.HassConfig.Elevation = result["elevation"].(float64)

			a.sendMessage("get_states")

		}
	} else if message.MessageType == "event" {
		if message.Event.EventType == "state_changed" {
			data := message.Event.Data
			log.Tracef("message->: %s=%s", data.EntityId, data.NewState.State)
			lastUpdated, _ := time.Parse(time.RFC3339, data.NewState.LastUpdated)
			lastChanged, _ := time.Parse(time.RFC3339, data.NewState.LastChanged)
			new := HassEntityState{
				LastUpdated: lastUpdated,
				LastChanged: lastChanged,
				State:       fmt.Sprint(data.NewState.State),
				Attributes:  data.NewState.Attributes}
			lastUpdated, _ = time.Parse(time.RFC3339, data.OldState.LastUpdated)
			lastChanged, _ = time.Parse(time.RFC3339, data.OldState.LastChanged)

			old := HassEntityState{
				LastUpdated: lastUpdated,
				LastChanged: lastChanged,
				State:       fmt.Sprint(data.OldState.State),
				Attributes:  data.OldState.Attributes}

			newHassEntity := NewHassEntity(data.EntityId, data.EntityId, old, new)
			a.list.SetEntity(newHassEntity)
			a.HassChannel <- *newHassEntity
		} else if message.Event.EventType == "call_service" {

			timeFired, _ := time.Parse(time.RFC3339, message.Event.TimeFired)
			newHassCallServiceEvent := NewHassCallServiceEvent(timeFired, message.Event.Data.Domain,
				message.Event.Data.Service, message.Event.Data.ServiceData)
			a.HassChannel <- *newHassCallServiceEvent

		} else {
			//log.Warning(message)
		}

	}

}

func init() {

	log = logrus.WithField("prefix", "hassclient")

}
