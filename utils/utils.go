package utils

import (
	"github.com/l0k18/gio/f32"
	"image"
)

// FPt converts an point to a f32.Point.
func FPt(p image.Point) f32.Point {
	return f32.Point{
		X: float32(p.X), Y: float32(p.Y),
	}
}

// FRect converts a rectangle to a f32.Rectangle.
func FRect(r image.Rectangle) f32.Rectangle {
	return f32.Rectangle{
		Min: FPt(r.Min), Max: FPt(r.Max),
	}
}
