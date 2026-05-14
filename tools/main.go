package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	printHelpAndDie := func() {
		fmt.Println(`
Usage: tools [command]`)
		os.Exit(1)
	}
	if len(os.Args) < 2 {
		printHelpAndDie()
	}

	// Sub-commands.
	matchCmd := flag.NewFlagSet("match", flag.ExitOnError)
	switch os.Args[1] {
	case "match":
		err := matchCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println("Error parsing flags:", err)
			os.Exit(1)
		}
		if matchCmd.NArg() != 1 {
			fmt.Println("Usage: match [file]")
			os.Exit(1)
		}
		file := matchCmd.Arg(0)

		// For debugging purposes, allow overriding these.
		goos, found := os.LookupEnv("GOHOSTOS")
		if !found {
			goos = runtime.GOOS
		}
		goarch, found := os.LookupEnv("GOARCH")
		if !found {
			goarch = runtime.GOARCH
		}
		ctx := build.Context{
			GOOS:   goos,
			GOARCH: goarch,
		}
		abs, err := filepath.Abs(file)
		if err != nil {
			panic(err)
		}
		match, err := ctx.MatchFile(filepath.Dir(abs), filepath.Base(abs))
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		if match {
			os.Exit(0)
		}
		os.Exit(1)
	default:
		printHelpAndDie()
	}
}
