// SPDX-License-Identifier: Unlicense OR MIT

package wm

/*
#include <android/native_window_jni.h>
#include <EGL/egl.h>
*/
import "C"

import (
	"unsafe"

	"github.com/l0k18/giocore/internal/egl"
)

type context struct {
	win *window
	*egl.Context
}

func (w *window) NewContext() (Context, error) {
	ctx, err := egl.NewContext(nil)
	if err != nil {
		return nil, err
	}
	return &context{win: w, Context: ctx}, nil
}

func (c *context) Release() {
	if c.Context != nil {
		c.Context.Release()
		c.Context = nil
	}
}

func (c *context) MakeCurrent() error {
	c.Context.ReleaseSurface()
	var (
		win           *C.ANativeWindow
		width, height int
	)
	// Run on main thread. Deadlock is avoided because MakeCurrent is only
	// called during a FrameEvent.
	c.win.callbacks.Run(func() {
		win, width, height = c.win.nativeWindow(c.Context.VisualID())
	})
	if win == nil {
		return nil
	}
	eglSurf := egl.NativeWindowType(unsafe.Pointer(win))
	if err := c.Context.CreateSurface(eglSurf, width, height); err != nil {
		return err
	}
	if err := c.Context.MakeCurrent(); err != nil {
		return err
	}
	return nil
}

func (c *context) Lock() {}

func (c *context) Unlock() {}
