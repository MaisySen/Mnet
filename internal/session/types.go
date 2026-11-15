package session

// Payload types carried as JSON on the typed streams.

// ScreenOpen describes the screen stream.
type ScreenOpen struct {
	Codec  string `json:"codec"` // "jpeg" in v1
	Width  int    `json:"width"`
	Height int    `json:"height"`
	FPS    int    `json:"fps"`
}

// ScreenFrame is one screen update. Rects is empty for a full frame;
// v1 always sends full frames when anything changed, and the rect
// list keeps dirty-rect deltas a compatible later step.
type ScreenFrame struct {
	Seq   uint32 `json:"seq"`
	Full  bool   `json:"full"`
	Rects []Rect `json:"rects,omitempty"`
	Image []byte `json:"image"` // codec bytes (JPEG in v1)
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// InputEvent is one viewer-to-host input message.
type InputEvent struct {
	Kind   string `json:"kind"` // "pointer" or "key"
	X      int    `json:"x,omitempty"`
	Y      int    `json:"y,omitempty"`
	Button string `json:"button,omitempty"` // "left", "right", "middle"
	Action string `json:"action,omitempty"` // "down", "up", "move", "click", "scroll"
	DX     int    `json:"dx,omitempty"`     // scroll delta
	DY     int    `json:"dy,omitempty"`
	Text   string `json:"text,omitempty"` // for kind=key
}

// ChatMessage is one chat line.
type ChatMessage struct {
	From string `json:"from"`
	Text string `json:"text"`
	At   int64  `json:"at"` // unix seconds
}

// FileOffer opens a file transfer stream.
type FileOffer struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // hex digest of the whole file
}

// FileAccept is the receiver's answer.
type FileAccept struct {
	OK     bool  `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Offset int64 `json:"offset"` // resume position
}

// FileAck closes a completed transfer.
type FileAck struct {
	OK     bool   `json:"ok"`
	SHA256 string `json:"sha256"` // receiver's computed digest
}
