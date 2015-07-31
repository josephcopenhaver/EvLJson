package EvLJson

import (
	"bytes"
	"log"
	"testing"
)

const LOG_STMT_FMT = "%s\n"

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

func parseStringWithoutCallbacks(jsonString string) error {
	reader := bytes.NewReader([]byte(jsonString))
	evLJsonParser := NewParser()
	return evLJsonParser.Parse(reader)
}

func TestInvalidJsonEmpty(t *testing.T) {
	err := parseStringWithoutCallbacks("")
	if err == nil {
		t.Fail()
	}
}

func TestStartObjects(t *testing.T) {
	testCases := []string{
		"0",
		"1",
		"null",
		"true",
		"false",
	}
	for _, str := range testCases {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacks(str)
		if err == nil {
			t.FailNow()
		}
	}
}

func TestStrangeValidJson(t *testing.T) {
	/*

		Unexpected format features:
		1. exponent coefficients with any number of leading zeros
		2. negative zero ( makes sense for floats / wanting easy parsing )
		    but it is not clear why this is supported in exponent coefficients
		3. empty dict keys

	*/
	testCases := []string{
		"[-0]",
		"[0.0e0]",
		"[0.0e00]",
		"[0.0e001]",
		"[-0.0e0]",
		"[-0.0e00]",
		"[-0.0e001]",
		"[0.0e-0]",
		"[0.0e-00]",
		"[0.0e-001]",
		"[-0.0e-0]",
		"[-0.0e-00]",
		"[-0.0e-001]",
		"{\"\":-0}",
		"{\"\":0.0e0}",
		"{\"\":0.0e00}",
		"{\"\":0.0e001}",
		"{\"\":-0.0e0}",
		"{\"\":-0.0e00}",
		"{\"\":-0.0e001}",
		"{\"\":0.0e-0}",
		"{\"\":0.0e-00}",
		"{\"\":0.0e-001}",
		"{\"\":-0.0e-0}",
		"{\"\":-0.0e-00}",
		"{\"\":-0.0e-001}",
		"{\"\":null}",
		"{\"\":0}",
		"{\"\":1}",
		"{\"\":true}",
		"{\"\":false}",
		"{\"\":null}",
	}
	for _, str := range testCases {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacks(str)
		if err != nil {
			t.FailNow()
		}
	}
}

func TestNormalValidJson(t *testing.T) {
	testCases := []string{
		"[0]",
		"[1]",
		"[true]",
		"[false]",
		"[null]",
		"{\"a\":0}",
		"{\"a\":1}",
		"{\"a\":true}",
		"{\"a\":false}",
		"{\"a\":null}",
	}
	for _, str := range testCases {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacks(str)
		if err != nil {
			t.FailNow()
		}
	}
}

func TestBadJson(t *testing.T) {
	testCases := []string{
		"[00]",
		"[-00]",
		"[01]",
		"[-01]",
		"[0",
		"{\"\":0",
		"[0,",
		"{\"\":0,",
		"[0,]",
		"{\"\":0,}",
		"{\"\":0,\"\"}",
		"{\"\":0,\"a\"}",
		"{\"\":0,\"\":}",
		"{\"\":0,\"a\":}",
		"{\"\":0,\"\":,}",
		"{\"\":0,\"a\":,}",
	}
	for _, str := range testCases {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacks(str)
		if err == nil {
			t.FailNow()
		}
	}
}
