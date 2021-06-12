// SPDX-License-Identifier: Unlicense OR MIT

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/cybriq/giocore/gpu/internal/driver"
)

func main() {
	packageName := flag.String("package", "", "specify Go package name")
	workdir := flag.String("work", "", "temporary working directory (default TEMP)")
	shadersDir := flag.String("dir", "shaders", "shaders directory")
	directCompute := flag.Bool("directcompute", false, "enable compiling DirectCompute shaders")

	flag.Parse()

	var work WorkDir
	cleanup := func() {}
	if *workdir == "" {
		tempdir, err := ioutil.TempDir("", "shader-convert")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create tempdir: %v\n", err)
			os.Exit(1)
		}
		cleanup = func() { os.RemoveAll(tempdir) }
		defer cleanup()

		work = WorkDir(tempdir)
	} else {
		if abs, err := filepath.Abs(*workdir); err == nil {
			*workdir = abs
		}
		work = WorkDir(*workdir)
	}

	var out bytes.Buffer
	conv := NewConverter(work, *packageName, *shadersDir, *directCompute)
	if err := conv.Run(&out); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		cleanup()
		os.Exit(1)
	}

	if err := ioutil.WriteFile("shaders.go", out.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create shaders: %v\n", err)
		cleanup()
		os.Exit(1)
	}

	cmd := exec.Command("gofmt", "-s", "-w", "shaders.go")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "formatting shaders.go failed: %v\n", err)
		cleanup()
		os.Exit(1)
	}
}

type Converter struct {
	workDir       WorkDir
	shadersDir    string
	directCompute bool

	packageName string

	glslvalidator *GLSLValidator
	spirv         *SPIRVCross
	fxc           *FXC
}

func NewConverter(workDir WorkDir, packageName, shadersDir string, directCompute bool) *Converter {
	if abs, err := filepath.Abs(shadersDir); err == nil {
		shadersDir = abs
	}

	conv := &Converter{}
	conv.workDir = workDir
	conv.shadersDir = shadersDir
	conv.directCompute = directCompute

	conv.packageName = packageName

	conv.glslvalidator = NewGLSLValidator()
	conv.spirv = NewSPIRVCross()
	conv.fxc = NewFXC()

	verifyBinaryPath(&conv.glslvalidator.Bin)
	verifyBinaryPath(&conv.spirv.Bin)
	// We cannot check fxc since it may depend on wine.

	conv.glslvalidator.WorkDir = workDir.Dir("glslvalidator")
	conv.fxc.WorkDir = workDir.Dir("fxc")
	conv.spirv.WorkDir = workDir.Dir("spirv")

	return conv
}

func verifyBinaryPath(bin *string) {
	new, err := exec.LookPath(*bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to find %q: %v\n", *bin, err)
	} else {
		*bin = new
	}
}

func (conv *Converter) Run(out io.Writer) error {
	shaders, err := filepath.Glob(filepath.Join(conv.shadersDir, "*"))
	if len(shaders) == 0 || err != nil {
		return fmt.Errorf("failed to list shaders in %q: %w", conv.shadersDir, err)
	}

	sort.Strings(shaders)

	var workers Workers

	type ShaderResult struct {
		Path    string
		Shaders []driver.ShaderSources
		Error   error
	}
	shaderResults := make([]ShaderResult, len(shaders))

	for i, shaderPath := range shaders {
		i, shaderPath := i, shaderPath

		switch filepath.Ext(shaderPath) {
		case ".vert", ".frag":
			workers.Go(func() {
				shaders, err := conv.Shader(shaderPath)
				shaderResults[i] = ShaderResult{
					Path:    shaderPath,
					Shaders: shaders,
					Error:   err,
				}
			})
		case ".comp":
			workers.Go(func() {
				shaders, err := conv.ComputeShader(shaderPath)
				shaderResults[i] = ShaderResult{
					Path:    shaderPath,
					Shaders: shaders,
					Error:   err,
				}
			})
		default:
			continue
		}
	}

	workers.Wait()

	var allErrors string
	for _, r := range shaderResults {
		if r.Error != nil {
			if len(allErrors) > 0 {
				allErrors += "\n\n"
			}
			allErrors += "--- " + r.Path + " --- \n\n" + r.Error.Error() + "\n"
		}
	}
	if len(allErrors) > 0 {
		return errors.New(allErrors)
	}

	fmt.Fprintf(out, "// Code generated by build.go. DO NOT EDIT.\n\n")
	fmt.Fprintf(out, "package %s\n\n", conv.packageName)
	fmt.Fprintf(out, "import %q\n\n", "github.com/cybriq/giocore/gpu/internal/driver")

	fmt.Fprintf(out, "var (\n")

	for _, r := range shaderResults {
		if len(r.Shaders) == 0 {
			continue
		}

		name := filepath.Base(r.Path)
		name = strings.ReplaceAll(name, ".", "_")
		fmt.Fprintf(out, "\tshader_%s = ", name)

		multiVariant := len(r.Shaders) > 1
		if multiVariant {
			fmt.Fprintf(out, "[...]driver.ShaderSources{\n")
		}

		for _, src := range r.Shaders {
			fmt.Fprintf(out, "driver.ShaderSources{\n")
			fmt.Fprintf(out, "Name: %#v,\n", src.Name)
			if len(src.Inputs) > 0 {
				fmt.Fprintf(out, "Inputs: %#v,\n", src.Inputs)
			}
			if u := src.Uniforms; len(u.Blocks) > 0 {
				fmt.Fprintf(out, "Uniforms: driver.UniformsReflection{\n")
				fmt.Fprintf(out, "Blocks: %#v,\n", u.Blocks)
				fmt.Fprintf(out, "Locations: %#v,\n", u.Locations)
				fmt.Fprintf(out, "Size: %d,\n", u.Size)
				fmt.Fprintf(out, "},\n")
			}
			if len(src.Textures) > 0 {
				fmt.Fprintf(out, "Textures: %#v,\n", src.Textures)
			}
			if len(src.GLSL100ES) > 0 {
				fmt.Fprintf(out, "GLSL100ES: `%s`,\n", src.GLSL100ES)
			}
			if len(src.GLSL300ES) > 0 {
				fmt.Fprintf(out, "GLSL300ES: `%s`,\n", src.GLSL300ES)
			}
			if len(src.GLSL310ES) > 0 {
				fmt.Fprintf(out, "GLSL310ES: `%s`,\n", src.GLSL310ES)
			}
			if len(src.GLSL130) > 0 {
				fmt.Fprintf(out, "GLSL130: `%s`,\n", src.GLSL130)
			}
			if len(src.GLSL150) > 0 {
				fmt.Fprintf(out, "GLSL150: `%s`,\n", src.GLSL150)
			}
			if len(src.HLSL) > 0 {
				fmt.Fprintf(out, "HLSL: %q,\n", src.HLSL)
			}
			fmt.Fprintf(out, "}")
			if multiVariant {
				fmt.Fprintf(out, ",")
			}
			fmt.Fprintf(out, "\n")
		}
		if multiVariant {
			fmt.Fprintf(out, "}\n")
		}
	}
	fmt.Fprintf(out, ")\n")

	return nil
}

func (conv *Converter) Shader(shaderPath string) ([]driver.ShaderSources, error) {
	type Variant struct {
		FetchColorExpr string
		Header         string
	}
	variantArgs := [...]Variant{
		{
			FetchColorExpr: `_color.color`,
			Header:         `layout(binding=0) uniform Color { vec4 color; } _color;`,
		},
		{
			FetchColorExpr: `mix(_gradient.color1, _gradient.color2, clamp(vUV.x, 0.0, 1.0))`,
			Header:         `layout(binding=0) uniform Gradient { vec4 color1; vec4 color2; } _gradient;`,
		},
		{
			FetchColorExpr: `texture(tex, vUV)`,
			Header:         `layout(binding=0) uniform sampler2D tex;`,
		},
	}

	shaderTemplate, err := template.ParseFiles(shaderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", shaderPath, err)
	}

	var variants []driver.ShaderSources
	for i, variantArg := range variantArgs {
		variantName := strconv.Itoa(i)
		var buf bytes.Buffer
		err := shaderTemplate.Execute(&buf, variantArg)
		if err != nil {
			return nil, fmt.Errorf("failed to execute template %q with %#v: %w", shaderPath, variantArg, err)
		}

		var sources driver.ShaderSources
		sources.Name = filepath.Base(shaderPath)

		// Ignore error; some shaders are not meant to run in GLSL 1.00.
		sources.GLSL100ES, _, _ = conv.ShaderVariant(shaderPath, variantName, buf.Bytes(), "es", "100")

		var metadata Metadata
		sources.GLSL300ES, metadata, err = conv.ShaderVariant(shaderPath, variantName, buf.Bytes(), "es", "300")
		if err != nil {
			return nil, fmt.Errorf("failed to convert GLSL300ES:\n%w", err)
		}

		sources.GLSL130, _, err = conv.ShaderVariant(shaderPath, variantName, buf.Bytes(), "glsl", "130")
		if err != nil {
			return nil, fmt.Errorf("failed to convert GLSL130:\n%w", err)
		}

		hlsl, _, err := conv.ShaderVariant(shaderPath, variantName, buf.Bytes(), "hlsl", "40")
		if err != nil {
			return nil, fmt.Errorf("failed to convert HLSL:\n%w", err)
		}
		sources.HLSL, err = conv.fxc.Compile(shaderPath, variantName, []byte(hlsl), "main", "4_0_level_9_1")
		if err != nil {
			// Attempt shader model 4.0. Only the gpu/headless
			// test shaders use features not supported by level
			// 9.1.
			sources.HLSL, err = conv.fxc.Compile(shaderPath, variantName, []byte(hlsl), "main", "4_0")
			if err != nil {
				return nil, fmt.Errorf("failed to compile HLSL: %w", err)
			}
		}

		sources.GLSL150, _, err = conv.ShaderVariant(shaderPath, variantName, buf.Bytes(), "glsl", "150")
		if err != nil {
			return nil, fmt.Errorf("failed to convert GLSL150:\n%w", err)
		}

		sources.Uniforms = metadata.Uniforms
		sources.Inputs = metadata.Inputs
		sources.Textures = metadata.Textures

		variants = append(variants, sources)
	}

	// If the shader don't use the variant arguments, output only a single version.
	if variants[0].GLSL100ES == variants[1].GLSL100ES {
		variants = variants[:1]
	}

	return variants, nil
}

func (conv *Converter) ShaderVariant(shaderPath, variant string, src []byte, lang, profile string) (string, Metadata, error) {
	spirv, err := conv.glslvalidator.Convert(shaderPath, variant, lang == "hlsl", src)
	if err != nil {
		return "", Metadata{}, fmt.Errorf("failed to generate SPIR-V for %q: %w", shaderPath, err)
	}

	dst, err := conv.spirv.Convert(shaderPath, variant, spirv, lang, profile)
	if err != nil {
		return "", Metadata{}, fmt.Errorf("failed to convert shader %q: %w", shaderPath, err)
	}

	meta, err := conv.spirv.Metadata(shaderPath, variant, spirv)
	if err != nil {
		return "", Metadata{}, fmt.Errorf("failed to extract metadata for shader %q: %w", shaderPath, err)
	}

	return dst, meta, nil
}

func (conv *Converter) ComputeShader(shaderPath string) ([]driver.ShaderSources, error) {
	shader, err := ioutil.ReadFile(shaderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load shader %q: %w", shaderPath, err)
	}

	spirv, err := conv.glslvalidator.Convert(shaderPath, "", false, shader)
	if err != nil {
		return nil, fmt.Errorf("failed to convert compute shader %q: %w", shaderPath, err)
	}

	var sources driver.ShaderSources
	sources.Name = filepath.Base(shaderPath)

	sources.GLSL310ES, err = conv.spirv.Convert(shaderPath, "", spirv, "es", "310")
	if err != nil {
		return nil, fmt.Errorf("failed to convert es compute shader %q: %w", shaderPath, err)
	}
	sources.GLSL310ES = unixLineEnding(sources.GLSL310ES)

	hlslSource, err := conv.spirv.Convert(shaderPath, "", spirv, "hlsl", "50")
	if err != nil {
		return nil, fmt.Errorf("failed to convert hlsl compute shader %q: %w", shaderPath, err)
	}

	dxil, err := conv.fxc.Compile(shaderPath, "0", []byte(hlslSource), "main", "5_0")
	if err != nil {
		return nil, fmt.Errorf("failed to compile hlsl compute shader %q: %w", shaderPath, err)
	}
	if conv.directCompute {
		sources.HLSL = dxil
	}

	return []driver.ShaderSources{sources}, nil
}

// Workers implements wait group with synchronous logging.
type Workers struct {
	running sync.WaitGroup
}

func (lg *Workers) Go(fn func()) {
	lg.running.Add(1)
	go func() {
		defer lg.running.Done()
		fn()
	}()
}

func (lg *Workers) Wait() {
	lg.running.Wait()
}

func unixLineEnding(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
