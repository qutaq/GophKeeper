// Command checkcoverage fails when total coverage from `go tool cover -func` is below threshold.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	threshold := flag.Float64("threshold", 70, "minimum total coverage percent")
	funcOut := flag.String("func", "", "path to `go tool cover -func` output (or stdin if empty)")
	flag.Parse()

	var sc *bufio.Scanner
	if *funcOut == "" {
		sc = bufio.NewScanner(os.Stdin)
	} else {
		f, err := os.Open(*funcOut)
		if err != nil {
			fail("open: %v", err)
		}
		defer f.Close()
		sc = bufio.NewScanner(f)
	}

	var last string
	for sc.Scan() {
		last = sc.Text()
	}
	if err := sc.Err(); err != nil {
		fail("read: %v", err)
	}

	fields := strings.Fields(last)
	if len(fields) < 3 || !strings.HasPrefix(fields[0], "total:") {
		fail("unexpected cover summary: %q", last)
	}
	total, err := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-1], "%"), 64)
	if err != nil {
		fail("parse total: %v", err)
	}

	fmt.Printf("Total coverage: %.1f%% (threshold %.1f%%)\n", total, *threshold)
	if total < *threshold {
		fail("coverage %.1f%% is below threshold %.1f%%", total, *threshold)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
