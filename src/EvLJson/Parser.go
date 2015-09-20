package EvLJson

import (
	"io"
	//"fmt"  // DEBUG
	//"log"  // DEBUG
)

// REMINDER: leave data member [literalStateIndex] in as removing it
// would only result in the same # of operations, but using more jump
// table space which slows performance

// TODO: callback properties
// 1. publish state changes
// 2. publish byte data
// 3. allow signaling the parser to stop

// at least two for encoded short chars in strings plus 1 so that short
// handling does not need to signal more than once as the buffer is updated
//
// ^^^ this is very important ^^^
//
const MIN_DATA_BUFFER_SIZE = 3

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
	EVT_NONE = iota
	EVT_NULL
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
type UnspecifiedJsonParserError struct{}

func (err UnspecifiedJsonParserError) Error() string {
	return "Unspecified json parser error"
}

var unspecifiedParserError = UnspecifiedJsonParserError{}

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
	p.DataBuffer = p.DataBuffer[0:1]
	p.DataBuffer[0] = b
	return SIG_NEXT_BYTE
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

func pushHandleEvent(p *Parser, newHandle parserHandle_t, evt event_t) {
	pushHandle(p, newHandle)
	p.onEvent(p, EVT_ENTER)
	p.onEvent(p, evt)
}

func popHandle(p *Parser) {
	newMaxIdx := len(p.handleStack) - 1
	p.handle, p.handleStack = p.handleStack[newMaxIdx], p.handleStack[:newMaxIdx]
	if len(p.DataBuffer) == 0 {
		p.onEvent(p, EVT_LEAVE)
		return
	}
	if p.OnData != nil {
		p.OnData(p, DATA_END)
	}
	p.DataBuffer = p.DataBuffer[:0]
	p.onEvent(p, EVT_LEAVE)
}

func getNewValueHandle(b byte) (parserHandle_t, event_t) {
	if b >= '1' && b <= '9' {
		return handleInt, EVT_NUMBER
	}
	switch b {
	case '0':
		return handleZeroOrDecimalOrExponentStart, EVT_NUMBER
	case '[':
		return handleArrayExpectFirstEntryOrEnd, EVT_ARRAY
	case '{':
		return handleDictExpectFirstKeyOrEnd, EVT_DICT
	case VALUE_STR_NULL[0]:
		return handleNull, EVT_NONE
	case VALUE_STR_FALSE[0]:
		return handleFalse, EVT_NONE
	case VALUE_STR_TRUE[0]:
		return handleTrue, EVT_NONE
	case '"':
		return handleString, EVT_STRING
	case '-':
		return handleZeroOrDecimalOrExponentNegativeStart, EVT_NUMBER
	default:
		return nil, EVT_NONE
	}
}

func handleStart(p *Parser, b byte) signal_t {
	p.handle = p.handleEnd
	if b == '[' {
		pushHandleEvent(p, p.handleArrayExpectFirstEntryOrEnd, EVT_ARRAY)
		return SIG_NEXT_BYTE
	}
	if b == '{' {
		pushHandleEvent(p, p.handleDictExpectFirstKeyOrEnd, EVT_DICT)
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
		p.onEvent(p, EVT_NULL)
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
		p.onEvent(p, EVT_TRUE)
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
		p.onEvent(p, EVT_FALSE)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleZeroOrDecimalOrExponentStart(p *Parser, b byte) signal_t {
	switch b {
	case '.':
		p.onEvent(p, EVT_DECIMAL)
		p.handle = handleDecimalFractionalStart
		return signalDataNextByte(p, b)
	case 'e':
		p.onEvent(p, EVT_EXPONENT)
		p.handle = handleExponentCoefficientStart
		return signalDataNextByte(p, b)
	default:
		popHandle(p)
		return SIG_REUSE_BYTE
	}
}

func handleInt(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		return signalDataNextByte(p, b)
	}
	switch b {
	case '.':
		p.onEvent(p, EVT_DECIMAL)
		p.handle = handleDecimalFractionalStart
		return signalDataNextByte(p, b)
	case 'e':
		p.onEvent(p, EVT_EXPONENT)
		p.handle = handleExponentCoefficientStart
		return signalDataNextByte(p, b)
	}
	popHandle(p)
	return SIG_REUSE_BYTE
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
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleDecimalFractionalEnd(p *Parser, b byte) signal_t {
	switch {
	case b >= '0' && b <= '9':
		return signalDataNextByte(p, b)
	case b == 'e':
		p.onEvent(p, EVT_EXPONENT)
		p.handle = handleExponentCoefficientStart
		return signalDataNextByte(p, b)
	}
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleExponentCoefficientStart(p *Parser, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.handle = handleExponentCoefficientEnd
		return signalDataNextByte(p, b)
	}
	if b == '0' {
		p.handle = handleExponentCoefficientLeadingZero
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
		p.handle = handleExponentCoefficientLeadingZero
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
	// TODO: exponent only had /0+/ for the exponent
	// signal this if it is important
	popHandle(p)
	return SIG_REUSE_BYTE
}

func handleExponentCoefficientEnd(p *Parser, b byte) signal_t {
	if b >= '0' && b <= '9' {
		return signalDataNextByte(p, b)
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
		return SIG_NEXT_BYTE
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
		p.handle = handleStringHexShortEven
		return SIG_NEXT_BYTE
	default:
		return signalUnspecifiedError(p)
	}
UNESCAPED:
	p.handle = handleString
	return signalDataNextByte(p, b)
}

func handleStringHexShortEven(p *Parser, b byte) signal_t {
	p.handle = handleStringHexShortOdd
	if b <= '9' {
		if b >= '0' {
			p.hexShortBuffer = b
			return SIG_NEXT_BYTE
		}
	} else {
		if b >= 'a' {
			if b <= 'f' {
				p.hexShortBuffer = b
				return SIG_NEXT_BYTE
			}
		} else if b >= 'A' && b <= 'F' {
			p.hexShortBuffer = b + ('a' - 'A')
			return SIG_NEXT_BYTE
		}
	}
	return signalUnspecifiedError(p)
}

func handleStringHexShortOdd(p *Parser, b byte) signal_t {
	if p.literalStateIndex == 1 {
		p.literalStateIndex = 2
		p.handle = handleStringHexShortEven
	} else {
		p.literalStateIndex = 1
		p.handle = handleString
	}
	if b <= '9' {
		if b >= '0' {
			if p.OnData == nil {
				return SIG_NEXT_BYTE
			}
			size := len(p.DataBuffer)
			if !(size+1 >= cap(p.DataBuffer)) {
				p.DataBuffer = p.DataBuffer[0 : size+2]
			} else {
				p.OnData(p, DATA_CONTINUES)
				p.DataBuffer = p.DataBuffer[0:2]
			}
			p.DataBuffer[size] = p.hexShortBuffer
			size++
			p.DataBuffer[size] = b
			return SIG_NEXT_BYTE
		}
	} else {
		if b >= 'a' {
			if b <= 'f' {
				if p.OnData == nil {
					return SIG_NEXT_BYTE
				}
				size := len(p.DataBuffer)
				if !(size+1 >= cap(p.DataBuffer)) {
					p.DataBuffer = p.DataBuffer[0 : size+2]
				} else {
					p.OnData(p, DATA_CONTINUES)
					p.DataBuffer = p.DataBuffer[0:2]
				}
				p.DataBuffer[size] = p.hexShortBuffer
				size++
				p.DataBuffer[size] = b
				return SIG_NEXT_BYTE
			}
		} else if b >= 'A' && b <= 'F' {
			if p.OnData == nil {
				return SIG_NEXT_BYTE
			}
			size := len(p.DataBuffer)
			if !(size+1 >= cap(p.DataBuffer)) {
				p.DataBuffer = p.DataBuffer[0 : size+2]
			} else {
				p.OnData(p, DATA_CONTINUES)
				p.DataBuffer = p.DataBuffer[0:2]
			}
			p.DataBuffer[size] = p.hexShortBuffer
			size++
			p.DataBuffer[size] = b
			return SIG_NEXT_BYTE
		}
	}
	return signalUnspecifiedError(p)
}

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) signal_t {
	switch b {
	case '"':
		p.handle = p.handleDictExpectKeyValueDelim
		pushHandleEvent(p, handleString, EVT_STRING)
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
	if newHandle, newEvent := getNewValueHandle(b); newHandle != nil {
		p.handle = p.handleDictExpectEntryDelimOrEnd
		switch newEvent {
		case EVT_NONE:
			pushHandle(p, newHandle)
		default:
			pushHandleEvent(p, newHandle, newEvent)
		}
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
		pushHandleEvent(p, handleString, EVT_STRING)
		return SIG_NEXT_BYTE
	}
	return signalUnspecifiedError(p)
}

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) signal_t {
	if b != ']' {
		if newHandle, newEvent := getNewValueHandle(b); newHandle != nil {
			p.handle = p.handleArrayExpectDelimOrEnd
			switch newEvent {
			case EVT_NONE:
				pushHandle(p, newHandle)
			default:
				pushHandleEvent(p, newHandle, newEvent)
			}
			return SIG_NEXT_BYTE
		}
		return signalUnspecifiedError(p)
	}
	popHandle(p)
	return SIG_NEXT_BYTE
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
	if newHandle, newEvent := getNewValueHandle(b); newHandle != nil {
		p.handle = p.handleArrayExpectDelimOrEnd
		switch newEvent {
		case EVT_NONE:
			pushHandle(p, newHandle)
		default:
			pushHandleEvent(p, newHandle, newEvent)
		}
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

func defaultOnEvent(parser *Parser, evt event_t) {
	return
}

const (
	OPT_ALLOW_EXTRA_WHITESPACE = 0x01
	OPT_STRICTER_EXPONENTS     = 0x02 // TODO: not implemented
	OPT_PARSE_UNTIL_EOF        = 0x04
)

func (p *Parser) Parse(byteReader io.ByteReader, onEvent eventReceiver_t, OnData dataReceiver_t, options uint8) error {
	singleByte, err := byteReader.ReadByte()
	if err != nil {
		return err
	}

	if onEvent != nil {
		p.onEvent = onEvent
	} else {
		p.onEvent = defaultOnEvent
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

	hexShortBuffer byte

	literalStateIndex uint8
	err               error
	handleStack       []parserHandle_t
	DataBuffer        []byte
}

func NewParser(dataBuffer []byte) Parser {
	self := Parser{
		literalStateIndex: 1,
		err:               nil,
		UserData:          nil,
		onEvent:           nil,
		OnData:            nil,
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
	// minimum nominal case will require 3 state levels
	// TODO: allow for configuring this size parameter
	self.handleStack = make([]parserHandle_t, 0, 3)
	return self
}
