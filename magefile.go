//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	binaryName = "gitsyncer"
	buildDir   = "."
	cmdPath    = "./cmd/gitsyncer"
	ldflags    = "-s -w"
)

// Default target to run when none is specified
var Default = Build

// Build builds the gitsyncer binary
func Build() error {
	fmt.Println("Building gitsyncer...")
	return sh.RunWith(nil, "go", "build", "-ldflags", ldflags, "-o", filepath.Join(buildDir, binaryName), cmdPath)
}

// BuildAll builds for all supported platforms
func BuildAll() error {
	mg.Deps(BuildLinux, BuildDarwin, BuildWindows)
	return nil
}

// BuildLinux builds for Linux
func BuildLinux() error {
	fmt.Println("Building for Linux...")
	env := map[string]string{
		"GOOS":   "linux",
		"GOARCH": "amd64",
	}
	return sh.RunWith(env, "go", "build", "-ldflags", ldflags, "-o", filepath.Join(buildDir, binaryName+"-linux-amd64"), cmdPath)
}

// BuildDarwin builds for macOS
func BuildDarwin() error {
	fmt.Println("Building for macOS...")
	env := map[string]string{
		"GOOS":   "darwin",
		"GOARCH": "amd64",
	}
	if err := sh.RunWith(env, "go", "build", "-ldflags", ldflags, "-o", filepath.Join(buildDir, binaryName+"-darwin-amd64"), cmdPath); err != nil {
		return err
	}

	env["GOARCH"] = "arm64"
	return sh.RunWith(env, "go", "build", "-ldflags", ldflags, "-o", filepath.Join(buildDir, binaryName+"-darwin-arm64"), cmdPath)
}

// BuildWindows builds for Windows
func BuildWindows() error {
	fmt.Println("Building for Windows...")
	env := map[string]string{
		"GOOS":   "windows",
		"GOARCH": "amd64",
	}
	return sh.RunWith(env, "go", "build", "-ldflags", ldflags, "-o", filepath.Join(buildDir, binaryName+"-windows-amd64.exe"), cmdPath)
}

// Run builds and runs the gitsyncer binary
func Run() error {
	mg.Deps(Build)
	return sh.Run(filepath.Join(".", binaryName))
}

// Test runs tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.Run("go", "test", "./...")
}

// TestVerbose runs tests with verbose output
func TestVerbose() error {
	fmt.Println("Running tests (verbose)...")
	return sh.Run("go", "test", "-v", "./...")
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")
	files, err := filepath.Glob(binaryName + "*")
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove %s: %v\n", f, err)
		}
	}
	return nil
}

// ModTidy tidies go modules
func ModTidy() error {
	fmt.Println("Tidying go modules...")
	return sh.Run("go", "mod", "tidy")
}

// Fmt formats Go code
func Fmt() error {
	fmt.Println("Formatting Go code...")
	return sh.Run("go", "fmt", "./...")
}

// Vet runs go vet
func Vet() error {
	fmt.Println("Running go vet...")
	return sh.Run("go", "vet", "./...")
}

// Lint runs golangci-lint
func Lint() error {
	fmt.Println("Running golangci-lint...")
	return sh.Run("golangci-lint", "run")
}

// Install installs gitsyncer to $GOPATH/bin
func Install() error {
	fmt.Println("Installing gitsyncer...")
	return sh.Run("go", "install", cmdPath)
}

// Version shows version
func Version() error {
	mg.Deps(Build)
	return sh.Run(filepath.Join(".", binaryName), "version")
}
