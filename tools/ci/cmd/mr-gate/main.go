package main

import (
	"fmt"
	"os"

	"ci-tools/internal/gate"
)

func main() {
	if err := gate.CheckDescription(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println("MR description within limit.")
}
