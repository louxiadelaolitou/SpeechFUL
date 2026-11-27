// recommendationlab emits a fixed, finite synthetic experiment to stdout.
// It intentionally accepts no database, HTTP, account, or input-file options.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/synthetic"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: recommendationlab (synthetic data only; no arguments)")
		os.Exit(2)
	}
	report, err := synthetic.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
