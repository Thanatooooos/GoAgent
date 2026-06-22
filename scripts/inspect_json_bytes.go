package main

import (
	"fmt"
	"os"
)

func main() {
	raw, _ := os.ReadFile("testdata/evals/rewrite/v2_24_semantic_judge.json")
	idx := 0
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == '{' && i+8 < len(raw) && string(raw[i:i+8]) == `{"suite"` {
			idx = i
			break
		}
	}
	s := raw[idx:]
	start := 1235
	end := 1270
	if end > len(s) {
		end = len(s)
	}
	fmt.Printf("bytes[%d:%d] = %q\n", start, end, string(s[start:end]))
	for i := start; i < end; i++ {
		fmt.Printf("%d:%c(%02x) ", i, s[i], s[i])
	}
	fmt.Println()
}
