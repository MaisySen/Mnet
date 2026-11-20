package desk

import (
	"math/big"
	"testing"
)

// Independent rational oracle: no compiled links or production mapping helper.
// Queries remain offline geometry, never edge arrival, authority or input.
func rationalEdgeCandidate(a, b Display, edge Edge, along int64) (Candidate, bool) {
	vertical := edge == Left || edge == Right
	adjacent := edge == Left && a.X == b.X+b.Width || edge == Right && a.X+a.Width == b.X || edge == Top && a.Y == b.Y+b.Height || edge == Bottom && a.Y+a.Height == b.Y
	if !adjacent {
		return Candidate{}, false
	}
	origin, size, pixels, targetOrigin, targetSize, targetPixels := a.X, a.Width, a.PixelWidth, b.X, b.Width, b.PixelWidth
	if vertical {
		origin, size, pixels, targetOrigin, targetSize, targetPixels = a.Y, a.Height, a.PixelHeight, b.Y, b.Height, b.PixelHeight
	}
	tangent := new(big.Rat).Mul(big.NewRat(along, pixels), big.NewRat(size, 1))
	position := new(big.Rat).Add(big.NewRat(origin, 1), tangent)
	if position.Cmp(big.NewRat(targetOrigin, 1)) < 0 || position.Cmp(big.NewRat(targetOrigin+targetSize, 1)) >= 0 {
		return Candidate{}, false
	}
	mapped := new(big.Rat).Sub(position, big.NewRat(targetOrigin, 1))
	mapped.Mul(mapped, big.NewRat(targetPixels, targetSize))
	native := new(big.Int).Quo(mapped.Num(), mapped.Denom()).Int64()
	out := Candidate{Display: b.ID, Device: b.Device}
	if vertical {
		out.Y = native
		if edge == Left {
			out.X = b.PixelWidth - 1
		}
	} else {
		out.X = native
		if edge == Top {
			out.Y = b.PixelHeight - 1
		}
	}
	return out, true
}

func FuzzDeskExactRationalPartialEdges(f *testing.F) {
	for direction := uint8(0); direction < 4; direction++ {
		f.Add(uint64(999999), uint64(999998), uint64(65534), uint64(65533), uint64(999997), uint64(999996), uint64(999998), uint64(65532), uint64(65531), direction)
	}
	f.Add(uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint8(0))
	f.Add(^uint64(0), uint64(999999), uint64(65534), uint64(65533), uint64(777777), uint64(876543), uint64(499999), uint64(65530), uint64(65529), uint8(3))
	f.Fuzz(func(t *testing.T, aw, ah, apx, apy, bw, bh, offset, bpx, bpy uint64, direction uint8) {
		dim := func(v uint64) int64 { return int64(v%1000000) + 1 }
		pixel := func(v uint64) int64 { return int64(v%65535) + 1 }
		a := Display{ID: "a", Device: "primary", Width: dim(aw), Height: dim(ah), PixelWidth: pixel(apx), PixelHeight: pixel(apy)}
		b := Display{ID: "b", Device: "recipient", Width: dim(bw), Height: dim(bh), PixelWidth: pixel(bpx), PixelHeight: pixel(bpy)}
		switch direction % 4 {
		case 0:
			b.X = a.Width
			b.Y = int64(offset % uint64(a.Height))
		case 1:
			b.X = -b.Width
			b.Y = int64(offset % uint64(a.Height))
		case 2:
			b.Y = a.Height
			b.X = int64(offset % uint64(a.Width))
		case 3:
			b.Y = -b.Height
			b.X = int64(offset % uint64(a.Width))
		}
		c, err := Compile(Layout{Version: 1, Displays: []Display{a, b}})
		if err != nil {
			t.Fatal("valid partial edge refused", err)
		}
		for _, pair := range [][2]Display{{a, b}, {b, a}} {
			source, target := pair[0], pair[1]
			for _, edge := range []Edge{Left, Right, Top, Bottom} {
				extent := source.PixelWidth
				if edge == Left || edge == Right {
					extent = source.PixelHeight
				}
				for _, along := range []int64{0, extent / 2, extent - 1, int64(offset % uint64(extent))} {
					want, matched := rationalEdgeCandidate(source, target, edge, along)
					got, ok, err := c.MapEdge(source.ID, edge, along)
					if err != nil || ok != matched || got != want {
						t.Fatal("exact rational mapping differs", source, target, edge, along, got, ok, want, matched, err)
					}
				}
			}
		}
	})
}
