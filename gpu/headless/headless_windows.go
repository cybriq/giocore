// SPDX-License-Identifier: Unlicense OR MIT

package headless

import (
	"unsafe"

	"github.com/l0k18/giocore/gpu"
	"github.com/l0k18/giocore/internal/d3d11"
)

type d3d11Context struct {
	dev *d3d11.Device
}

func newContext() (context, error) {
	dev, ctx, _, err := d3d11.CreateDevice(
		d3d11.DRIVER_TYPE_HARDWARE,
		0,
	)
	if err != nil {
		return nil, err
	}
	// Don't need it.
	d3d11.IUnknownRelease(unsafe.Pointer(ctx), ctx.Vtbl.Release)
	return &d3d11Context{dev: dev}, nil
}

func (c *d3d11Context) API() gpu.API {
	return gpu.Direct3D11{Device: unsafe.Pointer(c.dev)}
}

func (c *d3d11Context) MakeCurrent() error {
	return nil
}

func (c *d3d11Context) ReleaseCurrent() {
}

func (c *d3d11Context) Release() {
	d3d11.IUnknownRelease(unsafe.Pointer(c.dev), c.dev.Vtbl.Release)
	c.dev = nil
}
