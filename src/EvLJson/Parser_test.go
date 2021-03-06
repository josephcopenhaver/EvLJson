package EvLJson

import (
	"bytes"
	"encoding/hex"
	"io"
	"log"
	"testing"
)

const (
	LOG_STMT_FMT          = "%s\n"
	TEST_DATA_BUFFER_SIZE = 1024
)

var BENCHMARK_BYTES []byte

func init() {
	BENCHMARK_BYTES = []byte(STR_OBSFUCATED_BENCHMARK_BASIS)
}

func BenchmarkParseWithCallbacks(b *testing.B) {
	var err error
	dataBuffer := make([]byte, TEST_DATA_BUFFER_SIZE)
	evLJsonParser := NewParser(dataBuffer, nil, 0)
	onData := func(parser *Parser, endOfData bool) {}

	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(BENCHMARK_BYTES)
		if err = evLJsonParser.Parse(reader, nil, onData); err == nil {
			evLJsonParser.Reset()
			continue
		}
		log.Fatal(err)
	}
}

func BenchmarkParseWithoutCallbacks(b *testing.B) {
	var err error
	dataBuffer := make([]byte, TEST_DATA_BUFFER_SIZE)
	evLJsonParser := NewParser(dataBuffer, nil, 0)

	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(BENCHMARK_BYTES)
		if err = evLJsonParser.Parse(reader, nil, nil); err == nil {
			evLJsonParser.Reset()
			continue
		}
		log.Fatal(err)
	}
}

func BenchmarkByteReader(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(BENCHMARK_BYTES)
		_, err = reader.ReadByte()
		for ; err == nil; _, err = reader.ReadByte() {
			// Do Nothing
		}
		if err == io.EOF {
			continue
		}
		log.Fatal(err)
	}
}

func parseStringAllowWhitespace(jsonString string) error {
	reader := bytes.NewReader([]byte(jsonString))
	evLJsonParser := NewParser(nil, nil, OPT_ALLOW_EXTRA_WHITESPACE)
	return evLJsonParser.Parse(reader, nil, nil)
}

func parseStringWithoutCallbacksOrOptions(jsonString string) error {
	reader := bytes.NewReader([]byte(jsonString))
	evLJsonParser := NewParser(nil, nil, 0)
	return evLJsonParser.Parse(reader, nil, nil)
}

func parseStringWithoutCallbacksTillEOF(jsonString string) error {
	reader := bytes.NewReader([]byte(jsonString))
	evLJsonParser := NewParser(nil, nil, OPT_PARSE_UNTIL_EOF)
	return evLJsonParser.Parse(reader, nil, nil)
}

func TestInvalidJsonEmpty(t *testing.T) {
	err := parseStringWithoutCallbacksOrOptions("")
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
		err := parseStringWithoutCallbacksOrOptions(str)
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
		"[0e0]",
		"[-0e0]",
		"[0e-0]",
		"[-0e-0]",
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
		err := parseStringWithoutCallbacksOrOptions(str)
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
		err := parseStringWithoutCallbacksOrOptions(str)
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
		"[1.2e0.]",
		"[1.2e1.]",
		"[1.2e.]",
		"[1.2e.0]",
		"[1.2e-0.]",
		"[1.2e-1.]",
		"[1.2e-.]",
		"[1.2e-.0]",
	}
	for _, str := range testCases {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacksOrOptions(str)
		if err == nil {
			t.FailNow()
		}
	}
}

func whitespaceTestCases() []string {
	return []string{
		" [ 0 ] ",
		"[ 0 ] ",
		" [0 ] ",
		" [ 0] ",
		" [ 0 ]",
		"[0 ] ",
		" [0] ",
		" [ 0]",
		" [0]",
		"[ 0]",
		"[0 ]",
		"[0] ",
		" { \"\" : 0 } ",
		"{ \"\" : 0 } ",
		" {\"\" : 0 } ",
		" { \"\": 0 } ",
		" { \"\" :0 } ",
		" { \"\" : 0} ",
		" { \"\" : 0 }",
		" { \"\" : 0 } ",
		"{\"\" : 0 } ",
		" {\"\": 0 } ",
		" { \"\": 0 } ",
		" { \"\" :0} ",
		" { \"\" : 0}",
		"{\"\": 0 } ",
		" {\"\":0 } ",
		" { \"\": 0} ",
		" { \"\" :0}",
		"{\"\":0 } ",
		" {\"\":0} ",
		" { \"\": 0}",
		"{\"\":0} ",
		" {\"\":0}",
		" { \"a\" : 0 } ",
		"{ \"a\" : 0 } ",
		" {\"a\" : 0 } ",
		" { \"a\": 0 } ",
		" { \"a\" :0 } ",
		" { \"a\" : 0} ",
		" { \"a\" : 0 }",
		" { \"a\" : 0 } ",
		"{\"a\" : 0 } ",
		" {\"a\": 0 } ",
		" { \"a\": 0 } ",
		" { \"a\" :0} ",
		" { \"a\" : 0}",
		"{\"a\": 0 } ",
		" {\"a\":0 } ",
		" { \"a\": 0} ",
		" { \"a\" :0}",
		"{\"a\":0 } ",
		" {\"a\":0} ",
		" { \"a\": 0}",
		"{\"a\":0} ",
		" {\"a\":0}",
	}
}

func TestFailOnWhitepsace(t *testing.T) {
	for _, str := range whitespaceTestCases() {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringWithoutCallbacksTillEOF(str)
		if err == nil {
			t.FailNow()
		}
	}
}

func TestPassOnWhitepsace(t *testing.T) {
	for _, str := range whitespaceTestCases() {
		t.Logf(LOG_STMT_FMT, str)
		err := parseStringAllowWhitespace(str)
		if err != nil {
			t.FailNow()
		}
	}
}

func BenchmarkCapitolHexConversion(b *testing.B) {
	bytes := []byte{0}
	var err error

	for i := 0; i < b.N; i++ {
		if _, err = hex.Decode(bytes, []byte{'A', 'F'}); err == nil {
			continue
		}
		log.Fatal(err)
	}
}

func BenchmarkLowerHexConversion(b *testing.B) {
	bytes := []byte{0}
	var err error

	for i := 0; i < b.N; i++ {
		if _, err = hex.Decode(bytes, []byte{'a', 'f'}); err == nil {
			continue
		}
		log.Fatal(err)
	}
}

func BenchmarkSpeedupHexConversion(b *testing.B) {
	bytes := []byte{0}
	var err error
	low := byte('A')
	high := byte('F')

	for i := 0; i < b.N; i++ {
		if _, err = hex.Decode(bytes, []byte{low + ('a' - 'A'), high + ('a' - 'A')}); err == nil {
			continue
		}
		log.Fatal(err)
	}
}
