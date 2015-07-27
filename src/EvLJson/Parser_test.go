package EvLJson

import (
    "io/ioutil"
    "log"
    "net/http"
    "bytes"
    "testing"
)


func BenchmarkParseWithoutCallbacks(b *testing.B) {

    httpResponse, err := http.Get("http://127.0.0.1:8080")

    if err != nil {
        log.Fatal(err)
    }

    benchmarkBytes, err := ioutil.ReadAll(httpResponse.Body)

    if err != nil {
        log.Fatal(err)
    }

    for i := 0; i < b.N; i++ {
        reader := bytes.NewReader(benchmarkBytes)
        evLJsonParser := NewParser()

        err = evLJsonParser.Parse(reader)

        if err != nil {
            log.Fatal(err)
        }
    }

    defer httpResponse.Body.Close()
}