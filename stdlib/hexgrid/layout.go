package hexgrid

import "math"

type Point struct {
	x float64
	y float64
}

/*
Layout describes the layout of the hexagons

	    100(size) 50  50
	    |-------|---|---|
	    |       |   *********
	    |       |  * U:1,V:-1*
	    |       | *           *
	    |        * (150,-86.6) *
	    *********       +       *********
	   * U:0,V:0 *             * U:2,V:-1*
	  *           *           *           *
	 *    (0,0)    *         *   (300,0)   *
	*       +       *********       +       *
	 *             * U:1,V:0 *             *
	  *           *           *           *
	   *         *  (150,86.6) *         *
	    *********       +       *********
	             *             *
	              *           *
	               *         *
	                *********
*/
type Layout struct {
	Orientation Orientation
	Size        Point // multiplication factor relative to the canonical hexagon, where the Points are on a unit circle
	Origin      Point // center Point for hexagon 0,0
}

type Orientation struct {
	f0, f1, f2, f3, b0, b1, b2, b3, startAngle float64
}

var (
	OrientationPointy = Orientation{math.Sqrt(3.), math.Sqrt(3.) / 2., 0., 3. / 2., math.Sqrt(3.) / 3., -1. / 3., 0., 2. / 3., 0.5}
	OrientationFlat   = Orientation{3. / 2., 0., math.Sqrt(3.) / 2., math.Sqrt(3.), 2. / 3., 0., -1. / 3., math.Sqrt(3.) / 3., 0.}
)

// HexToPixel returns the center pixel for a given hexagon an a certain layout
func HexToPixel(l Layout, h Hex) Point {
	M := l.Orientation
	size := l.Size
	origin := l.Origin
	x := (M.f0*float64(h.q) + M.f1*float64(h.r)) * size.x
	y := (M.f2*float64(h.q) + M.f3*float64(h.r)) * size.y
	return Point{x + origin.x, y + origin.y}
}

// PixelToHex returns the corresponding hexagon axial coordinates for a given pixel on a certain layout
func PixelToHex(l Layout, p Point) fractionalHex {
	M := l.Orientation
	size := l.Size
	origin := l.Origin

	pt := Point{(p.x - origin.x) / size.x, (p.y - origin.y) / size.y}
	q := M.b0*pt.x + M.b1*pt.y
	r := M.b2*pt.x + M.b3*pt.y
	return fractionalHex{q, r, -q - r}
}

func HexCornerOffset(l Layout, c int) Point {
	M := l.Orientation
	size := l.Size
	angle := 2. * math.Pi * (M.startAngle - float64(c)) / 6.
	return Point{size.x * math.Cos(angle), size.y * math.Sin(angle)}
}

// Gets the corners of the hexagon for the given layout, starting at the E vertex and proceeding in a CCW order
func HexagonCorners(l Layout, h Hex) []Point {

	corners := make([]Point, 0)
	center := HexToPixel(l, h)

	for i := 0; i < 6; i++ {
		offset := HexCornerOffset(l, i)
		corners = append(corners, Point{center.x + offset.x, center.y + offset.y})
	}
	return corners
}
