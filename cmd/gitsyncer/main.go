package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/paul/gitsyncer/internal/version"
)

func main() {
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "print version information")
	flag.BoolVar(&versionFlag, "v", false, "print version information (short)")
	flag.Parse()

	if versionFlag {
		fmt.Println(version.GetVersion())
		os.Exit(0)
	}

	// TODO: Implement main gitsyncer functionality
	fmt.Println("gitsyncer - Git repository synchronization tool")
	fmt.Println("Use --version to display version information")
}