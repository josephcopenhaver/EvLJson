package EvLJson

import (
    "io/ioutil"
    "log"
    "net/http"
    "bytes"
    "testing"
)

var BENCHMARK_BYTES []byte


func init() {
    httpResponse, err := http.Get("http://127.0.0.1:8080")
    if err != nil {
        log.Fatal(err)
    }

    benchmarkBytes, err := ioutil.ReadAll(httpResponse.Body)

    if err != nil {
        log.Fatal(err)
    }

    BENCHMARK_BYTES = benchmarkBytes

    defer httpResponse.Body.Close()
}


func BenchmarkParseWithoutCallbacks(b *testing.B) {

    var err error

    for i := 0; i < b.N; i++ {
        reader := bytes.NewReader(BENCHMARK_BYTES)
        evLJsonParser := NewParser()
        err = evLJsonParser.Parse(reader)
    }

    if err != nil {
        log.Fatal(err)
    }
}