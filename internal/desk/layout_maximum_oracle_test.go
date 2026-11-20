package desk

import (
	"fmt"
	"testing"
)

// The independent two-rectangle rational oracle checks the complete32display
// limit. Display order affects fingerprints but must never pick a destination.
func TestLayoutMaximumSplitEdgeMatchesIndependentCandidates(t *testing.T) {
	for _, edge := range []Edge{Left, Right, Top, Bottom} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse-%t", edge, reverse), func(t *testing.T) {
				source := Display{ID: "source", Device: "primary", X: -100, Y: -100, Width: 310, Height: 310, PixelWidth: 997, PixelHeight: 991}
				displays := []Display{source}
				for i := 0; i < 31; i++ {
					d := Display{ID: fmt.Sprintf("target-%02d", i), Device: fmt.Sprintf("device-%02d", i), X: source.X + source.Width, Y: source.Y + int64(i)*10, Width: 70, Height: 10, PixelWidth: int64(101 + i*17), PixelHeight: int64(3 + i*7)}
					switch edge {
					case Left:
						d.X = source.X - d.Width
					case Top, Bottom:
						d.X = source.X + int64(i)*10
						d.Y = source.Y + source.Height
						d.Width = 10
						d.Height = 70
						if edge == Top {
							d.Y = source.Y - d.Height
						}
					}
					displays = append(displays, d)
				}
				if reverse {
					for i, j := 0, len(displays)-1; i < j; i, j = i+1, j-1 {
						displays[i], displays[j] = displays[j], displays[i]
					}
				}
				c, err := Compile(Layout{Version: 1, Displays: displays})
				if err != nil {
					t.Fatal(err)
				}
				extent := source.PixelHeight
				if edge == Top || edge == Bottom {
					extent = source.PixelWidth
				}
				for along := int64(0); along < extent; along++ {
					var want Candidate
					found := false
					for _, d := range displays {
						if d.ID == source.ID {
							continue
						}
						candidate, ok := rationalEdgeCandidate(source, d, edge, along)
						if ok {
							if found {
								t.Fatal("oracle multiple targets")
							}
							want, found = candidate, true
						}
					}
					got, ok, err := c.MapEdge(source.ID, edge, along)
					if err != nil || ok != found || got != want {
						t.Fatal("maximum split differs", along, got, ok, want, found, err)
					}
				}
			})
		}
	}
}
