// Command genfi generates internal/geogate/fi_prefixes.go from the
// RIPE NCC delegated statistics file (all Finnish IPv4/IPv6 space).
//
// Usage:
//
//	go run ./scripts/genfi                      # download from RIPE
//	go run ./scripts/genfi -file delegated.txt  # use a local copy
//	go run ./scripts/genfi -o internal/geogate/fi_prefixes.go
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultURL = "https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-extended-latest"

func main() {
	var (
		url    = flag.String("url", defaultURL, "delegated stats URL")
		file   = flag.String("file", "", "local delegated stats file (.gz supported)")
		out    = flag.String("o", "internal/geogate/fi_prefixes.go", "output file")
		verify = flag.Bool("check", false, "verify output instead of writing (for CI)")
	)
	flag.Parse()

	var r io.ReadCloser
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		r = f
		if strings.HasSuffix(*file, ".gz") {
			gz, err := gzip.NewReader(f)
			if err != nil {
				log.Fatal(err)
			}
			defer gz.Close()
			r = gz
		}
	} else {
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Get(*url)
		if err != nil {
			log.Fatalf("download %s: %v", *url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("download %s: status %s", *url, resp.Status)
		}
		r = resp.Body
	}

	var v4, v6 []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Split(strings.TrimSpace(scanner.Text()), "|")
		if len(fields) < 5 {
			continue
		}
		if fields[0] == "#" || fields[3] == "*" {
			continue
		}
		if fields[1] != "FI" {
			continue
		}
		switch fields[2] {
		case "ipv4":
			start := net.ParseIP(fields[3]).To4()
			if start == nil {
				continue
			}
			count, err := strconv.Atoi(fields[4])
			if err != nil {
				continue
			}
			v4 = append(v4, splitRange(start, count)...)
		case "ipv6":
			prefix := net.ParseIP(fields[3])
			if prefix == nil {
				continue
			}
			bits, err := strconv.Atoi(fields[4])
			if err != nil {
				continue
			}
			v6 = append(v6, prefix.String()+"/"+strconv.Itoa(bits))
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	sort.Strings(v4)
	sort.Strings(v6)

	var sb strings.Builder
	fmt.Fprintf(&sb, `package geogate

// Code generated from RIPE NCC delegated statistics. DO NOT EDIT.
// Regenerate with: make geo-update
//
// Source: %s
// Country: FI (Finland), IPv4 prefixes: %d, IPv6 prefixes: %d
// Generated: %s

var fiPrefixes = []string{
`, *url, len(v4), len(v6), time.Now().UTC().Format(time.RFC3339))
	for _, p := range v4 {
		fmt.Fprintf(&sb, "\t%q,\n", p)
	}
	for _, p := range v6 {
		fmt.Fprintf(&sb, "\t%q,\n", p)
	}
	sb.WriteString("}\n")

	if *verify {
		fmt.Printf("%d IPv4 + %d IPv6 prefixes\n", len(v4), len(v6))
		return
	}
	if err := os.WriteFile(*out, []byte(sb.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d IPv4 + %d IPv6 prefixes)\n", *out, len(v4), len(v6))
}

// splitRange converts an (start, count) IPv4 allocation into CIDR blocks.
func splitRange(start net.IP, count int) []string {
	var out []string
	u := binary.BigEndian.Uint32(start.To4())
	remaining := uint32(count)
	for remaining > 0 {
		size := uint32(1)
		for size*2 <= remaining {
			size *= 2
		}
		for u%size != 0 {
			size /= 2
		}
		bits := 32
		for s := size; s > 1; s /= 2 {
			bits--
		}
		base := make(net.IP, 4)
		binary.BigEndian.PutUint32(base, u)
		out = append(out, fmt.Sprintf("%s/%d", base.String(), bits))
		u += size
		remaining -= size
	}
	return out
}
