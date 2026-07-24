// Command swagger2yaml converts a swagger JSON file to YAML.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	in := flag.String("in", "", "input swagger JSON path")
	out := flag.String("out", "", "output swagger YAML path")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: swagger2yaml -in <file.json> -out <file.yaml>")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fail(err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		fail(err)
	}

	yamlBytes, err := yaml.Marshal(doc)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, yamlBytes, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
