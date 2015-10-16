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

type handle_t uint8
type signal_t uint8
type event_t uint8
type eventReceiver_t func(parser *Parser, evt event_t)
type dataReceiver_t func(parser *Parser, endOfData bool)
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

var whitespaces = map[byte]interface{}{
	0x20: nil, // SPACE
	0x09: nil, // TAB
	0x0A: nil, // LF
	0x0D: nil, // CR
}

func isCharWhitespace(b byte) bool {
	_, exists := whitespaces[b]
	return exists
}

func pushHandle(p *Parser, handle *handle_t, newHandle handle_t) {
	p.ContextStack = append(p.ContextStack, *handle)
	*handle = newHandle
}

// Note: user can signal within this function
func pushEnterHandle(p *Parser, handle *handle_t, newHandle handle_t, evt event_t) {
	p.onEvent(p, EVT_ENTER)
	if p.userSignal != SIG_STOP {
		pushHandle(p, handle, newHandle)
		p.onEvent(p, evt)
	}
}

func pushNewValueHandle(p *Parser, handle *handle_t, err *error, b byte) signal_t {
	if b >= '1' && b <= '9' {
		p.DataIsJsonNum = true
		pushEnterHandle(p, handle, HANDLE_INT, EVT_NUMBER)
		return p.yieldToUserSig(SIG_NEXT_BYTE)
	}
	switch b {
	case '0':
		p.DataIsJsonNum = true
		pushEnterHandle(p, handle, HANDLE_ZD_EXP_START, EVT_NUMBER)
	case '[':
		pushEnterHandle(p, handle, p.handleArrayStart, EVT_ARRAY)
	case '{':
		pushEnterHandle(p, handle, p.handleDictStart, EVT_DICT)
	case VALUE_STR_NULL[0]:
		p.onEvent(p, EVT_NULL)
		pushHandle(p, handle, HANDLE_NULL)
	case VALUE_STR_FALSE[0]:
		p.onEvent(p, EVT_FALSE)
		pushHandle(p, handle, HANDLE_FALSE)
	case VALUE_STR_TRUE[0]:
		p.onEvent(p, EVT_TRUE)
		pushHandle(p, handle, HANDLE_TRUE)
	case '"':
		p.DataIsJsonNum = false
		pushEnterHandle(p, handle, HANDLE_STRING, EVT_STRING)
	case '-':
		p.DataIsJsonNum = true
		pushEnterHandle(p, handle, HANDLE_ZD_EXPN_START, EVT_NUMBER)
	default:
		*err = unspecifiedParserError
		return SIG_ERR
	}
	return p.yieldToUserSig(SIG_NEXT_BYTE)
}

func popHandle(p *Parser, handle *handle_t) {
	newMaxIdx := len(p.ContextStack) - 1
	*handle, p.ContextStack = p.ContextStack[newMaxIdx], p.ContextStack[:newMaxIdx]
}

// Note: user can signal within this function
func popHandleEvent(p *Parser, handle *handle_t) {
	popHandle(p, handle)
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

const ( // handle_t
	HANDLE_START_AEW = iota
	HANDLE_START
	HANDLE_NULL
	HANDLE_TRUE
	HANDLE_FALSE
	HANDLE_ZD_EXP_START // handleZeroOrDecimalOrExponentStart
	HANDLE_INT
	HANDLE_ZD_EXPN_START         // handleZeroOrDecimalOrExponentNegativeStart
	HANDLE_DEC_FRAC_START        // handleDecimalFractionalStart
	HANDLE_DEC_FRAC_END          // handleDecimalFractionalEnd
	HANDLE_EXP_COEF_START        // handleExponentCoefficientStart
	HANDLE_EXP_COEF_NEG          // handleExponentCoefficientNegative
	HANDLE_EXP_COEF_LZERO        // handleExponentCoefficientLeadingZero
	HANDLE_EXP_COEF_STRICT_LZERO // handleExponentCoefficientStrictLeadingZero
	HANDLE_EXP_COEF_END          // handleExponentCoefficientEnd
	HANDLE_STRING
	HANDLE_STRING_RSP // handleStringReverseSolidusPrefix
	HANDLE_HEX_NR     // handleStringHexShortNoReporting
	HANDLE_HEX_EVEN   // handleStringHexShortEven
	HANDLE_HEX_ODD    // handleStringHexShortOdd
	HANDLE_DICT_START_AEW
	HANDLE_DICT_START
	HANDLE_DICT_KV_DELIM_AEW
	HANDLE_DICT_KV_DELIM
	HANDLE_DICT_VALUE_AEW
	HANDLE_DICT_VALUE
	HANDLE_DICT_VALUE_END_AEW
	HANDLE_DICT_VALUE_END
	HANDLE_DICT_EXPECT_KEY_AEW
	HANDLE_DICT_EXPECT_KEY
	HANDLE_ARRAY_START_AEW
	HANDLE_ARRAY_START
	HANDLE_ARRAY_DELIM_AEW
	HANDLE_ARRAY_DELIM
	HANDLE_ARRAY_EXPECT_ENTRY_AEW
	HANDLE_ARRAY_EXPECT_ENTRY
	HANDLE_END_AEW
	HANDLE_END
	HANDLE_STOP
)

var allhexchars = map[byte]interface{}{
	'0': nil,
	'1': nil,
	'2': nil,
	'3': nil,
	'4': nil,
	'5': nil,
	'6': nil,
	'7': nil,
	'8': nil,
	'9': nil,
	'a': nil,
	'b': nil,
	'c': nil,
	'd': nil,
	'e': nil,
	'f': nil,
	'A': nil,
	'B': nil,
	'C': nil,
	'D': nil,
	'E': nil,
	'F': nil,
}

var caphexchars = map[byte]interface{}{
	'A': nil,
	'B': nil,
	'C': nil,
	'D': nil,
	'E': nil,
	'F': nil,
}

var lowerhexchars = map[byte]interface{}{
	'0': nil,
	'1': nil,
	'2': nil,
	'3': nil,
	'4': nil,
	'5': nil,
	'6': nil,
	'7': nil,
	'8': nil,
	'9': nil,
	'a': nil,
	'b': nil,
	'c': nil,
	'd': nil,
	'e': nil,
	'f': nil,
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

func (p *Parser) Parse(byteReader io.ByteReader, onEvent eventReceiver_t, onData dataReceiver_t) error {
	isEmptyJson := true
	handle := p.handleStart
	var literalStateIndex uint8 = 1
	var b byte
	var err error
	var signal signal_t
	var hexShortBuffer [2]byte

	if onEvent != nil {
		p.onEvent = onEvent
	} else {
		p.onEvent = defaultOnEvent
	}

	p.OnData = onData

NEXT_BYTE:
	b, err = byteReader.ReadByte()
	if err == nil {
	PARSE_LOOP:
		// fmt.Printf("%s: %d\n", string(b), handle)  // DEBUG
		switch handle {
		case HANDLE_START_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_START:
			handle = p.handleEnd
			if b == '[' {
				pushEnterHandle(p, &handle, p.handleArrayStart, EVT_ARRAY)
			} else if b == '{' {
				pushEnterHandle(p, &handle, p.handleDictStart, EVT_DICT)
			} else {
				return unspecifiedParserError
			}
			isEmptyJson = false
			signal = p.yieldToUserSig(SIG_NEXT_BYTE)
		case HANDLE_NULL:
			if b == VALUE_STR_NULL[literalStateIndex] {
				if literalStateIndex != uint8(len(VALUE_STR_NULL)-1) {
					literalStateIndex++
					goto NEXT_BYTE
				}
				literalStateIndex = 1
				popHandle(p, &handle)
				goto NEXT_BYTE
			}
			return unspecifiedParserError
		case HANDLE_TRUE:
			if b == VALUE_STR_TRUE[literalStateIndex] {
				if literalStateIndex != uint8(len(VALUE_STR_TRUE)-1) {
					literalStateIndex++
					goto NEXT_BYTE
				}
				literalStateIndex = 1
				popHandle(p, &handle)
				goto NEXT_BYTE
			}
			return unspecifiedParserError
		case HANDLE_FALSE:
			if b == VALUE_STR_FALSE[literalStateIndex] {
				if literalStateIndex != uint8(len(VALUE_STR_FALSE)-1) {
					literalStateIndex++
					goto NEXT_BYTE
				}
				literalStateIndex = 1
				popHandle(p, &handle)
				goto NEXT_BYTE
			}
			return unspecifiedParserError
		case HANDLE_ZD_EXP_START:
			switch b {
			case '.':
				p.onEvent(p, EVT_DECIMAL)
				if p.userSignal != SIG_STOP {
					handle = HANDLE_DEC_FRAC_START
					signal = signalDataNextByte(p, b)
					break
				}
				return nil
			case 'e':
				p.onEvent(p, EVT_EXPONENT)
				if p.userSignal != SIG_STOP {
					handle = HANDLE_EXP_COEF_START
					signal = signalDataNextByte(p, b)
					break
				}
				return nil
			default:
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
			}
		case HANDLE_INT:
			switch b {
			case '0':
				fallthrough
			case '1':
				fallthrough
			case '2':
				fallthrough
			case '3':
				fallthrough
			case '4':
				fallthrough
			case '5':
				fallthrough
			case '6':
				fallthrough
			case '7':
				fallthrough
			case '8':
				fallthrough
			case '9':
				break
			case '.':
				p.onEvent(p, EVT_DECIMAL)
				if p.userSignal != SIG_STOP {
					handle = HANDLE_DEC_FRAC_START
				} else {
					return nil
				}
			case 'e':
				p.onEvent(p, EVT_EXPONENT)
				if p.userSignal != SIG_STOP {
					handle = HANDLE_EXP_COEF_START
				} else {
					return nil
				}
			default:
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
				goto SIGNAL_PROCESSING
			}
			signal = signalDataNextByte(p, b)
		case HANDLE_ZD_EXPN_START:
			if b == '0' {
				handle = HANDLE_ZD_EXP_START
			} else if b >= '1' && b <= '9' {
				handle = HANDLE_INT
			} else {
				return unspecifiedParserError
			}
			signal = signalDataNextByte(p, b)
		case HANDLE_DEC_FRAC_START:
			if b >= '0' && b <= '9' {
				handle = HANDLE_DEC_FRAC_END
				signal = signalDataNextByte(p, b)
			} else {
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
			}
		case HANDLE_DEC_FRAC_END:
			switch {
			case b >= '0' && b <= '9':
				signal = signalDataNextByte(p, b)
			case b == 'e':
				p.onEvent(p, EVT_EXPONENT)
				if p.userSignal != SIG_STOP {
					handle = HANDLE_EXP_COEF_START
					signal = signalDataNextByte(p, b)
					break
				}
				return nil
			default:
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
			}
		case HANDLE_EXP_COEF_START:
			if b >= '1' && b <= '9' {
				handle = HANDLE_EXP_COEF_END
			} else if b == '0' {
				handle = p.handleExponentCoefficientLeadingZero
			} else if b == '-' {
				handle = HANDLE_EXP_COEF_NEG
			} else {
				return unspecifiedParserError
			}
			signal = signalDataNextByte(p, b)
		case HANDLE_EXP_COEF_NEG:
			if b >= '1' && b <= '9' {
				handle = HANDLE_EXP_COEF_END
			} else if b == '0' {
				handle = p.handleExponentCoefficientLeadingZero
			} else {
				return unspecifiedParserError
			}
			signal = signalDataNextByte(p, b)
		case HANDLE_EXP_COEF_LZERO:
			if b >= '1' && b <= '9' {
				handle = HANDLE_EXP_COEF_END
			} else if b != '0' {
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
				break
			}
			signal = signalDataNextByte(p, b)
		case HANDLE_EXP_COEF_STRICT_LZERO:
			if b >= '1' && b <= '9' {
				handle = HANDLE_EXP_COEF_END
				signal = signalDataNextByte(p, b)
			} else if b == '0' {
				return invalidStricterExponentFormat
			} else {
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
			}
		case HANDLE_EXP_COEF_END:
			if b >= '0' && b <= '9' {
				signal = signalDataNextByte(p, b)
			} else {
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_REUSE_BYTE)
			}
		case HANDLE_STRING:
			switch b {
			case '\\':
				// reverse solidus prefix detected
				handle = HANDLE_STRING_RSP
				goto NEXT_BYTE
			case '"':
				// end of string
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_NEXT_BYTE)
			default:
				signal = signalDataNextByte(p, b)
			}
		case HANDLE_STRING_RSP:
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
					handle = HANDLE_HEX_NR
				} else {
					handle = HANDLE_HEX_EVEN
				}
				goto NEXT_BYTE
			default:
				return unspecifiedParserError
			}
			break
		UNESCAPED:
			handle = HANDLE_STRING
			signal = signalDataNextByte(p, b)
		case HANDLE_HEX_NR:
			if _, exists := allhexchars[b]; exists {
				newLiteralStateIndex := literalStateIndex + 1
				if newLiteralStateIndex != 5 {
					literalStateIndex = newLiteralStateIndex
				} else {
					handle = HANDLE_STRING
					literalStateIndex = 1
				}
				goto NEXT_BYTE
			} else {
				return unspecifiedParserError
			}
		case HANDLE_HEX_EVEN:
			handle = HANDLE_HEX_ODD
			if _, exists := lowerhexchars[b]; exists {
				// Do Nothing
			} else if _, exists = caphexchars[b]; exists {
				b = b + ('a' - 'A') // convert to lower case hex char (speedup)
			} else {
				return unspecifiedParserError
			}
			hexShortBuffer[literalStateIndex] = b
			goto NEXT_BYTE
		case HANDLE_HEX_ODD:
			if _, exists := lowerhexchars[b]; exists {
				// Do Nothing
			} else if _, exists = caphexchars[b]; exists {
				b = b + ('a' - 'A') // convert to lower case hex char (speedup)
			} else {
				return unspecifiedParserError
			}
			var err error
			decodedBytes := []byte{0}
			newLiteralStateIndex := literalStateIndex
			if _, err = hex.Decode(decodedBytes, []byte{hexShortBuffer[newLiteralStateIndex], b}); err == nil {
				if newLiteralStateIndex == 1 {
					literalStateIndex = 0
					handle = HANDLE_HEX_EVEN
					hexShortBuffer[1] = decodedBytes[0]
					goto NEXT_BYTE
				} else {
					literalStateIndex = 1
					handle = HANDLE_STRING
				}
				size := len(p.DataBuffer)
				if !(size+1 >= cap(p.DataBuffer)) {
					p.DataBuffer = p.DataBuffer[0 : size+2]
					p.DataBuffer[size] = hexShortBuffer[1]
					size++
					p.DataBuffer[size] = decodedBytes[0]
					goto NEXT_BYTE
				}
				p.OnData(p, DATA_CONTINUES)
				if p.userSignal != SIG_STOP {
					p.DataBuffer = p.DataBuffer[0:2]
					p.DataBuffer[0] = hexShortBuffer[1]
					p.DataBuffer[1] = decodedBytes[0]
					goto NEXT_BYTE
				}
				return nil
			}
			return err
		case HANDLE_DICT_START_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_DICT_START:
			if b == '"' {
				handle = p.handleDictKVDelim
				pushEnterHandle(p, &handle, HANDLE_STRING, EVT_STRING)
			} else if b == '}' {
				popHandleEvent(p, &handle)
			} else {
				return unspecifiedParserError
			}
			signal = p.yieldToUserSig(SIG_NEXT_BYTE)
		case HANDLE_DICT_KV_DELIM_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_DICT_KV_DELIM:
			if b == ':' {
				handle = p.handleDictValue
				goto NEXT_BYTE
			} else {
				return unspecifiedParserError
			}
		case HANDLE_DICT_VALUE_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_DICT_VALUE:
			handle = p.handleDictValueEnd
			signal = pushNewValueHandle(p, &handle, &err, b)
		case HANDLE_DICT_VALUE_END_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_DICT_VALUE_END:
			switch b {
			case ',':
				handle = p.handleDictExpectKey
				goto NEXT_BYTE
			case '}':
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_NEXT_BYTE)
			default:
				return unspecifiedParserError
			}
		case HANDLE_DICT_EXPECT_KEY_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_DICT_EXPECT_KEY:
			if b == '"' {
				handle = p.handleDictKVDelim
				pushEnterHandle(p, &handle, HANDLE_STRING, EVT_STRING)
				signal = p.yieldToUserSig(SIG_NEXT_BYTE)
			} else {
				return unspecifiedParserError
			}
		case HANDLE_ARRAY_START_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_ARRAY_START:
			if b != ']' {
				handle = p.handleArrayDelim
				signal = pushNewValueHandle(p, &handle, &err, b)
			} else {
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_NEXT_BYTE)
			}
		case HANDLE_ARRAY_DELIM_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_ARRAY_DELIM:
			switch b {
			case ',':
				handle = p.handleArrayExpectEntry
				goto NEXT_BYTE
			case ']':
				popHandleEvent(p, &handle)
				signal = p.yieldToUserSig(SIG_NEXT_BYTE)
			default:
				return unspecifiedParserError
			}
		case HANDLE_ARRAY_EXPECT_ENTRY_AEW:
			if isCharWhitespace(b) {
				goto NEXT_BYTE
			}
			fallthrough
		case HANDLE_ARRAY_EXPECT_ENTRY:
			handle = p.handleArrayDelim
			signal = pushNewValueHandle(p, &handle, &err, b)
		case HANDLE_END_AEW:
			for isCharWhitespace(b) {
				b, err = byteReader.ReadByte()
				if err == nil {
					continue
				}
				if err == io.EOF {
					return nil
				}
			}
			return unspecifiedParserError
		case HANDLE_END:
			return unspecifiedParserError
		case HANDLE_STOP:
			return nil
		}
	SIGNAL_PROCESSING:
		switch signal {
		case SIG_NEXT_BYTE:
			goto NEXT_BYTE
		case SIG_REUSE_BYTE:
			goto PARSE_LOOP
		case SIG_STOP:
			return nil
		default:
			// SIG_ERR
			if err == nil {
				return unspecifiedParserError
			}
			return err
		}
	} else if err == io.EOF && len(p.ContextStack) == 0 && !isEmptyJson {
		return nil
	}
	return err
}

type Parser struct {

	// current state processor

	UserData interface{}
	onEvent  eventReceiver_t
	OnData   dataReceiver_t

	// BEGIN: configured calls
	handleStart                          handle_t
	handleDictStart                      handle_t
	handleDictKVDelim                    handle_t
	handleDictValue                      handle_t
	handleDictValueEnd                   handle_t
	handleDictExpectKey                  handle_t
	handleArrayStart                     handle_t
	handleArrayDelim                     handle_t
	handleArrayExpectEntry               handle_t
	handleEnd                            handle_t
	handleExponentCoefficientLeadingZero handle_t

	// END: configured calls

	ContextStack   []handle_t
	DataBuffer     []byte
	DataIsJsonNum  bool
	userSignal     signal_t
	yieldToUserSig userSig_t
}

func (p *Parser) Reset() {
	p.userSignal = SIG_NEXT_BYTE
	p.yieldToUserSig = userSigNone
}

func NewParser(dataBuffer []byte, contextStack []handle_t, options uint8) Parser {
	self := Parser{}
	self.Reset()

	if options&OPT_ALLOW_EXTRA_WHITESPACE == 0 {
		self.handleStart = HANDLE_START
		self.handleDictStart = HANDLE_DICT_START
		self.handleDictKVDelim = HANDLE_DICT_KV_DELIM
		self.handleDictValue = HANDLE_DICT_VALUE
		self.handleDictValueEnd = HANDLE_DICT_VALUE_END
		self.handleDictExpectKey = HANDLE_DICT_EXPECT_KEY
		self.handleArrayStart = HANDLE_ARRAY_START
		self.handleArrayDelim = HANDLE_ARRAY_DELIM
		self.handleArrayExpectEntry = HANDLE_ARRAY_EXPECT_ENTRY

		if options&OPT_PARSE_UNTIL_EOF == 0 {
			self.handleEnd = HANDLE_STOP
		} else {
			self.handleEnd = HANDLE_END
		}
	} else {
		self.handleStart = HANDLE_START_AEW
		self.handleDictStart = HANDLE_DICT_START_AEW
		self.handleDictKVDelim = HANDLE_DICT_KV_DELIM_AEW
		self.handleDictValue = HANDLE_DICT_VALUE_AEW
		self.handleDictValueEnd = HANDLE_DICT_VALUE_END_AEW
		self.handleDictExpectKey = HANDLE_DICT_EXPECT_KEY_AEW
		self.handleArrayStart = HANDLE_ARRAY_START_AEW
		self.handleArrayDelim = HANDLE_ARRAY_DELIM_AEW
		self.handleArrayExpectEntry = HANDLE_ARRAY_EXPECT_ENTRY_AEW

		if options&OPT_PARSE_UNTIL_EOF == 0 {
			self.handleEnd = HANDLE_STOP
		} else {
			self.handleEnd = HANDLE_END_AEW
		}
	}

	if options&OPT_STRICTER_EXPONENTS == 0 {
		self.handleExponentCoefficientLeadingZero = HANDLE_EXP_COEF_LZERO
	} else {
		self.handleExponentCoefficientLeadingZero = HANDLE_EXP_COEF_STRICT_LZERO
	}

	if contextStack == nil {
		contextStack = make([]handle_t, 0, MIN_STACK_DEPTH)
	} else {
		if cap(contextStack) < MIN_STACK_DEPTH {
			contextStack = contextStack[0:MIN_STACK_DEPTH]
		}
		contextStack = contextStack[:0]
	}
	self.ContextStack = contextStack
	if dataBuffer == nil {
		dataBuffer = make([]byte, MIN_DATA_BUFFER_SIZE)
	} else {
		if cap(dataBuffer) < MIN_DATA_BUFFER_SIZE {
			dataBuffer = dataBuffer[0:MIN_DATA_BUFFER_SIZE]
		}
		dataBuffer = dataBuffer[:0]
	}
	self.DataBuffer = dataBuffer
	return self
}
