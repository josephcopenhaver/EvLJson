package EvLJson

import (
	"bytes"
	"log"
	"testing"
)

var BENCHMARK_BYTES []byte

func init() {
	BENCHMARK_BYTES = []byte(STR_OBSFUCATED_BENCHMARK_BASIS)
}

func BenchmarkParseWithoutCallbacks(b *testing.B) {
	var err error

	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(BENCHMARK_BYTES)
		evLJsonParser := NewParser()
		if err = evLJsonParser.Parse(reader); err == nil {
			continue
		}
		log.Fatal(err)
	}
}
