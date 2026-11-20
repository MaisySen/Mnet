package desk

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Local parser/geometry fuzzing never creates a TLS socket, native input,
// approval or backend. A valid document is still only offline evidence.
func fuzzDeskLayout() Layout {
	return Layout{Version: 1, Displays: []Display{
		{ID: "source", Device: strings.Repeat("a", 64), Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768},
		{ID: "target", Device: strings.Repeat("b", 64), X: 100, Width: 100, Height: 100, PixelWidth: 1280, PixelHeight: 960},
	}}
}

func exerciseFuzzGeometry(t *testing.T, c *Compiled) {
	t.Helper()
	before := c.Fingerprint()
	displays := c.Displays()
	for _, source := range displays {
		for _, edge := range []Edge{Left, Right, Top, Bottom} {
			extent := source.PixelWidth
			if edge == Left || edge == Right {
				extent = source.PixelHeight
			}
			for _, along := range []int64{math.MinInt64, -1, 0, extent / 2, extent - 1, extent, math.MaxInt64} {
				candidate, ok, err := c.MapEdge(source.ID, edge, along)
				if along < 0 || along >= extent {
					if err == nil || ok {
						t.Fatal("outside source pixel accepted", edge, along, candidate)
					}
					continue
				}
				if err != nil {
					t.Fatal("validated geometry mapping failed", edge, along, err)
				}
				if !ok {
					continue
				}
				found := false
				for _, target := range displays {
					if target.ID == candidate.Display {
						found = target.Device == candidate.Device && candidate.X >= 0 && candidate.Y >= 0 && candidate.X < target.PixelWidth && candidate.Y < target.PixelHeight
					}
				}
				if !found {
					t.Fatal("candidate outside exact destination", candidate)
				}
			}
		}
	}
	// Neither inspection nor edits to returned display/link copies mutate the
	// independently approved arrangement fingerprint.
	if len(displays) > 0 {
		displays[0].X++
		displays[0].Device = "different"
	}
	links := c.Links()
	if len(links) > 0 {
		links[0].To = "different"
	}
	if c.Fingerprint() != before {
		t.Fatal("inspection changed approved geometry")
	}
}

func FuzzDeskLayoutDecoder(f *testing.F) {
	seed, _ := json.Marshal(fuzzDeskLayout())
	for _, data := range [][]byte{seed, {}, []byte(`null`), []byte(`{"version":1,"version":2}`), bytes.Repeat([]byte("["), 12), bytes.Repeat([]byte(" "), 65537)} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		layout, err := Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		encoded, err := json.Marshal(layout)
		if err != nil {
			t.Fatal(err)
		}
		round, err := Decode(bytes.NewReader(encoded))
		if err != nil || !reflect.DeepEqual(round, layout) {
			t.Fatal("decoded layout not roundtrippable", err)
		}
		compiled, err := Compile(layout)
		if err != nil {
			return
		}
		exerciseFuzzGeometry(t, compiled)
	})
}

func FuzzDeskRunDecoder(f *testing.F) {
	layout := fuzzDeskLayout()
	compiled, err := Compile(layout)
	if err != nil {
		f.Fatal(err)
	}
	spec := RunSpec{Version: 1, Role: "primary", Address: "127.0.0.1:4567", Certificate: "cert.pem", PrivateKey: "key.pem", RunDirectory: "runs", LifetimeSeconds: 30, SourceDisplay: "source", Edge: Right, Layout: layout,
		Binding: Binding{Run: strings.Repeat("c", 64), Primary: layout.Displays[0].Device, Recipient: layout.Displays[1].Device, Layout: compiled.Fingerprint(), Keyboard: strings.Repeat("d", 64), Display: "target"}}
	seed, _ := json.Marshal(spec)
	for _, data := range [][]byte{seed, {}, []byte(`null`), append(append([]byte{}, seed...), []byte(`{}`)...), bytes.Repeat([]byte(" "), 65537)} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got, compiled, err := DecodeRun(bytes.NewReader(data))
		if err != nil {
			return
		}
		if compiled == nil || got.Binding.Validate(compiled) != nil {
			t.Fatal("decoded run without exact layout binding")
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		round, again, err := DecodeRun(bytes.NewReader(encoded))
		if err != nil || again == nil || !reflect.DeepEqual(round, got) || again.Fingerprint() != compiled.Fingerprint() {
			t.Fatal("run not roundtrippable", err)
		}
		exerciseFuzzGeometry(t, compiled)
	})
}

func FuzzDeskWireDecoder(f *testing.F) {
	for _, data := range [][]byte{{}, {0, 0, 0, 0}, {0xff, 0xff, 0xff, 0xff}, {0, 0, 0, 2, '{', '}'}, {0, 0, 0, 15, '{', '"', 'k', 'i', 'n', 'd', '"', ':', '"', 's', 't', 'o', 'p', '"', '}'}} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		got, err := readWire(reader)
		if err != nil {
			return
		}
		if len(data) < 4 {
			t.Fatal("missing frame header accepted")
		}
		size := binary.BigEndian.Uint32(data[:4])
		if size == 0 || size > 4096 || int(size)+4 > len(data) || reader.Len() != len(data)-int(size)-4 {
			t.Fatal("frame boundary escaped")
		}
		var encoded bytes.Buffer
		if err := writeWire(&encoded, got); err != nil {
			t.Fatal(err)
		}
		round, err := readWire(&encoded)
		if err != nil || !reflect.DeepEqual(round, got) || encoded.Len() != 0 {
			t.Fatal("wire roundtrip changed fields", err)
		}
	})
}

// Structured valid geometry keeps pixel-density/offset arithmetic reachable
// without requiring mutations to rediscover a valid complete JSON document.
func FuzzDeskMappedGeometry(f *testing.F) {
	f.Add(int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(0), false)
	f.Add(int64(999999), int64(65534), int64(999999), int64(65534), int64(-1), int64(-1), int64(-500000), true)
	f.Fuzz(func(t *testing.T, a, b, c, d, e, g, origin int64, vertical bool) {
		span := func(v int64) int64 { return int64(uint64(v)%1000000) + 1 }
		pixels := func(v int64) int64 { return int64(uint64(v)%65535) + 1 }
		start := -int64(uint64(origin) % 500001)
		source := Display{ID: "source", Device: "primary", X: start, Y: start, Width: span(a), Height: span(c), PixelWidth: pixels(b), PixelHeight: pixels(d)}
		target := Display{ID: "target", Device: "recipient", X: source.X + source.Width, Y: start, Width: span(e), Height: span(g), PixelWidth: pixels(e), PixelHeight: pixels(g)}
		if vertical {
			target.X = start
			target.Y = source.Y + source.Height
		}
		compiled, err := Compile(Layout{Version: 1, Displays: []Display{source, target}})
		if err != nil {
			t.Fatal("bounded adjacent geometry refused", err)
		}
		exerciseFuzzGeometry(t, compiled)
	})
}
