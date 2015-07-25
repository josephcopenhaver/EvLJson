package main

import (
	"bufio"
	"io"
	"log"
	"net/http"
	//"fmt"  // DEBUG
)

/*

Lets assume that there is copper somewhere between the target and us:

-
http://stackoverflow.com/questions/2613734/maximum-packet-size-for-a-tcp-connection
-

The absolute limitation on TCP packet size is 64K (65535 bytes), but in practicality this is far larger than the size of any packet you will see, because the lower layers (e.g. ethernet) have lower packet sizes. The MTU (Maximum Transmission Unit) for Ethernet, for instance, is 1500 bytes.


default buffer size as of 7/24/2015:

-
http://golang.org/src/bufio/bufio.go?s=1648:1684#L51
-

const (
    defaultBufSize = 4096
)


instagram post size limits:
-
http://www.jennstrends.com/limits-on-instagram/
-
2200 characters, possibly 6 byte utf-8 encoding

*/

const TCP_HEADER_SIZE = 40
const BUFIO_READER_SIZE = 1500 - TCP_HEADER_SIZE // between ( 1024 = 2 ^ 10 ) and ( 2048 = 2 ^ 11 )
const LITERAL_BUFF_SIZE = 13200                  // 13200 ( 2200 * 6 )

const (
	OBJ_STR_NULL  = "null"
	OBJ_STR_TRUE  = "true"
	OBJ_STR_FALSE = "false"
)

type UnspecifiedJsonParseError struct{}

func (err UnspecifiedJsonParseError) Error() string {
	return "Unspecified json parser error"
}

var unspecifiedParseError = UnspecifiedJsonParseError{}

const (
	STATE_START = iota
	STATE_IN_NULL
	STATE_IN_TRUE
	STATE_IN_FALSE
	STATE_IN_ZERO_OR_DECIMAL_OR_EXPONENT_START
	STATE_IN_INT
	STATE_IN_DECIMAL_FRACTIONAL_START
	STATE_IN_DECIMAL_FRACTIONAL_END
	STATE_IN_EXPONENT_START
	STATE_IN_EXPONENT_END
	STATE_IN_STRING
	STATE_IN_DICT_EXPECT_FIRST_KEY_OR_END
	STATE_IN_DICT_EXPECT_KEY_VALUE_DELIM
	STATE_IN_DICT_EXPECT_VALUE
	STATE_IN_DICT_EXPECT_ENTRY_DELIM_OR_END
	STATE_IN_DICT_EXPECT_KEY
	STATE_IN_ARRAY_EXPECT_FIRST_ENTRY_OR_END
	STATE_IN_ARRAY_EXPECT_DELIM_OR_END
	STATE_IN_ARRAY_EXPECT_ENTRY
	STATE_END
)

var PARSER_STATE_ACTION_LOOKUP = []func(p *EvLParser, b byte) bool{
	handleStart,
	handleNull,
	handleTrue,
	handleFalse,
	handleZeroOrDecimalOrExponentStart,
	handleInt,
	handleDecimalFractionalStart,
	handleDecimalFractionalEnd,
	handleExponentStart,
	handleExponentEnd,
	handleString,
	handleDictExpectFirstKeyOrEnd,
	handleDictExpectKeyValueDelim,
	handleDictExpectValue,
	handleDictExpectEntryDelimOrEnd,
	handleDictExpectKey,
	handleArrayExpectFirstEntryOrEnd,
	handleArrayExpectDelimOrEnd,
	handleArrayExpectEntry,
	handleEnd,
}

func isCharWhitespace(b byte) bool {
	switch b {
	case 0x20: // SPACE
		fallthrough
	case 0x09: // TAB
		fallthrough
	case 0x0A: // LF
		fallthrough
	case 0x0D: // CR
		return true
	default:
		return false
	}
}

func pushState(p *EvLParser, newState uint8) {
	p.stateStack = append(p.stateStack, p.state)
	p.state = newState
}

func popState(p *EvLParser) {
	// TODO: experiment with removing the `newMaxIdx` var if this function fails to inline
	newMaxIdx := len(p.stateStack) - 1
	p.state, p.stateStack = p.stateStack[newMaxIdx], p.stateStack[:newMaxIdx]
}

func isNewValue(p *EvLParser, b byte) bool {
	switch b {
	case '0':
		pushState(p, STATE_IN_ZERO_OR_DECIMAL_OR_EXPONENT_START)
		return true
	case '[':
		pushState(p, STATE_IN_ARRAY_EXPECT_FIRST_ENTRY_OR_END)
		return true
	case '{':
		pushState(p, STATE_IN_DICT_EXPECT_FIRST_KEY_OR_END)
		return true
	case OBJ_STR_NULL[0]:
		pushState(p, STATE_IN_NULL)
		return true
	case OBJ_STR_FALSE[0]:
		pushState(p, STATE_IN_FALSE)
		return true
	case OBJ_STR_TRUE[0]:
		pushState(p, STATE_IN_TRUE)
		return true
	case '"':
		pushState(p, STATE_IN_STRING)
		return true
	case '-':
		pushState(p, STATE_IN_INT)
		return true
	default:
		if b >= '1' || b <= '9' {
			pushState(p, STATE_IN_INT)
			return true
		}
		return false
	}
}

func handleStart(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	p.state = STATE_END
	if isNewValue(p, b) {
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleNull(p *EvLParser, b byte) bool {
	if b == OBJ_STR_NULL[p.literalStateIndex] {
		p.literalStateIndex++
		if p.literalStateIndex == uint8(len(OBJ_STR_NULL)) {
			p.literalStateIndex = 1
			popState(p)
		}
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleTrue(p *EvLParser, b byte) bool {
	if b == OBJ_STR_TRUE[p.literalStateIndex] {
		p.literalStateIndex++
		if p.literalStateIndex == uint8(len(OBJ_STR_TRUE)) {
			p.literalStateIndex = 1
			popState(p)
		}
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleFalse(p *EvLParser, b byte) bool {
	if b == OBJ_STR_FALSE[p.literalStateIndex] {
		p.literalStateIndex++
		if p.literalStateIndex == uint8(len(OBJ_STR_FALSE)) {
			p.literalStateIndex = 1
			popState(p)
		}
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleZeroOrDecimalOrExponentStart(p *EvLParser, b byte) bool {
	switch b {
	case '.':
		// TODO: negotiate type changing features
		p.state = STATE_IN_DECIMAL_FRACTIONAL_START
		return true
	case 'e':
		// TODO: negotiate type changing features
		p.state = STATE_IN_EXPONENT_END
		return true
	default:
		popState(p)
		return false
	}
}

func handleInt(p *EvLParser, b byte) bool {
	if b >= '0' && b <= '9' {
		return true
	}
	switch b {
	case '.':
		// TODO: negotiate type changing features
		p.state = STATE_IN_DECIMAL_FRACTIONAL_START
		return true
	case 'e':
		// TODO: negotiate type changing features
		p.state = STATE_IN_EXPONENT_START
		return true
	}
	popState(p)
	return false
}

func handleDecimalFractionalStart(p *EvLParser, b byte) bool {
	if b >= '0' && b <= '9' {
		p.state = STATE_IN_DECIMAL_FRACTIONAL_END
		return true
	}
	popState(p)
	return false
}

func handleDecimalFractionalEnd(p *EvLParser, b byte) bool {
	switch {
	case b == 'e':
		// TODO: negotiate type changing features
		p.state = STATE_IN_EXPONENT_START
		return true
	case b >= '0' && b <= '9':
		return true
	}
	popState(p)
	return false
}

func handleExponentStart(p *EvLParser, b byte) bool {
	if b >= '0' && b <= '9' {
		p.state = STATE_IN_EXPONENT_END
		return true
	}
	if b == '-' {
		p.state = STATE_IN_EXPONENT_END
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleExponentEnd(p *EvLParser, b byte) bool {
	if b >= '0' && b <= '9' {
		p.state = STATE_IN_EXPONENT_END
		return true
	}
	popState(p)
	return false
}

func handleString(p *EvLParser, b byte) bool {
	if p.stringHexDigitIndex > 0 {
		p.stringHexDigitIndex++
		switch {
		case b >= '0' && b <= '9':
			break
		case b >= 'a' && b <= 'f':
			break
		case b >= 'A' && b <= 'F':
			break
		default:
			p.err = unspecifiedParseError
			return true
		}
		if p.stringHexDigitIndex == 5 {
			p.stringHexDigitIndex = 0
		}
		return true
	}

	switch b {
	case 'b':
		fallthrough
	case 'f':
		fallthrough
	case 'n':
		fallthrough
	case 'r':
		fallthrough
	case 't':
		fallthrough
	case '/':
		// allowed to be escaped, no special implications
		p.reverseSolidusParity = false
		return true
	case '\\':
		// reverse solidus (escape) parity adjusted
		p.reverseSolidusParity = !p.reverseSolidusParity
		return true
	case '"':
		if !p.reverseSolidusParity {
			// end of string
			popState(p)
			return true
		}
		p.reverseSolidusParity = false
		return true
	case 'u':
		if p.reverseSolidusParity {
			p.reverseSolidusParity = false
			p.stringHexDigitIndex = 1
		}
		return true
	default:
		if p.reverseSolidusParity {
			p.err = unspecifiedParseError
		}
		return true
	}
}

func handleDictExpectFirstKeyOrEnd(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	switch b {
	case '"':
		p.state = STATE_IN_DICT_EXPECT_KEY_VALUE_DELIM
		pushState(p, STATE_IN_STRING)
		return true
	case '}':
		popState(p)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleDictExpectKeyValueDelim(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	if b == ':' {
		p.state = STATE_IN_DICT_EXPECT_VALUE
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleDictExpectValue(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	if isNewValue(p, b) {
		p.stateStack[len(p.stateStack)-1] = STATE_IN_DICT_EXPECT_ENTRY_DELIM_OR_END // TODO: fix impl (HACK)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleDictExpectEntryDelimOrEnd(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	switch b {
	case ',':
		p.state = STATE_IN_DICT_EXPECT_KEY
		return true
	case '}':
		popState(p)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleDictExpectKey(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	if b == '"' {
		p.state = STATE_IN_DICT_EXPECT_KEY_VALUE_DELIM
		pushState(p, STATE_IN_STRING)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleArrayExpectFirstEntryOrEnd(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	switch {
	case b == ']':
		popState(p)
		return true
	case isNewValue(p, b):
		p.stateStack[len(p.stateStack)-1] = STATE_IN_ARRAY_EXPECT_DELIM_OR_END // TODO: fix impl (HACK)
		return true
	default:
		p.err = unspecifiedParseError
		return true
	}
}

func handleArrayExpectDelimOrEnd(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	switch b {
	case ',':
		p.state = STATE_IN_ARRAY_EXPECT_ENTRY
		return true
	case ']':
		popState(p)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleArrayExpectEntry(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	if isNewValue(p, b) {
		p.stateStack[len(p.stateStack)-1] = STATE_IN_ARRAY_EXPECT_DELIM_OR_END // TODO: fix impl (HACK)
		return true
	}
	p.err = unspecifiedParseError
	return true
}

func handleEnd(p *EvLParser, b byte) bool {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return true
		}
	}
	p.err = unspecifiedParseError
	return true
}

func (p *EvLParser) Parse(bufferedReader *bufio.Reader) error {
	singleByte, err := bufferedReader.ReadByte()
	if err != nil {
		return err
	}

	/*
	   debugOldState := uint8(STATE_START)
	   debugOldDepth := 0

	   debugFunc := func() {
	       newState := uint8(p.state)
	       newDepth := len(p.stateStack)
	       if debugOldState != newState || newDepth != debugOldDepth {
	           fmt.Printf("\n\nSTATE_SHIFT:  Depth(%d => %d)  State(%d => %d)\n\n", debugOldDepth, newDepth, debugOldState, newState)
	           debugOldState = newState
	           debugOldDepth = newDepth
	       }
	   }
	*/

	for err == nil {
		//fmt.Printf("%s", string(singleByte))  // DEBUG
		for !PARSER_STATE_ACTION_LOOKUP[p.state](p, singleByte) {
			//debugFunc()
		}
		//debugFunc()
		err = p.err
		if err == nil {
			singleByte, err = bufferedReader.ReadByte()
		}
	}

	/*
	   if p.state != STATE_END && err != nil {
	       fmt.Printf("\n\n")
	       fmt.Printf("End of stream while in state: %d\n", p.state)
	       fmt.Printf("Stack size: %d\n", len(p.stateStack))
	   }
	*/

	if p.state != STATE_END || err != io.EOF {
		return err
	}

	return nil
}

type EvLParser struct {
	state                      uint8
	literalStateIndex          uint8
	stringHexDigitIndex        uint8
	stateStack                 []uint8
	err                        error
	reverseSolidusParity       bool
	allowFreeContextWhitespace bool
	//literalBuffer [LITERAL_BUFF_SIZE]byte
}

const (
	OPT_IGNORE_EXTRA_KEYS              = 0x01
	OPT_EXPECT_NO_FREE_FORM_WHITESPACE = 0x02
)

// TODO: support config options
func NewEvLParser() EvLParser {
	return EvLParser{
		reverseSolidusParity: false,
		stringHexDigitIndex:  0,
		// minimum nominal case will require 3 state levels
		// TODO: allow for configuring this size parameter
		stateStack:        []uint8{0, 0, 0},
		literalStateIndex: 1,
		state:             STATE_START,
		err:               nil,
		allowFreeContextWhitespace: false,
	}
}

func main() {

	httpResponse, err := http.Get("http://127.0.0.1:8080")

	if err != nil {
		log.Fatal(err)
	}

	bufferedReader := bufio.NewReaderSize(httpResponse.Body, BUFIO_READER_SIZE)
	evLParser := NewEvLParser()

	err = evLParser.Parse(bufferedReader)

	if err != nil {
		log.Fatal(err)
	}

	defer httpResponse.Body.Close()
}
