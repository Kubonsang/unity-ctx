package core

type View string

const (
	ViewTiny    View = "tiny"
	ViewCompact View = "compact"
	ViewDetail  View = "detail"
)

type Result struct {
	Status    string `json:"status"`
	Namespace string `json:"namespace"`
	Command   string `json:"command"`
	File      string `json:"file"`
	View      View   `json:"view"`
	Body      string `json:"body"`
}

func (v View) Valid() bool {
	switch v {
	case ViewTiny, ViewCompact, ViewDetail:
		return true
	default:
		return false
	}
}
