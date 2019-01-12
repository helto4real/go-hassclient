package client

type Result struct {
	Id          int64       `json:"id"`
	MessageType string      `json:"type"`
	Success     bool        `json:"success"`
	Result      []GetResult `json:"result"`
	Event       Event       `json:"event"`
}

type GetResult struct {
	EntityId    string                 `json:"entity_id"`
	LastChanged string                 `json:"last_changed"`
	LastUpdated string                 `json:"last_updated"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
}
type Event struct {
	Data      EventData `json:"data"`
	EventType string    `json:"event_type"`
	TimeFired string    `json:"time_fired"`
}

type EventData struct {
	EntityId    string                 `json:"entity_id"`
	NewState    StateData              `json:"new_state"`
	OldState    StateData              `json:"old_state"`
	Domain      string                 `json:"domain"`
	Service     string                 `json:"service"`
	ServiceData map[string]interface{} `json:"service_data"`
}

type StateData struct {
	LastChanged string                 `json:"last_changed"`
	LastUpdated string                 `json:"last_updated"`
	State       interface{}            `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
}

type SetStateData struct {
	State      interface{}            `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}
