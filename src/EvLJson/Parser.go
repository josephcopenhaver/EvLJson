package EvLJson

import (
	"encoding/hex"
	"io"
	//"fmt"  // DEBUG
	//"log"  // DEBUG
)

// REMINDER: leave data member [literalStateIndex] in as removing it
// would only result in the same # of operations, but using more jump
// table space which slows performance

// at least two for encoded short chars in strings plus 1 so that short
// handling does not need to signal more than once as the buffer is updated
//
// ^^^ this is very important ^^^
//
const MIN_DATA_BUFFER_SIZE = 3

// minimum nominal case will require 3 state levels
const MIN_STACK_DEPTH = 3

const ( // data stream output signals
	DATA_CONTINUES = false
	DATA_END       = true
)

const ( // signal_t
	SIG_NEXT_BYTE = iota
	SIG_REUSE_BYTE
	SIG_STOP
	SIG_EOF
	SIG_ERR
)
const (
	VALUE_STR_NULL  = "null"
	VALUE_STR_TRUE  = "true"
	VALUE_STR_FALSE = "false"
)
const ( // event_t
	EVT_NULL = iota
	EVT_TRUE
	EVT_FALSE
	EVT_ENTER // BEGIN: container list
	EVT_ARRAY
	EVT_DICT
	EVT_LEAVE // END: container list
	EVT_STRING
	EVT_NUMBER
	EVT_DECIMAL
	EVT_EXPONENT
)

type signal_t uint8
type event_t uint8
type eventReceiver_t func(parser *Parser, evt event_t)
type dataReceiver_t func(parser *Parser, endOfData bool)
type parserHandle_t func(p *Parser, b byte) signal_t
type userSig_t func(normalSignal signal_t) signal_t
type UnspecifiedJsonParserError struct{}

func (err UnspecifiedJsonParserError) Error() string {
	return "Unspecified json parser error"
}

var unspecifiedParserError = UnspecifiedJsonParserError{}

type InvalidStricterExponentFormat struct{}

func (err InvalidStricterExponentFormat) Error() string {
	return "OPT_STRICTER_EXPONENTS is on: More than one leading zero in exponent"
}

var invalidStricterExponentFormat = InvalidStricterExponentFormat{}

func signalUnspecifiedError(p *Parser) signal_t {
	p.err = unspecifiedParserError
	return SIG_ERR
}

func signalDataNextByte(p *Parser, b byte) signal_t {
	if p.OnData == nil {
		return SIG_NEXT_BYTE
	}
	size := len(p.DataBuffer)
	if size != cap(p.DataBuffer) {
		p.DataBuffer = p.DataBuffer[0 : size+1]
		p.DataBuffer[size] = b
		return SIG_NEXT_BYTE
	}
	p.OnData(p, DATA_CONTINUES)
	if p.userSignal != SIG_STOP {
		p.DataBuffer = p.DataBuffer[0:1]
		p.DataBuffer[0] = b
		return SIG_NEXT_BYTE
	}
	return SIG_STOP
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

// Note: user can signal within this function
func pushEnterHandle(p *Parser, newHandle parserHandle_t, evt event_t) {
	p.onEvent(p, EVT_ENTER)
	if p.userSignal != SIG_STOP {
		pushHandle(p, newHandle)
		p.onEvent(p, evt)
	}
}

func pushNewValueHandle(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.DataIsJsonNum = true
		pushEnterHandle(p, handleInt, EVT_NUMBER)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	switch b {
	case '0':
		p.DataIsJsonNum = true
		pushEnterHandle(p, handleZeroOrDecimalOrExponentStart, EVT_NUMBER)
	case '[':
		pushEnterHandle(p, handleArrayExpectFirstEntryOrEnd, EVT_ARRAY)
	case '{':
		pushEnterHandle(p, handleDictExpectFirstKeyOrEnd, EVT_DICT)
	case VALUE_STR_NULL[0]:
		p.onEvent(p, EVT_NULL)
		pushHandle(p, handleNull)
		break
	case VALUE_STR_FALSE[0]:
		p.onEvent(p, EVT_FALSE)
		pushHandle(p, handleFalse)
		break
	case VALUE_STR_TRUE[0]:
		p.onEvent(p, EVT_TRUE)
		pushHandle(p, handleTrue)
		break
	case '"':
		p.DataIsJsonNum = false
		pushEnterHandle(p, handleString, EVT_STRING)
	case '-':
		p.DataIsJsonNum = true
		pushEnterHandle(p, handleZeroOrDecimalOrExponentNegativeStart, EVT_NUMBER)
	default:
		return signalUnspecifiedError(p)
	}
	return p.yieldToUserSig(SIG_NEXT_BYTE)
}

func popHandle(p *Parser) {
	newMaxIdx := len(p.handleStack) - 1
	p.handle, p.handleStack = p.handleStack[newMaxIdx], p.handleStack[:newMaxIdx]
}

// Note: user can signal within this function
func popHandleEvent(p *Parser) {
	popHandle(p)
	if len(p.DataBuffer) == 0 {
		p.onEvent(p, EVT_LEAVE)
		return
	}
	if p.OnData != nil {
		p.OnData(p, DATA_END)
		if p.userSignal != SIG_STOP {
			goto FIRE_LEAVE_EVT
		}
		return
	}
FIRE_LEAVE_EVT:
	p.DataBuffer = p.DataBuffer[:0]
	p.onEvent(p, EVT_LEAVE)
}

func handleStart(p *Parser, b byte) signal_t {
	p.handle = p.handleEnd
	if b == '[' {
		pushEnterHandle(p, p.handleArrayExpectFirstEntryOrEnd, EVT_ARRAY)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	if b == '{' {
		pushEnterHandle(p, p.handleDictExpectFirstKeyOrEnd, EVT_DICT)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
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
		p.onEvent(p, EVT_DECIMAL)
		if p.userSignal != SIG_STOP {
			p.handle = handleDecimalFractionalStart
			return signalDataNextByte(p, b)
		}
		return SIG_STOP
	case 'e':
		p.onEvent(p, EVT_EXPONENT)
		if p.userSignal != SIG_STOP {
			p.handle = handleExponentCoefficientStart
			return signalDataNextByte(p, b)
		}
		return SIG_STOP
	default:
		popHandleEvent(p)
		return p.yieldToUserSig(SIG_REUSE_BYTE)
	}
}

func handleInt(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		return signalDataNextByte(p, b)
	}
	switch b {
	case '.':
		p.onEvent(p, EVT_DECIMAL)
		if p.userSignal != SIG_STOP {
			p.handle = handleDecimalFractionalStart
			return signalDataNextByte(p, b)
		}
		return SIG_STOP
	case 'e':
		p.onEvent(p, EVT_EXPONENT)
		if p.userSignal != SIG_STOP {
			p.handle = handleExponentCoefficientStart
			return signalDataNextByte(p, b)
		}
		return SIG_STOP
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleZeroOrDecimalOrExponentNegativeStart(p *Parser, b byte) signal_t {
	if b == '0' {
		p.handle = handleZeroOrDecimalOrExponentStart
		return signalDataNextByte(p, b)
	}
	if b >= '1' && b <= '9' {
		p.handle = handleInt
		return signalDataNextByte(p, b)
	}
	return signalUnspecifiedError(p)
}

func handleDecimalFractionalStart(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		p.handle = handleDecimalFractionalEnd
		return signalDataNextByte(p, b)
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleDecimalFractionalEnd(p *Parser, b byte) signal_t {
	switch {
	case b >= '0' && b <= '9':
		return signalDataNextByte(p, b)
	case b == 'e':
		p.onEvent(p, EVT_EXPONENT)
		if p.userSignal != SIG_STOP {
			p.handle = handleExponentCoefficientStart
			return signalDataNextByte(p, b)
		}
		return SIG_STOP
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleExponentCoefficientStart(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return signalDataNextByte(p, b)
	}
	if b == '0' {
		p.handle = p.handleExponentCoefficientLeadingZero
		return signalDataNextByte(p, b)
	}
	if b == '-' {
		p.handle = handleExponentCoefficientNegative
		return signalDataNextByte(p, b)
	}
	return signalUnspecifiedError(p)
}

func handleExponentCoefficientNegative(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return signalDataNextByte(p, b)
	}
	if b == '0' {
		p.handle = p.handleExponentCoefficientLeadingZero
		return signalDataNextByte(p, b)
	}
	return signalUnspecifiedError(p)
}

func handleExponentCoefficientLeadingZero(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return signalDataNextByte(p, b)
	}
	if b == '0' {
		return signalDataNextByte(p, b)
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleExponentCoefficientStricterLeadingZero(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return signalDataNextByte(p, b)
	}
	if b == '0' {
		p.err = invalidStricterExponentFormat
		return SIG_ERR
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleExponentCoefficientEnd(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		return signalDataNextByte(p, b)
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_REUSE_BYTE)
}

func handleString(p *Parser, b byte) signal_t {
	switch b {
	case '\\':
		// reverse solidus prefix detected
		p.handle = handleStringReverseSolidusPrefix
		return SIG_NEXT_BYTE
	case '"':
		// end of string
		popHandleEvent(p)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	default:
		return signalDataNextByte(p, b)
	}
}

func handleStringReverseSolidusPrefix(p *Parser, b byte) signal_t {
	switch b {
	case 'b':
		// backspace
		b = '\b'
		goto UNESCAPED
	case 'f':
		// formfeed
		b = '\f'
		goto UNESCAPED
	case 'n':
		// newline
		b = '\n'
		goto UNESCAPED
	case 'r':
		// carriage return
		b = '\r'
		goto UNESCAPED
	case 't':
		b = '\t'
		fallthrough
	case '/':
		// allowed to be escaped, no special implications
		fallthrough
	case '\\':
		fallthrough
	case '"':
		goto UNESCAPED
	case 'u':
		if p.OnData == nil {
			p.handle = handleStringHexShortNoReporting
			return SIG_NEXT_BYTE
		}
		p.handle = handleStringHexShortEven
		return SIG_NEXT_BYTE
	default:
		return signalUnspecifiedError(p)
	}
UNESCAPED:
	p.handle = handleString
	return signalDataNextByte(p, b)
}

func handleStringHexShortNoReporting(p *Parser, b byte) signal_t {
	for true {
		if b <= '9' {
			if b >= '0' {
				break
			}
		} else {
			if b >= 'a' {
				if b <= 'f' {
					break
				}
			} else if b >= 'A' && b <= 'F' {
				break
			}
		}
		return signalUnspecifiedError(p)
	}
	literalStateIndex := p.literalStateIndex + 1
	if literalStateIndex != 5 {
		p.literalStateIndex = literalStateIndex
		return SIG_NEXT_BYTE
	}
	p.handle = handleString
	p.literalStateIndex = 1
	return SIG_NEXT_BYTE
}

func handleStringHexShortEven(p *Parser, b byte) signal_t {
	p.handle = handleStringHexShortOdd
	if b <= '9' {
		if b >= '0' {
			p.hexShortBuffer[p.literalStateIndex] = b
			return SIG_NEXT_BYTE
		}
	} else {
		if b >= 'a' {
			if b <= 'f' {
				p.hexShortBuffer[p.literalStateIndex] = b
				return SIG_NEXT_BYTE
			}
		} else if b >= 'A' && b <= 'F' {
			p.hexShortBuffer[p.literalStateIndex] = b + ('a' - 'A')
			return SIG_NEXT_BYTE
		}
	}
	return signalUnspecifiedError(p)
}

func handleStringHexShortOdd(p *Parser, b byte) signal_t {
	for true {
		if b <= '9' {
			if b >= '0' {
				break
			}
		} else {
			if b >= 'a' {
				if b <= 'f' {
					break
				}
			} else if b >= 'A' && b <= 'F' {
				b = b + ('a' - 'A')
				break
			}
		}
		return signalUnspecifiedError(p)
	}
	var err error
	decodedBytes := []byte{0}
	literalStateIndex := p.literalStateIndex
	if _, err = hex.Decode(decodedBytes, []byte{p.hexShortBuffer[literalStateIndex], b}); err == nil {
		if literalStateIndex == 1 {
			p.literalStateIndex = 0
			p.handle = handleStringHexShortEven
			p.hexShortBuffer[1] = decodedBytes[0]
			return SIG_NEXT_BYTE
		} else {
			p.literalStateIndex = 1
			p.handle = handleString
		}
		size := len(p.DataBuffer)
		if !(size+1 >= cap(p.DataBuffer)) {
			p.DataBuffer = p.DataBuffer[0 : size+2]
			p.DataBuffer[size] = p.hexShortBuffer[1]
			size++
			p.DataBuffer[size] = decodedBytes[0]
			return SIG_NEXT_BYTE
		} else {
			p.OnData(p, DATA_CONTINUES)
			if p.userSignal != SIG_STOP {
				p.DataBuffer = p.DataBuffer[0:2]
				p.DataBuffer[0] = p.hexShortBuffer[1]
				p.DataBuffer[1] = decodedBytes[0]
				return SIG_NEXT_BYTE
			}
			return SIG_STOP
		}
	}
	p.err = err
	return SIG_ERR
}

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case '"':
		p.handle = p.handleDictExpectKeyValueDelim
		pushEnterHandle(p, handleString, EVT_STRING)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	case '}':
		popHandleEvent(p)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
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
	p.handle = p.handleDictExpectEntryDelimOrEnd
	return pushNewValueHandle(p, b)
}

func handleDictExpectEntryDelimOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case ',':
		p.handle = p.handleDictExpectKey
		return SIG_NEXT_BYTE
	case '}':
		popHandleEvent(p)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectKey(p *Parser, b byte) signal_t {
	if b == '"' {
		p.handle = p.handleDictExpectKeyValueDelim
		pushEnterHandle(p, handleString, EVT_STRING)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) signal_t {
	if b != ']' {
		p.handle = p.handleArrayExpectDelimOrEnd
		return pushNewValueHandle(p, b)
	}
	popHandleEvent(p)
	return p.yieldToUserSig(SIG_NEXT_BYTE)
}

func handleArrayExpectDelimOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case ',':
		p.handle = p.handleArrayExpectEntry
		return SIG_NEXT_BYTE
	case ']':
		popHandleEvent(p)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectEntry(p *Parser, b byte) signal_t {
	p.handle = p.handleArrayExpectDelimOrEnd
	return pushNewValueHandle(p, b)
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

func defaultOnEvent(parser *Parser, evt event_t) {
	return
}

func userSigNone(normalSignal signal_t) signal_t {
	return normalSignal
}

func userSigStop(normalSignal signal_t) signal_t {
	return SIG_STOP
}

const (
	OPT_ALLOW_EXTRA_WHITESPACE = 0x01
	OPT_STRICTER_EXPONENTS     = 0x02
	OPT_PARSE_UNTIL_EOF        = 0x04
)

func (p *Parser) ParseStop() {
	p.userSignal = SIG_STOP
	p.yieldToUserSig = userSigStop
}

func (p *Parser) Parse(byteReader io.ByteReader, onEvent eventReceiver_t, onData dataReceiver_t, options uint8) error {
	singleByte, err := byteReader.ReadByte()
	if err != nil {
		return err
	}

	if onEvent != nil {
		p.onEvent = onEvent
	} else {
		p.onEvent = defaultOnEvent
	}

	p.OnData = onData

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

	if options&OPT_STRICTER_EXPONENTS == 0 {
		p.handleExponentCoefficientLeadingZero = handleExponentCoefficientLeadingZero
	} else {
		p.handleExponentCoefficientLeadingZero = handleExponentCoefficientStricterLeadingZero
	}

PARSE_LOOP:
	//fmt.Printf("%s", string(singleByte))  // DEBUG
	switch p.handle(p, singleByte) {
	case SIG_NEXT_BYTE:
		singleByte, err = byteReader.ReadByte()
		if err == nil {
			goto PARSE_LOOP
		}
		if err == io.EOF && len(p.handleStack) == 0 {
			return nil
		}
		return err
	case SIG_REUSE_BYTE:
		goto PARSE_LOOP
	case SIG_STOP:
		return nil
	case SIG_EOF:
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
	default:
		// SIG_ERR
		return p.err
	}
}

type Parser struct {

	// current state processor

	handle   parserHandle_t
	UserData interface{}
	onEvent  eventReceiver_t
	OnData   dataReceiver_t

	// BEGIN: configured calls

	handleDictExpectFirstKeyOrEnd        parserHandle_t
	handleDictExpectKeyValueDelim        parserHandle_t
	handleDictExpectValue                parserHandle_t
	handleDictExpectEntryDelimOrEnd      parserHandle_t
	handleDictExpectKey                  parserHandle_t
	handleArrayExpectFirstEntryOrEnd     parserHandle_t
	handleArrayExpectDelimOrEnd          parserHandle_t
	handleArrayExpectEntry               parserHandle_t
	handleEnd                            parserHandle_t
	handleExponentCoefficientLeadingZero parserHandle_t

	// END: configured calls

	hexShortBuffer    []byte
	DataBuffer        []byte
	handleStack       []parserHandle_t
	DataIsJsonNum     bool
	literalStateIndex uint8
	userSignal        signal_t
	yieldToUserSig    userSig_t
	err               error
}

func NewParser(dataBuffer []byte, typeDepthHint int) Parser {
	self := Parser{
		literalStateIndex: 1,
		err:               nil,
		UserData:          nil,
		onEvent:           nil,
		OnData:            nil,
		hexShortBuffer:    make([]byte, 2),
		DataIsJsonNum:     false,
		userSignal:        SIG_NEXT_BYTE,
		yieldToUserSig:    userSigNone,
	}
	if dataBuffer == nil {
		dataBuffer = make([]byte, MIN_DATA_BUFFER_SIZE)
	} else {
		if cap(dataBuffer) < MIN_DATA_BUFFER_SIZE {
			dataBuffer = dataBuffer[0:MIN_DATA_BUFFER_SIZE]
		}
		dataBuffer = dataBuffer[:0]
	}
	self.DataBuffer = dataBuffer
	if typeDepthHint < MIN_STACK_DEPTH {
		typeDepthHint = MIN_STACK_DEPTH
	}
	self.handleStack = make([]parserHandle_t, 0, typeDepthHint)
	return self
}
