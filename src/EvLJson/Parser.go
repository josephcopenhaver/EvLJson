package EvLJson

import (
	"io"
	//"fmt"  // DEBUG
	//"log"  // DEBUG
)

// REMINDER: leave data member [literalStateIndex] in as removing it
// would only result in the same # of operations, but using more jump
// table space which slows performance

// TODO: optimize out data member [allowFreeContextWhitespace]

const (
	SIG_NEXT_BYTE = iota
	SIG_REUSE_BYTE
	SIG_EOF
	SIG_ERR
)
const (
	VALUE_STR_NULL  = "null"
	VALUE_STR_TRUE  = "true"
	VALUE_STR_FALSE = "false"
)

type UnspecifiedJsonParseError struct{}

func (err UnspecifiedJsonParseError) Error() string {
	return "Unspecified json parser error"
}

var unspecifiedParseError = UnspecifiedJsonParseError{}

func signalUnspecifiedError(p *Parser) uint8 {
	p.err = unspecifiedParseError
	return SIG_ERR
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

func checkHexChar(p *Parser, b byte) uint8 {
	if b <= '9' {
		if b >= '0' {
			return SIG_NEXT_BYTE
		}
	} else {
		if b >= 'a' {
			if b <= 'f' {
				return SIG_NEXT_BYTE
			}
		} else if b >= 'A' && b <= 'F' {
			return SIG_NEXT_BYTE
		}
	}
	return signalUnspecifiedError(p)
}

func pushHandle(p *Parser, newHandle func(p *Parser, b byte) uint8) {
	p.handleStack = append(p.handleStack, p.handle)
	p.handle = newHandle
}

func popHandle(p *Parser) {
	newMaxIdx := len(p.handleStack) - 1
	p.handle, p.handleStack = p.handleStack[newMaxIdx], p.handleStack[:newMaxIdx]
}

func getNewValueHandle(b byte) func(p *Parser, b byte) uint8 {
	if b >= '1' && b <= '9' {
		return handleInt
	}
	switch b {
	case '0':
		return handleZeroOrDecimalOrExponentStart
	case '[':
		return handleArrayExpectFirstEntryOrEnd
	case '{':
		return handleDictExpectFirstKeyOrEnd
	case VALUE_STR_NULL[0]:
		return handleNull
	case VALUE_STR_FALSE[0]:
		return handleFalse
	case VALUE_STR_TRUE[0]:
		return handleTrue
	case '"':
		return handleString
	case '-':
		return handleZeroOrDecimalOrExponentNegativeStart
	default:
		return nil
	}
}

func handleStart(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	p.handle = handleEnd
	if b == '[' {
		pushHandle(p, handleArrayExpectFirstEntryOrEnd)
		return SIG_NEXT_BYTE
	}
	if b == '{' {
		pushHandle(p, handleDictExpectFirstKeyOrEnd)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleNull(p *Parser, b byte) uint8 {
	literalStateIndex := p.literalStateIndex
	if b == VALUE_STR_NULL[literalStateIndex] {
		literalStateIndex++
		if literalStateIndex != uint8(len(VALUE_STR_NULL)) {
			p.literalStateIndex = literalStateIndex
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleTrue(p *Parser, b byte) uint8 {
	literalStateIndex := p.literalStateIndex
	if b == VALUE_STR_TRUE[literalStateIndex] {
		literalStateIndex++
		if literalStateIndex != uint8(len(VALUE_STR_TRUE)) {
			p.literalStateIndex = literalStateIndex
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleFalse(p *Parser, b byte) uint8 {
	literalStateIndex := p.literalStateIndex
	if b == VALUE_STR_FALSE[literalStateIndex] {
		literalStateIndex++
		if literalStateIndex != uint8(len(VALUE_STR_FALSE)) {
			p.literalStateIndex = literalStateIndex
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleZeroOrDecimalOrExponentStart(p *Parser, b byte) uint8 {
	switch b {
	case '.':
		// TODO: negotiate type changing features
		p.handle = handleDecimalFractionalStart
		return SIG_NEXT_BYTE
	case 'e':
		// TODO: negotiate type changing features
		p.handle = handleExponentCoefficientStart
		return SIG_NEXT_BYTE
	default:
		popHandle(p)
		return SIG_REUSE_BYTE
	}
}

func handleInt(p *Parser, b byte) uint8 {
	if b >= '0' && b <= '9' {
		return SIG_NEXT_BYTE
	}
	switch b {
	case '.':
		// TODO: negotiate type changing features
		p.handle = handleDecimalFractionalStart
		return SIG_NEXT_BYTE
	case 'e':
		// TODO: negotiate type changing features
		p.handle = handleExponentCoefficientStart
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleZeroOrDecimalOrExponentNegativeStart(p *Parser, b byte) uint8 {
	if b == '0' {
		p.handle = handleZeroOrDecimalOrExponentStart
		return SIG_NEXT_BYTE
	}
	if b >= '1' && b <= '9' {
		p.handle = handleInt
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDecimalFractionalStart(p *Parser, b byte) uint8 {
	if b >= '0' && b <= '9' {
		p.handle = handleDecimalFractionalEnd
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleDecimalFractionalEnd(p *Parser, b byte) uint8 {
	switch {
	case b >= '0' && b <= '9':
		return SIG_NEXT_BYTE
	case b == 'e':
		// TODO: negotiate type changing features
		p.handle = handleExponentCoefficientStart
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleExponentCoefficientStart(p *Parser, b byte) uint8 {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return SIG_NEXT_BYTE
	}
	if b == '0' {
		p.handle = handleExponentCoefficientLeadingZero
		return SIG_NEXT_BYTE
	}
	if b == '-' {
		p.handle = handleExponentCoefficientNegative
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleExponentCoefficientNegative(p *Parser, b byte) uint8 {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return SIG_NEXT_BYTE
	}
	if b == '0' {
		p.handle = handleExponentCoefficientLeadingZero
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleExponentCoefficientLeadingZero(p *Parser, b byte) uint8 {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return SIG_NEXT_BYTE
	}
	if b == '0' {
		return SIG_NEXT_BYTE
	}
	// TODO: exponent only had /0+/ for the exponent
	// signal this if it is important
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleExponentCoefficientEnd(p *Parser, b byte) uint8 {
	if b >= '0' && b <= '9' {
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleString(p *Parser, b byte) uint8 {
	switch b {
	case '\\':
		// reverse solidus prefix detected
		p.handle = handleStringReverseSolidusPrefix
		return SIG_NEXT_BYTE
	case '"':
		// end of string
		popHandle(p)
		fallthrough
	default:
		return SIG_NEXT_BYTE
	}
}

func handleStringReverseSolidusPrefix(p *Parser, b byte) uint8 {
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
		fallthrough
	case '\\':
		fallthrough
	case '"':
		p.handle = handleString
		return SIG_NEXT_BYTE
	case 'u':
		p.handle = handleStringHexShortIndex0
		return SIG_NEXT_BYTE
	default:
		return signalUnspecifiedError(p)
	}
}

func handleStringHexShortIndex0(p *Parser, b byte) uint8 {
	p.handle = handleStringHexShortIndex1
	return checkHexChar(p, b)
}

func handleStringHexShortIndex1(p *Parser, b byte) uint8 {
	p.handle = handleStringHexShortIndex2
	return checkHexChar(p, b)
}

func handleStringHexShortIndex2(p *Parser, b byte) uint8 {
	p.handle = handleStringHexShortIndex3
	return checkHexChar(p, b)
}

func handleStringHexShortIndex3(p *Parser, b byte) uint8 {
	p.handle = handleString
	return checkHexChar(p, b)
}

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	switch b {
	case '"':
		p.handle = handleDictExpectKeyValueDelim
		pushHandle(p, handleString)
		return SIG_NEXT_BYTE
	case '}':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectKeyValueDelim(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	if b == ':' {
		p.handle = handleDictExpectValue
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectValue(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = handleDictExpectEntryDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectEntryDelimOrEnd(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	switch b {
	case ',':
		p.handle = handleDictExpectKey
		return SIG_NEXT_BYTE
	case '}':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectKey(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	if b == '"' {
		p.handle = handleDictExpectKeyValueDelim
		pushHandle(p, handleString)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	if b == ']' {
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = handleArrayExpectDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectDelimOrEnd(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	switch b {
	case ',':
		p.handle = handleArrayExpectEntry
		return SIG_NEXT_BYTE
	case ']':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectEntry(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace {
		if isCharWhitespace(b) {
			return SIG_NEXT_BYTE
		}
	}
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = handleArrayExpectDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleEnd(p *Parser, b byte) uint8 {
	if p.allowFreeContextWhitespace && isCharWhitespace(b) {
		return SIG_EOF
	}
	return signalUnspecifiedError(p)
}

func (p *Parser) Parse(byteReader io.ByteReader) error {
	singleByte, err := byteReader.ReadByte()
	if err != nil {
		return err
	}

PARSE_LOOP:
	//fmt.Printf("%s", string(singleByte))  // DEBUG
	sig := p.handle(p, singleByte)
	if sig == SIG_NEXT_BYTE {
		singleByte, err = byteReader.ReadByte()
		if err == nil {
			goto PARSE_LOOP
		}
		if err == io.EOF && len(p.handleStack) == 0 {
			return nil
		}
		return err
	} else if sig == SIG_REUSE_BYTE {
		goto PARSE_LOOP
	} else if sig == SIG_EOF {
		// only in this block if allowFreeContextWhitespace is on
		// and trailing whitespace does exist, so just make sure there
		// is truly no more data before EOF
	END_STATE_LOOP:
		singleByte, err = byteReader.ReadByte()
		if err == nil {
			if isCharWhitespace(singleByte) {
				goto END_STATE_LOOP
			}
			return unspecifiedParseError
		} else if err == io.EOF {
			return nil
		}
		return err
	} else if sig == SIG_ERR {
		return p.err
	}

	// NOTE: not possible to reach this point
	return nil
}

type Parser struct {
	handle                     func(p *Parser, b byte) uint8
	literalStateIndex          uint8
	allowFreeContextWhitespace bool
	err                        error
	handleStack                []func(p *Parser, b byte) uint8
}

const (
	OPT_IGNORE_EXTRA_KEYS              = 0x01
	OPT_EXPECT_NO_FREE_FORM_WHITESPACE = 0x02
)

// TODO: support config options
func NewParser() Parser {
	self := Parser{
		literalStateIndex: 1,
		handle:            handleStart,
		err:               nil,
		allowFreeContextWhitespace: false,
	}
	// minimum nominal case will require 3 state levels
	// TODO: allow for configuring this size parameter
	self.handleStack = make([]func(p *Parser, b byte) uint8, 0, 3)
	return self
}
