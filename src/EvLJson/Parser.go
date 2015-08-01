package EvLJson

import (
	"io"
	//"fmt"  // DEBUG
	//"log"  // DEBUG
)

// REMINDER: leave data member [literalStateIndex] in as removing it
// would only result in the same # of operations, but using more jump
// table space which slows performance

const (
	SIG_NEXT_BYTE = iota
	SIG_REUSE_BYTE
	SIG_EOF
	SIG_STOP
	SIG_ERR
)
const (
	VALUE_STR_NULL  = "null"
	VALUE_STR_TRUE  = "true"
	VALUE_STR_FALSE = "false"
)

type signal_t uint8
type parserHandle_t func(p *Parser, b byte) signal_t
type UnspecifiedJsonParserError struct{}

func (err UnspecifiedJsonParserError) Error() string {
	return "Unspecified json parser error"
}

var unspecifiedParserError = UnspecifiedJsonParserError{}

func signalUnspecifiedError(p *Parser) signal_t {
	p.err = unspecifiedParserError
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

func pushHandle(p *Parser, newHandle parserHandle_t) {
	p.handleStack = append(p.handleStack, p.handle)
	p.handle = newHandle
}

func popHandle(p *Parser) {
	newMaxIdx := len(p.handleStack) - 1
	p.handle, p.handleStack = p.handleStack[newMaxIdx], p.handleStack[:newMaxIdx]
}

func getNewValueHandle(b byte) parserHandle_t {
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

func handleStart(p *Parser, b byte) signal_t {
	p.handle = p.handleEnd
	if b == '[' {
		pushHandle(p, p.handleArrayExpectFirstEntryOrEnd)
		return SIG_NEXT_BYTE
	}
	if b == '{' {
		pushHandle(p, p.handleDictExpectFirstKeyOrEnd)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleNull(p *Parser, b byte) signal_t {
	if b == VALUE_STR_NULL[p.literalStateIndex] {
		if p.literalStateIndex != uint8(len(VALUE_STR_NULL)-1) {
			p.literalStateIndex++
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleTrue(p *Parser, b byte) signal_t {
	if b == VALUE_STR_TRUE[p.literalStateIndex] {
		if p.literalStateIndex != uint8(len(VALUE_STR_TRUE)-1) {
			p.literalStateIndex++
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleFalse(p *Parser, b byte) signal_t {
	if b == VALUE_STR_FALSE[p.literalStateIndex] {
		if p.literalStateIndex != uint8(len(VALUE_STR_FALSE)-1) {
			p.literalStateIndex++
			return SIG_NEXT_BYTE
		}
		p.literalStateIndex = 1
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleZeroOrDecimalOrExponentStart(p *Parser, b byte) signal_t {
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

func handleInt(p *Parser, b byte) signal_t {
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

func handleZeroOrDecimalOrExponentNegativeStart(p *Parser, b byte) signal_t {
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

func handleDecimalFractionalStart(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		p.handle = handleDecimalFractionalEnd
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleDecimalFractionalEnd(p *Parser, b byte) signal_t {
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

func handleExponentCoefficientStart(p *Parser, b byte) signal_t {
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

func handleExponentCoefficientNegative(p *Parser, b byte) signal_t {
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

func handleExponentCoefficientLeadingZero(p *Parser, b byte) signal_t {
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

func handleExponentCoefficientEnd(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		return SIG_NEXT_BYTE
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleString(p *Parser, b byte) signal_t {
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

func handleStringReverseSolidusPrefix(p *Parser, b byte) signal_t {
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
		p.handle = handleStringHexShort
		return SIG_NEXT_BYTE
	default:
		return signalUnspecifiedError(p)
	}
}

func handleStringHexShort(p *Parser, b byte) signal_t {
	if p.literalStateIndex != 4 {
		p.literalStateIndex++
	} else {
		p.literalStateIndex = 1
		p.handle = handleString
	}
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

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case '"':
		p.handle = p.handleDictExpectKeyValueDelim
		pushHandle(p, handleString)
		return SIG_NEXT_BYTE
	case '}':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectKeyValueDelim(p *Parser, b byte) signal_t {
	if b == ':' {
		p.handle = p.handleDictExpectValue
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectValue(p *Parser, b byte) signal_t {
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = p.handleDictExpectEntryDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectEntryDelimOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case ',':
		p.handle = p.handleDictExpectKey
		return SIG_NEXT_BYTE
	case '}':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectKey(p *Parser, b byte) signal_t {
	if b == '"' {
		p.handle = p.handleDictExpectKeyValueDelim
		pushHandle(p, handleString)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) signal_t {
	if b == ']' {
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = p.handleArrayExpectDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectDelimOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case ',':
		p.handle = p.handleArrayExpectEntry
		return SIG_NEXT_BYTE
	case ']':
		popHandle(p)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectEntry(p *Parser, b byte) signal_t {
	if newHandle := getNewValueHandle(b); newHandle != nil {
		p.handle = p.handleArrayExpectDelimOrEnd
		pushHandle(p, newHandle)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleEnd(p *Parser, b byte) signal_t {
	return signalUnspecifiedError(p)
}

// BEGIN: Allow Extra Whitespace wrappers

func handleStart_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleStart(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleDictExpectFirstKeyOrEnd_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleDictExpectFirstKeyOrEnd(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleDictExpectKeyValueDelim_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleDictExpectKeyValueDelim(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleDictExpectValue_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleDictExpectValue(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleDictExpectEntryDelimOrEnd_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleDictExpectEntryDelimOrEnd(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleDictExpectKey_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleDictExpectKey(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleArrayExpectFirstEntryOrEnd_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleArrayExpectFirstEntryOrEnd(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleArrayExpectDelimOrEnd_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleArrayExpectDelimOrEnd(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleArrayExpectEntry_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleArrayExpectEntry(p, b)
	}
	return SIG_NEXT_BYTE
}

func handleEnd_AEW(p *Parser, b byte) signal_t {
	if !isCharWhitespace(b) {
		return handleEnd(p, b)
	}
	return SIG_EOF
}

// END: Allow Extra Whitespace wrappers

func handleStop(p *Parser, b byte) signal_t {
	return SIG_STOP
}

const (
	OPT_ALLOW_EXTRA_WHITESPACE = 0x01
	OPT_STRICTER_EXPONENTS     = 0x02 // TODO: not implemented
	OPT_PARSE_UNTIL_EOF        = 0x04
)

func (p *Parser) Parse(byteReader io.ByteReader, options uint8) error {
	singleByte, err := byteReader.ReadByte()
	if err != nil {
		return err
	}

	if options&OPT_ALLOW_EXTRA_WHITESPACE == 0 {
		p.handle = handleStart
		p.handleDictExpectFirstKeyOrEnd = handleDictExpectFirstKeyOrEnd
		p.handleDictExpectKeyValueDelim = handleDictExpectKeyValueDelim
		p.handleDictExpectValue = handleDictExpectValue
		p.handleDictExpectEntryDelimOrEnd = handleDictExpectEntryDelimOrEnd
		p.handleDictExpectKey = handleDictExpectKey
		p.handleArrayExpectFirstEntryOrEnd = handleArrayExpectFirstEntryOrEnd
		p.handleArrayExpectDelimOrEnd = handleArrayExpectDelimOrEnd
		p.handleArrayExpectEntry = handleArrayExpectEntry

		if options&OPT_PARSE_UNTIL_EOF == 0 {
			p.handleEnd = handleStop
		} else {
			p.handleEnd = handleEnd
		}
	} else {
		p.handle = handleStart_AEW
		p.handleDictExpectFirstKeyOrEnd = handleDictExpectFirstKeyOrEnd_AEW
		p.handleDictExpectKeyValueDelim = handleDictExpectKeyValueDelim_AEW
		p.handleDictExpectValue = handleDictExpectValue_AEW
		p.handleDictExpectEntryDelimOrEnd = handleDictExpectEntryDelimOrEnd_AEW
		p.handleDictExpectKey = handleDictExpectKey_AEW
		p.handleArrayExpectFirstEntryOrEnd = handleArrayExpectFirstEntryOrEnd_AEW
		p.handleArrayExpectDelimOrEnd = handleArrayExpectDelimOrEnd_AEW
		p.handleArrayExpectEntry = handleArrayExpectEntry_AEW

		if options&OPT_PARSE_UNTIL_EOF == 0 {
			p.handleEnd = handleStop
		} else {
			p.handleEnd = handleEnd_AEW
		}
	}

PARSE_LOOP:
	//fmt.Printf("%s", string(singleByte))  // DEBUG
	signal := p.handle(p, singleByte)
	if signal == SIG_NEXT_BYTE {
		singleByte, err = byteReader.ReadByte()
		if err == nil {
			goto PARSE_LOOP
		}
		if err == io.EOF && len(p.handleStack) == 0 {
			return nil
		}
		return err
	} else if signal == SIG_REUSE_BYTE {
		goto PARSE_LOOP
	} else if signal == SIG_STOP {
		return nil
	} else if signal == SIG_EOF {
		// only in this block if OPT_ALLOW_EXTRA_WHITESPACE flag is on
		// and trailing whitespace does exist, so just make sure there
		// is truly no more data before EOF
	END_STATE_LOOP:
		singleByte, err = byteReader.ReadByte()
		if err == nil {
			if isCharWhitespace(singleByte) {
				goto END_STATE_LOOP
			}
			return unspecifiedParserError
		} else if err == io.EOF {
			return nil
		}
		return err
	} else if signal == SIG_ERR {
		return p.err
	}

	// NOTE: not possible to reach this point
	return unspecifiedParserError
}

type Parser struct {

	// current state processor

	handle parserHandle_t

	// BEGIN: configured calls

	handleDictExpectFirstKeyOrEnd    parserHandle_t
	handleDictExpectKeyValueDelim    parserHandle_t
	handleDictExpectValue            parserHandle_t
	handleDictExpectEntryDelimOrEnd  parserHandle_t
	handleDictExpectKey              parserHandle_t
	handleArrayExpectFirstEntryOrEnd parserHandle_t
	handleArrayExpectDelimOrEnd      parserHandle_t
	handleArrayExpectEntry           parserHandle_t
	handleEnd                        parserHandle_t

	// END: configured calls

	literalStateIndex uint8
	err               error
	handleStack       []parserHandle_t
}

func NewParser() Parser {
	self := Parser{
		literalStateIndex: 1,
		err:               nil,
	}
	// minimum nominal case will require 3 state levels
	// TODO: allow for configuring this size parameter
	self.handleStack = make([]parserHandle_t, 0, 3)
	return self
}
