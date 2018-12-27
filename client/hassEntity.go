package client

type HassEntity struct {
	ID         string
	Name       string
	Type       string
	State      string
	Attributes map[string]string
}

// GetID returns unique id of entity
func (a HassEntity) GetID() string                    { return a.ID }
func (a HassEntity) GetState() string                 { return a.State }
func (a HassEntity) GetType() string                  { return a.Type }
func (a HassEntity) GetAttributes() map[string]string { return a.Attributes }
func (a HassEntity) GetName() string                  { return a.Name }

func NewHassEntity(id string, name string, entityType string, state string, attributes map[string]string) *HassEntity {
	return &HassEntity{
		ID:         id,
		Name:       name,
		Type:       entityType,
		State:      state,
		Attributes: attributes}
}
