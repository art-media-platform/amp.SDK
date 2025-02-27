package tag

import "math"

// WORK IN PROGRESS proof-of-concept tag.ID visualizer, somewhat like how a QR code encodes a URL.
//
// The visualization can be exported as a svg or json and appears as hexagonal lattice of dot "glyphs".
// Each dot glyph encodes 3 bits (8 possible values), requiring 64 glyphs to encode the 24 bytes of a tag.ID.
//
// Spatial error correction is achieved by mirroring the glyphs along the Y axis while the hexagonal lattice packs the glyphs efficiently.
//
//

// Canonical ASCII representation of a dot glyph in a tag.ID visual encoding (3 bits)
var CanonicAsciiAlphabet = [8]AsciiDigit{
	'_', '.', ':', '*',
	'~', 'o', 'O', '0',
}

type AsciiDigit byte // base 8: ascii rune
type OctalDigit byte // base 8: 3 bits

type OctalEncoding [64]OctalDigit

const AsciiTemplate_v1 = "" +
	"     : * : . N N N N N N    \n" +
	"    ~ : : . N N N N N N N   \n" +
	"   O O   . N N N N N N N N  \n" +
	"    0 O o N N N N N N N N   \n" +
	"     O o N N N N N N N N    \n" +
	"      o N N N N N N N N     \n" +
	"       N N N N N N N N      \n" +
	"        N N N N N N N       \n" +
	"         N N N N N N        \n" +
	"          N N N N N         \n" +
	"           N N N N          \n" +
	"            N N N           \n" +
	"                            \n"

const AsciiTemplate_v2 = "" +
	"        * N N N\n" +
	"       * N N N \n" +
	"      : N N N N\n" +
	"     : N N N N \n" +
	"    . N N N N N\n" +
	"   . N N N N N \n" +
	"  . N N N N N N\n" +
	" ~ N N N N N N \n" +
	"  o N N N N N N\n" +
	"   o N N N N N \n" +
	"    o N N N N N\n" +
	"     O N N N N \n" +
	"      O N N N N\n" +
	"       0 N N N \n" +
	"        0 N N N\n"

const AsciiTemplate_v3 = "" + // TODO: mirror + rotate to form a regular hexagon symmetrical along the y-axis
	"  x x x x x x x x " + // 8
	"   x x x x x x x  " + // 7
	"  x x x x x x x   " + // 7
	"   x x x x x x    " + // 6
	"  x x x x x x     " + // 6
	"   x x x x x      " + // 5
	"  x x x x x       " + // 5
	"   x x x x        " + // 4
	"  x x x x         " + // 4
	"   x x x          " + // 3
	"  x x x           " + // 3
	"   x x            " + // 2
	"  x x             " + // 2
	"   x              " + // 1
	"  x               " // 1 == 64

type Dot struct {
	X, Y      float32 // 0..1
	Amplitude float32 // -1..1
	CharIndex int
	IndexR    int
}

// // function oddr_offset_to_pixel(hex):
//     var x = size * sqrt(3) * (hex.col + 0.5 * (hex.row&1))
//     var y = size * 3/2 * hex.row
//     return Point(x, y)

type Badge struct {
	Dots     []Dot  // location of each dot in template
	Template []byte // ascii template with newlines
}

func (badge *Badge) RegenFromTemplate(template string) {
	if len(badge.Dots) <= 64 {
		badge.Dots = make([]Dot, 0, 64)
	} else {
		badge.Dots = badge.Dots[:0]
	}
	templateLen := len(template)
	if len(badge.Template) <= templateLen {
		badge.Template = make([]byte, 0, 2*templateLen)
	} else {
		badge.Template = badge.Template[:0]
	}

	root3 := float32(math.Sqrt(3))

	row, col := 0, 0
	for ci := 0; ci < templateLen; ci++ {
		c := template[ci]
		switch c {
		case 'N':
			badge.Dots = append(badge.Dots, Dot{
				X:         root3 * (float32(col) + 0.5*float32(row&1)),
				Y:         1.5 * float32(row),
				CharIndex: ci,
			})
		case '\n':
			// rowLen := ci - rowStart - 1
			// for i := 0; i < rowLen; i++ {
			// 	badge.Template = append(badge.Template, ' ')
			// }
			// dots := len(badge.Dots)
			// for mi := (row & 1); mi < rowLen; mi++ { // mirror the current row
			// 	c_mirror := template[ci-mi]
			// 	badge.Template = append(badge.Template, c_mirror)
			// 	if c_mirror == 'N' {
			// 		dots--
			// 		badge.Dots[dots].IndexR = len(badge.Template)
			// 	}
			// }
			// for di := rowStartDot; di < len(badge.Dots); di++ {
			// 	badge.Dots[di].IndexR = 2*rowLen - badge.Dots[di].IndexL
			// }
			// 	dot.IndexR = len(badge.Dots)
			// 	badge.Dots = append(badge.Dots, Dot{
			// 		X:         -dot.X,
			// 		Y:         dot.Y,
			// 		CharIndex: ci,
			// 		Sort:   col + row,
			// 	})
			// }

			// // mirror the current row
			// for di := dotL; di < len(badge.Dots); di++ {
			// 	dot := &badge.Dots[di]
			// 	dot.IndexR = len(badge.Dots)
			// 	badge.Dots = append(badge.Dots, Dot{
			// 		X:         -dot.X,
			// 		Y:         dot.Y,
			// 		CharIndex: ci,
			// 		Sort:   col + row,
			// 	})
			// }
			row++
			col = 0

		}
		badge.Template = append(badge.Template, c)
	}

	// sort.Slice(badge.Dots, func(i, j int) bool {
	// 	ix, iy := badge.Dots[i].X, badge.Dots[i].Y
	// 	jx, jy := badge.Dots[j].X, badge.Dots[j].Y
	// 	wi := ix + iy
	// 	return badge.Dots[i].CharIndex < badge.Dots[j].CharIndex
	// }
}

var gBadge6424 Badge

func init() {
	gBadge6424.RegenFromTemplate(AsciiTemplate_v2)
}
