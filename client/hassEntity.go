package client

import (
	"fmt"
	"time"
)

type HassEntityState struct {
	LastUpdated time.Time
	LastChanged time.Time
	State       string
	Attributes  map[string]interface{}
}

type HassEntity struct {
	ID   string
	Name string
	New  HassEntityState
	Old  HassEntityState
}

func NewHassEntity(id string, name string, old HassEntityState, new HassEntityState) *HassEntity {
	return &HassEntity{
		ID:   id,
		Name: name,
		Old:  old,
		New:  new,
	}
}

type HassCallServiceEvent struct {
	TimeFired   time.Time
	Domain      string
	Service     string
	ServiceData map[string]interface{}
}

func NewHassCallServiceEvent(timeFired time.Time, domain string, service string, serviceData map[string]interface{}) *HassCallServiceEvent {
	return &HassCallServiceEvent{
		TimeFired:   timeFired,
		Domain:      domain,
		Service:     service,
		ServiceData: serviceData,
	}
}
func (a HassEntity) String() string {
	format := `%s
	state: %s
	last_updated: %s
	last_changed: %s
	attributes:`

	formatAttr := `
	- %s: %v`
	result := fmt.Sprintf(format, a.Name, a.New.State, a.New.LastUpdated, a.New.LastChanged)
	if len(a.New.Attributes) > 0 {

		for attr, val := range a.New.Attributes {
			result = result + fmt.Sprintf(formatAttr, attr, val)
		}
	}
	return result
}
