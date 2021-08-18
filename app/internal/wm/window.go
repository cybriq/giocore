// SPDX-License-Identifier: Unlicense OR MIT

// package wm implements platform specific windows
// and GPU contexts.
package wm

import (
	"errors"
	"github.com/cybriq/giocore/io/key"
	"image/color"

	"github.com/cybriq/giocore/gpu"
	"github.com/cybriq/giocore/io/event"
	"github.com/cybriq/giocore/io/pointer"
	"github.com/cybriq/giocore/io/system"
	"github.com/cybriq/giocore/unit"
)

type Size struct {
	Width  unit.Value
	Height unit.Value
}

type Options struct {
	Size            *Size
	MinSize         *Size
	MaxSize         *Size
	Title           *string
	WindowMode      *WindowMode
	StatusColor     *color.NRGBA
	NavigationColor *color.NRGBA
	Orientation     *Orientation
	CustomRenderer  bool
}

type WakeupEvent struct{}

type WindowMode uint8

const (
	Windowed WindowMode = iota
	Fullscreen
)

type Orientation uint8

const (
	AnyOrientation Orientation = iota
	LandscapeOrientation
	PortraitOrientation
)

type FrameEvent struct {
	system.FrameEvent

	Sync bool
}

type Callbacks interface {
	SetDriver(d Driver)
	Event(e event.Event)
}

type Context interface {
	API() gpu.API
	RenderTarget() gpu.RenderTarget
	Present() error
	Refresh() error
	Release()
	Lock() error
	Unlock()
}

// ErrDeviceLost is returned from Context.Present when
// the underlying GPU device is gone and should be
// recreated.
var ErrDeviceLost = errors.New("GPU device lost")

// Driver is the interface for the platform implementation
// of a window.
type Driver interface {
	// SetAnimating sets the animation flag. When the window is animating,
	// FrameEvents are delivered as fast as the display can handle them.
	SetAnimating(anim bool)

	// ShowTextInput updates the virtual keyboard state.
	ShowTextInput(show bool)

	SetInputHint(mode key.InputHint)

	NewContext() (Context, error)

	// ReadClipboard requests the clipboard content.
	ReadClipboard()
	// WriteClipboard requests a clipboard write.
	WriteClipboard(s string)

	// Option processes option changes.
	Option(opts *Options)

	// SetCursor updates the current cursor to name.
	SetCursor(name pointer.CursorName)

	// Close the window.
	Close()
	// Wakeup wakes up the event loop and sends a WakeupEvent.
	Wakeup()
}

type windowRendezvous struct {
	in   chan windowAndOptions
	out  chan windowAndOptions
	errs chan error
}

type windowAndOptions struct {
	window Callbacks
	opts   *Options
}

func newWindowRendezvous() *windowRendezvous {
	wr := &windowRendezvous{
		in:   make(chan windowAndOptions),
		out:  make(chan windowAndOptions),
		errs: make(chan error),
	}
	go func() {
		var main windowAndOptions
		var out chan windowAndOptions
		for {
			select {
			case w := <-wr.in:
				var err error
				if main.window != nil {
					err = errors.New("multiple windows are not supported")
				}
				wr.errs <- err
				main = w
				out = wr.out
			case out <- main:
			}
		}
	}()
	return wr
}

func (_ WakeupEvent) ImplementsEvent() {}
