package client

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// DaemonApplication represents an application
type HomeAssistant interface {
	// Start daemon only use in main
	Start(host string, ssl bool, token string) bool
	// Stop daemon only use in main
	Stop()
	GetEntity(entity string) (HassEntity, bool)
	CallService(service string, serviceData map[string]string)
	GetEntityChannel() chan *HassEntity
}

// HomeAssistantPlatform implements integration with Home Assistant
type HomeAssistantPlatform struct {
	wsClient          *websocketClient
	wsID              int64
	getStateID        int64
	cancelDiscovery   context.CancelFunc
	context           context.Context
	token             string
	HassEntityChannel chan *HassEntity
	list              List
}

// ServiceDataItem is used for a convenient way to provide service data in a variadic function CallService
type ServiceDataItem struct {
	Name  string
	Value string
}

// NewHassClient creates a new instance of the Home Assistant client
func NewHassClient() *HomeAssistantPlatform {
	context, cancelDiscovery := context.WithCancel(context.Background())
	return &HomeAssistantPlatform{
		wsID:              1,
		context:           context,
		cancelDiscovery:   cancelDiscovery,
		HassEntityChannel: make(chan *HassEntity, 1),
		list:              NewEntityList()}
}

func (a *HomeAssistantPlatform) GetEntityChannel() chan *HassEntity {
	return a.HassEntityChannel
}

// Start the Home Assistant Client
func (a *HomeAssistantPlatform) Start(host string, ssl bool, token string) bool {
	a.token = token
	a.wsClient = a.connectWithReconnect(host, ssl)

	for {
		select {
		case message, mc := <-a.wsClient.ReceiveChannel:
			if !mc {
				if a.wsClient.Fatal {
					a.wsClient = a.connectWithReconnect(host, ssl)
					if a.wsClient == nil {
						log.Println("Ending service discovery")
						return false
					}
				} else {
					return false
				}

			}
			var result Result
			json.Unmarshal(message, &result)
			go a.handleMessage(result)
		case <-a.context.Done():
			return false
		}

	}

}

// Stop the Home Assistant client
func (a *HomeAssistantPlatform) Stop() {

	a.cancelDiscovery()
	a.wsClient.Close(false)
	close(a.HassEntityChannel)
}

func (a *HomeAssistantPlatform) connectWithReconnect(host string, ssl bool) *websocketClient {
	for {

		client := ConnectWS(host, "/api/websocket", ssl)
		if client == nil {
			log.Println("Fail to connect, reconnecting to Home Assistant in 30 seconds...")
			// Fail to connect wait to connect again
			select {
			case <-time.After(30 * time.Second):

			case <-a.context.Done():
				return nil
			}

		} else {
			return client
		}
	}
}

// GetEntity returns the entity
func (a *HomeAssistantPlatform) GetEntity(entity string) (HassEntity, bool) {
	return a.list.GetEntity(entity)
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
	}
	a.wsClient.SendMap(s)

}

func (a *HomeAssistantPlatform) subscribeEvents() {
	a.wsID = a.wsID + 1
	s := map[string]interface{}{
		"id":         a.wsID,
		"type":       "subscribe_events",
		"event_type": "state_changed"}

	a.wsClient.SendMap(s)

}

func (a *HomeAssistantPlatform) handleMessage(message Result) {

	if message.MessageType == "auth_required" {
		log.Print("message->: ", message)
		log.Println("Got auth required, sending auth token")

		a.wsClient.SendString("{\"type\": \"auth\",\"access_token\": \"" + a.token + "\"}")
	} else if message.MessageType == "auth_ok" {
		log.Print("message->: ", message)
		log.Println("Got auth_ok, downloading all states initially")
		a.sendMessage("get_states")
	} else if message.MessageType == "result" {

		if message.Id == a.getStateID {
			log.Println("Got all states, getting events")
			for _, data := range message.Result {
				newHassEntity := NewHassEntity(data.EntityId, data.EntityId, "hass", data.State, data.Attributes)
				a.list.SetEntity(newHassEntity)
				a.HassEntityChannel <- newHassEntity
			}

			a.subscribeEvents()
		}
	} else if message.MessageType == "event" {
		data := message.Event.Data
		log.Println("---------------------------------------")
		log.Printf("message->: %s=%s", data.EntityId, data.NewState.State)
		log.Println("---------------------------------------")
		newHassEntity := NewHassEntity(data.EntityId, data.EntityId, "hass", data.NewState.State, data.NewState.Attributes)
		a.list.SetEntity(newHassEntity)
		a.HassEntityChannel <- newHassEntity

	}

}
