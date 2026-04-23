package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "bootstrap requires the Layer 2 workgroup slice; not yet implemented")
	os.Exit(1)
}
