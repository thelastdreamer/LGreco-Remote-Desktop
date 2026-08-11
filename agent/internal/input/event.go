package input

type Event struct {
	Type    string  `json:"type"`
	KeyCode int     `json:"keycode,omitempty"`
	Key     string  `json:"key,omitempty"`
	X       float64 `json:"x,omitempty"` // 0..1 normalized
	Y       float64 `json:"y,omitempty"`
	Button  int     `json:"button,omitempty"`
	Delta   float64 `json:"delta,omitempty"`
}
