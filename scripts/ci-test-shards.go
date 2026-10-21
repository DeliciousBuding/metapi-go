//go:build ignore

// ci-test-shards assigns the complete `go list ./...` output to deterministic
// longest-processing-time-first bins. It only selects packages, never tests.
// Usage (from the repository root):
//
//	go list ./... > packages.txt
//	go run scripts/ci-test-shards.go -shard 0 -total 4 < packages.txt
//
// Keep go list outside process substitution: its nonzero status must fail CI.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	modulePath      = "github.com/deliciousbuding/metapi-go"
	defaultWeightMS = int64(10_000) // Unmeasured/new packages are included, never skipped.
)

var packageSuffix = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_][A-Za-z0-9_.-]*)*$`)

func validPackage(name string) bool {
	return name == modulePath || strings.HasPrefix(name, modulePath+"/") &&
		packageSuffix.MatchString(strings.TrimPrefix(name, modulePath+"/"))
}

func weightsFrom(path string) (map[string]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	weights := map[string]int64{}
	scan := bufio.NewScanner(f)
	for line := 1; scan.Scan(); line++ {
		text := strings.TrimSpace(scan.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 || !packageSuffix.MatchString(fields[0]) {
			return nil, fmt.Errorf("weights line %d: expected package suffix and positive integer milliseconds", line)
		}
		weight, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || weight <= 0 || weight > 86_400_000 {
			return nil, fmt.Errorf("weights line %d: invalid milliseconds %q", line, fields[1])
		}
		name := modulePath + "/" + fields[0]
		if _, exists := weights[name]; exists {
			return nil, fmt.Errorf("weights line %d: duplicate package %s", line, name)
		}
		weights[name] = weight
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("read weights: %w", err)
	}
	if len(weights) == 0 {
		return nil, fmt.Errorf("weights contain no packages")
	}
	return weights, nil
}

func selectShard(input io.Reader, weights map[string]int64, shard, total int) ([]string, int64, int, error) {
	if total < 1 || shard < 0 || shard >= total {
		return nil, 0, 0, fmt.Errorf("require total >= 1 and 0 <= shard < total")
	}
	var packages []string
	seen := map[string]bool{}
	scan := bufio.NewScanner(input)
	for line := 1; scan.Scan(); line++ {
		name := scan.Text()
		if !validPackage(name) {
			return nil, 0, 0, fmt.Errorf("package line %d: invalid package %q", line, name)
		}
		if seen[name] {
			return nil, 0, 0, fmt.Errorf("package line %d: duplicate package %s", line, name)
		}
		seen[name] = true
		packages = append(packages, name)
	}
	if err := scan.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("read packages: %w", err)
	}
	if len(packages) < total {
		return nil, 0, 0, fmt.Errorf("got %d packages for %d shards; refusing an empty shard", len(packages), total)
	}
	weightOf := func(pkg string) int64 {
		if weight, exists := weights[pkg]; exists {
			return weight
		}
		return defaultWeightMS
	}
	sort.Slice(packages, func(i, j int) bool {
		if weightOf(packages[i]) != weightOf(packages[j]) {
			return weightOf(packages[i]) > weightOf(packages[j])
		}
		return packages[i] < packages[j]
	})
	bins := make([][]string, total)
	loads := make([]int64, total)
	for _, pkg := range packages {
		lightest := 0
		for i := 1; i < total; i++ {
			if loads[i] < loads[lightest] {
				lightest = i
			}
		}
		bins[lightest] = append(bins[lightest], pkg)
		loads[lightest] += weightOf(pkg)
	}
	// Validate the complete assignment before emitting even one package. Every
	// current input must appear exactly once, including packages absent from
	// the small timing table. Empty bins and unexpected output fail closed.
	assigned := map[string]bool{}
	for i, bin := range bins {
		if len(bin) == 0 {
			return nil, 0, 0, fmt.Errorf("shard %d selected no packages", i)
		}
		for _, pkg := range bin {
			if !seen[pkg] || assigned[pkg] {
				return nil, 0, 0, fmt.Errorf("invalid assignment for %s", pkg)
			}
			assigned[pkg] = true
		}
	}
	if len(assigned) != len(packages) {
		return nil, 0, 0, fmt.Errorf("assignment dropped packages")
	}
	sort.Strings(bins[shard])
	return bins[shard], loads[shard], len(packages), nil
}

func run() error {
	shard := flag.Int("shard", -1, "zero-based shard index")
	total := flag.Int("total", 0, "number of nonempty shards")
	weightsPath := flag.String("weights", "scripts/ci-test-shards.weights.tsv", "package weights in milliseconds")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	weights, err := weightsFrom(*weightsPath)
	if err != nil {
		return err
	}
	packages, load, count, err := selectShard(os.Stdin, weights, *shard, *total)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "shard %d/%d: %d of %d packages, estimated weight %.3fs (not wall time)\n", *shard, *total, len(packages), count, float64(load)/1000)
	for _, pkg := range packages {
		if _, err := fmt.Fprintln(os.Stdout, pkg); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ci-test-shards:", err)
		os.Exit(1)
	}
}
