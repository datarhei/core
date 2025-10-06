package fs

type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionRemove Action = "remove"
	ActionList   Action = "list"
)

func (a Action) String() string {
	return string(a)
}
