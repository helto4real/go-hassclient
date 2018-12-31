package client

import "fmt"

type HassEntity struct {
	ID   string
	Name string
	Type string
	New  HassEntityState
	Old  HassEntityState
}
type HassEntityState struct {
	State      string
	Attributes map[string]string
}

func NewHassEntity(id string, name string, entityType string, old HassEntityState, new HassEntityState) *HassEntity {
	return &HassEntity{
		ID:   id,
		Name: name,
		Type: entityType,
		Old:  old,
		New:  new}
}

func (a HassEntity) String() string {
	format := `%s
	state: %s
	attributes:`

	formatAttr := `
	- %s: %s`
	result := fmt.Sprintf(format, a.Name, a.New.State)
	if len(a.New.Attributes) > 0 {

		for attr, val := range a.New.Attributes {
			result = result + fmt.Sprintf(formatAttr, attr, val)
		}
	}
	return result
}
