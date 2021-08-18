// SPDX-License-Identifier: Unlicense OR MIT

//go:build linux || freebsd || openbsd
// +build linux freebsd openbsd

package headless

import (
	"github.com/cybriq/giocore/internal/egl"
)

func newContext() (context, error) {
	return egl.NewContext(egl.EGL_DEFAULT_DISPLAY)
}
