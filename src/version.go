package main

import "fmt"

var (
	Version   = "0.0.0-dev"
	GitCommit = "unknown"
)

func printVersion() {
	fmt.Printf("backup version %s (%s)\n", Version, GitCommit)
}
