package common

import "fmt"

type ItemType string

const (
	ItemTypeApplication ItemType = "Application"
)

type ActionType string

const (
	ActionTypeLaunch ActionType = "Launch"
	ActionTypeCopy   ActionType = "Copy"
)

type ItemAction struct {
	Title    string     `json:"title"`
	Icon     *string    `json:"icon,omitempty"`
	Shortcut *[]string  `json:"shortcut,omitempty"`
	Action   ActionType `json:"action"`
}

type BaseItem struct {
	ID          string       `json:"id"`
	PluginID    string       `json:"pluginID"`
	Type        ItemType     `json:"type"`
	Title       string       `json:"title"`
	SubTitle    *string      `json:"subtitle,omitempty"`
	Icon        *string      `json:"icon,omitempty"`
	Keywords    []string     `json:"keywords"`
	Actions     []ItemAction `json:"actions"`
	LaunchCount uint         `json:"launchCount"`
}

func (i *BaseItem) GetID() string {
	return i.ID
}

func (i *BaseItem) GetTitle() string {
	return i.Title
}

func (i *BaseItem) GetLaunchCount() uint {
	return i.LaunchCount
}

type SortableItem interface {
	GetID() string
	GetTitle() string
	GetLaunchCount() uint
}

func EmptyToOptionalString(v string) *string {
	if v == "" {
		return nil
	}

	return &v
}

func EmptyToOptionalIcon(v string, size int) *string {
	if v == "" {
		return nil
	}

	v = fmt.Sprintf("/icons/%s?size=%d", v, size)

	return &v
}
