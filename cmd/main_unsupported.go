//go:build !linux
// +build !linux

package main

import "fmt"

func main() {
	fmt.Println("❌ This tool is only supported on Linux.")
}
