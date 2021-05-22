// SPDX-License-Identifier: Unlicense OR MIT

package srgb

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"strings"

	"github.com/l0k18/gio/internal/byteslice"
	"github.com/l0k18/gio/internal/gl"
)

// FBO implements an intermediate sRGB FBO
// for gamma-correct rendering on platforms without
// sRGB enabled native framebuffers.
type FBO struct {
	c           *gl.Functions
	viewport    image.Point
	srgbBuffer  gl.Framebuffer
	depthBuffer gl.Renderbuffer
	colorTex    gl.Texture
	blitted     bool
	quad        gl.Buffer
	prog        gl.Program
	gl3         bool
}

func New(f *gl.Functions) (*FBO, error) {
	var gl3 bool
	glVer := f.GetString(gl.VERSION)
	ver, _, err := gl.ParseGLVersion(glVer)
	if err != nil {
		return nil, err
	}
	if ver[0] >= 3 {
		gl3 = true
	} else {
		exts := f.GetString(gl.EXTENSIONS)
		if !strings.Contains(exts, "EXT_sRGB") {
			return nil, fmt.Errorf("no support for OpenGL ES 3 nor EXT_sRGB")
		}
	}
	s := &FBO{
		c:           f,
		gl3:         gl3,
		srgbBuffer:  f.CreateFramebuffer(),
		colorTex:    f.CreateTexture(),
		depthBuffer: f.CreateRenderbuffer(),
	}
	f.BindTexture(gl.TEXTURE_2D, s.colorTex)
	f.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	f.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	f.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	f.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	return s, nil
}

func (s *FBO) Blit() {
	if !s.blitted {
		prog, err := gl.CreateProgram(s.c, blitVSrc, blitFSrc, []string{"pos", "uv"})
		if err != nil {
			panic(err)
		}
		s.prog = prog
		s.c.UseProgram(prog)
		s.c.Uniform1i(s.c.GetUniformLocation(prog, "tex"), 0)
		s.quad = s.c.CreateBuffer()
		s.c.BindBuffer(gl.ARRAY_BUFFER, s.quad)
		coords := byteslice.Slice([]float32{
			-1, +1, 0, 1,
			+1, +1, 1, 1,
			-1, -1, 0, 0,
			+1, -1, 1, 0,
		})
		s.c.BufferData(gl.ARRAY_BUFFER, len(coords), gl.STATIC_DRAW)
		s.c.BufferSubData(gl.ARRAY_BUFFER, 0, coords)
		s.blitted = true
	}
	s.c.UseProgram(s.prog)
	s.c.BindTexture(gl.TEXTURE_2D, s.colorTex)
	s.c.BindBuffer(gl.ARRAY_BUFFER, s.quad)
	s.c.VertexAttribPointer(0 /* pos */, 2, gl.FLOAT, false, 4*4, 0)
	s.c.VertexAttribPointer(1 /* uv */, 2, gl.FLOAT, false, 4*4, 4*2)
	s.c.EnableVertexAttribArray(0)
	s.c.EnableVertexAttribArray(1)
	s.c.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	s.c.BindTexture(gl.TEXTURE_2D, gl.Texture{})
	s.c.DisableVertexAttribArray(0)
	s.c.DisableVertexAttribArray(1)
	s.c.BindFramebuffer(gl.FRAMEBUFFER, s.srgbBuffer)
	s.c.InvalidateFramebuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0)
	s.c.InvalidateFramebuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT)
}

func (s *FBO) Framebuffer() gl.Framebuffer {
	return s.srgbBuffer
}

func (s *FBO) Refresh(viewport image.Point) error {
	if viewport.X == 0 || viewport.Y == 0 {
		return errors.New("srgb: zero-sized framebuffer")
	}
	if s.viewport == viewport {
		return nil
	}
	s.viewport = viewport
	s.c.BindTexture(gl.TEXTURE_2D, s.colorTex)
	if s.gl3 {
		s.c.TexImage2D(gl.TEXTURE_2D, 0, gl.SRGB8_ALPHA8, viewport.X, viewport.Y, gl.RGBA, gl.UNSIGNED_BYTE)
	} else /* EXT_sRGB */ {
		s.c.TexImage2D(gl.TEXTURE_2D, 0, gl.SRGB_ALPHA_EXT, viewport.X, viewport.Y, gl.SRGB_ALPHA_EXT, gl.UNSIGNED_BYTE)
	}
	currentRB := gl.Renderbuffer(s.c.GetBinding(gl.RENDERBUFFER_BINDING))
	s.c.BindRenderbuffer(gl.RENDERBUFFER, s.depthBuffer)
	s.c.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, viewport.X, viewport.Y)
	s.c.BindRenderbuffer(gl.RENDERBUFFER, currentRB)
	s.c.BindFramebuffer(gl.FRAMEBUFFER, s.srgbBuffer)
	s.c.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, s.colorTex, 0)
	s.c.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, s.depthBuffer)
	if st := s.c.CheckFramebufferStatus(gl.FRAMEBUFFER); st != gl.FRAMEBUFFER_COMPLETE {
		return fmt.Errorf("sRGB framebuffer incomplete (%dx%d), status: %#x error: %x", viewport.X, viewport.Y, st, s.c.GetError())
	}

	if runtime.GOOS == "js" {
		// With macOS Safari, rendering to and then reading from a SRGB8_ALPHA8
		// texture result in twice gamma corrected colors. Using a plain RGBA
		// texture seems to work.
		s.c.ClearColor(.5, .5, .5, 1.0)
		s.c.Clear(gl.COLOR_BUFFER_BIT)
		var pixel [4]byte
		s.c.ReadPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel[:])
		if pixel[0] == 128 { // Correct sRGB color value is ~188
			s.c.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, viewport.X, viewport.Y, gl.RGBA, gl.UNSIGNED_BYTE)
			if st := s.c.CheckFramebufferStatus(gl.FRAMEBUFFER); st != gl.FRAMEBUFFER_COMPLETE {
				return fmt.Errorf("fallback RGBA framebuffer incomplete (%dx%d), status: %#x error: %x", viewport.X, viewport.Y, st, s.c.GetError())
			}
		}
	}

	return nil
}

func (s *FBO) Release() {
	s.c.DeleteFramebuffer(s.srgbBuffer)
	s.c.DeleteTexture(s.colorTex)
	s.c.DeleteRenderbuffer(s.depthBuffer)
	if s.blitted {
		s.c.DeleteBuffer(s.quad)
		s.c.DeleteProgram(s.prog)
	}
	s.c = nil
}

const (
	blitVSrc = `
#version 100

precision highp float;

attribute vec2 pos;
attribute vec2 uv;

varying vec2 vUV;

void main() {
    gl_Position = vec4(pos, 0, 1);
    vUV = uv;
}
`
	blitFSrc = `
#version 100

precision mediump float;

uniform sampler2D tex;
varying vec2 vUV;

vec3 gamma(vec3 rgb) {
	vec3 exp = vec3(1.055)*pow(rgb, vec3(0.41666)) - vec3(0.055);
	vec3 lin = rgb * vec3(12.92);
	bvec3 cut = lessThan(rgb, vec3(0.0031308));
	return vec3(cut.r ? lin.r : exp.r, cut.g ? lin.g : exp.g, cut.b ? lin.b : exp.b);
}

void main() {
    vec4 col = texture2D(tex, vUV);
	vec3 rgb = col.rgb;
	rgb = gamma(rgb);
	gl_FragColor = vec4(rgb, col.a);
}
`
)
