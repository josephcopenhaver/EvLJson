package EvLJson

import (
    "io"
    //"fmt"  // DEBUG
    //"log"  // DEBUG
)


const (
    SIG_USE_NEXT_BYTE = iota
    SIG_REUSE_LAST_BYTE
    SIG_EOF
    SIG_ERR
)
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

func pushState(p *Parser, newHandle func(p *Parser, b byte) uint8) {
    p.handleStack = append(p.handleStack, p.handle)
    p.handle = newHandle
}

func popState(p *Parser) {
    newMaxIdx := len(p.handleStack) - 1
    p.handle, p.handleStack = p.handleStack[newMaxIdx], p.handleStack[:newMaxIdx]
}

// TODO: rename `newObjState` to `newHandle`

// TODO: rename ...NewValueHandle
func getNewObjState(p *Parser, b byte) func(p *Parser, b byte) uint8 {
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
    case OBJ_STR_NULL[0]:
        return handleNull
    case OBJ_STR_FALSE[0]:
        return handleFalse
    case OBJ_STR_TRUE[0]:
        return handleTrue
    case '"':
        return handleString
    case '-':
        return handleIntExpectFirstDigitNonZero
    default:
        return nil
    }
}

func signalUnspecifiedParseError(p *Parser) uint8 {
    p.err = unspecifiedParseError
    return SIG_ERR
}

func handleStart(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    p.handle = handleEnd
    if b == '[' {
        pushState(p, handleArrayExpectFirstEntryOrEnd)
        return SIG_USE_NEXT_BYTE
    }
    if b == '{' {
        pushState(p, handleDictExpectFirstKeyOrEnd)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleNull(p *Parser, b byte) uint8 {
    if b == OBJ_STR_NULL[p.literalStateIndex] {
        p.literalStateIndex++
        if p.literalStateIndex == uint8(len(OBJ_STR_NULL)) {
            p.literalStateIndex = 1
            popState(p)
        }
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleTrue(p *Parser, b byte) uint8 {
    if b == OBJ_STR_TRUE[p.literalStateIndex] {
        p.literalStateIndex++
        if p.literalStateIndex == uint8(len(OBJ_STR_TRUE)) {
            p.literalStateIndex = 1
            popState(p)
        }
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleFalse(p *Parser, b byte) uint8 {
    if b == OBJ_STR_FALSE[p.literalStateIndex] {
        p.literalStateIndex++
        if p.literalStateIndex == uint8(len(OBJ_STR_FALSE)) {
            p.literalStateIndex = 1
            popState(p)
        }
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleZeroOrDecimalOrExponentStart(p *Parser, b byte) uint8 {
    switch b {
    case '.':
        // TODO: negotiate type changing features
        p.handle = handleDecimalFractionalStart
        return SIG_USE_NEXT_BYTE
    case 'e':
        // TODO: negotiate type changing features
        p.handle = handleExponentStart
        return SIG_USE_NEXT_BYTE
    default:
        popState(p)
        return SIG_REUSE_LAST_BYTE
    }
}

func handleInt(p *Parser, b byte) uint8 {
    if b >= '0' && b <= '9' {
        return SIG_USE_NEXT_BYTE
    }
    switch b {
    case '.':
        // TODO: negotiate type changing features
        p.handle = handleDecimalFractionalStart
        return SIG_USE_NEXT_BYTE
    case 'e':
        // TODO: negotiate type changing features
        p.handle = handleExponentStart
        return SIG_USE_NEXT_BYTE
    }
    popState(p)
    return SIG_REUSE_LAST_BYTE
}

func handleIntExpectFirstDigitNonZero(p *Parser, b byte) uint8 {
    if b >= '1' && b <= '9' {
        p.handle = handleInt
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleDecimalFractionalStart(p *Parser, b byte) uint8 {
    if b >= '0' && b <= '9' {
        p.handle = handleDecimalFractionalEnd
        return SIG_USE_NEXT_BYTE
    }
    popState(p)
    return SIG_REUSE_LAST_BYTE
}

func handleDecimalFractionalEnd(p *Parser, b byte) uint8 {
    switch {
    case b >= '0' && b <= '9':
        return SIG_USE_NEXT_BYTE
    case b == 'e':
        // TODO: negotiate type changing features
        p.handle = handleExponentStart
        return SIG_USE_NEXT_BYTE
    }
    popState(p)
    return SIG_REUSE_LAST_BYTE
}

func handleExponentStart(p *Parser, b byte) uint8 {
    if b >= '1' && b <= '9' {
        p.handle = handleExponentEnd
        return SIG_USE_NEXT_BYTE
    }
    if b == '0' {
        p.handle = handleExponentLeadingZero
        return SIG_USE_NEXT_BYTE
    }
    if b == '-' {
        p.handle = handleExponentAfterMultSign
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleExponentAfterMultSign(p *Parser, b byte) uint8 {
    if b >= '1' && b <= '9' {
        p.handle = handleExponentEnd
        return SIG_USE_NEXT_BYTE
    }
    if b == '0' {
        p.handle = handleExponentLeadingZero
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleExponentLeadingZero(p *Parser, b byte) uint8 {
    if b >= '1' && b <= '9' {
        p.handle = handleExponentEnd
        return SIG_USE_NEXT_BYTE
    }
    if b == '0' {
        return SIG_USE_NEXT_BYTE
    }
    // TODO: exponent only had /0+/ for the exponent
    // signal this if it is important
    popState(p)
    return SIG_REUSE_LAST_BYTE
}

func handleExponentEnd(p *Parser, b byte) uint8 {
    if b >= '0' && b <= '9' {
        return SIG_USE_NEXT_BYTE
    }
    popState(p)
    return SIG_REUSE_LAST_BYTE
}

func handleString(p *Parser, b byte) uint8 {
    stringHexDigitIndex := p.stringHexDigitIndex
    if stringHexDigitIndex > 0 {
        if b >= '0' && b <= '9' {
            // do nothing
        } else if b >= 'a' && b <= 'f' {
            // do nothing
        } else if b < 'a' {
            b += ('a' - 'A')
            if b >= 'a' && b <= 'f' {
                // do nothing
            } else {
                return signalUnspecifiedParseError(p)
            }
        }
        if stringHexDigitIndex == 4 {
            p.stringHexDigitIndex = 0
        } else {
            p.stringHexDigitIndex = stringHexDigitIndex + 1
        }
        return SIG_USE_NEXT_BYTE
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
        return SIG_USE_NEXT_BYTE
    case '\\':
        // reverse solidus (escape) parity adjusted
        p.reverseSolidusParity = !p.reverseSolidusParity
        return SIG_USE_NEXT_BYTE
    case '"':
        if !p.reverseSolidusParity {
            // end of string
            popState(p)
            return SIG_USE_NEXT_BYTE
        }
        p.reverseSolidusParity = false
        return SIG_USE_NEXT_BYTE
    case 'u':
        if p.reverseSolidusParity {
            p.reverseSolidusParity = false
            p.stringHexDigitIndex = 1
        }
        return SIG_USE_NEXT_BYTE
    default:
        if !p.reverseSolidusParity {
            return SIG_USE_NEXT_BYTE
        }
        return signalUnspecifiedParseError(p)
    }
}

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    switch b {
    case '"':
        p.handle = handleDictExpectKeyValueDelim
        pushState(p, handleString)
        return SIG_USE_NEXT_BYTE
    case '}':
        popState(p)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleDictExpectKeyValueDelim(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    if b == ':' {
        p.handle = handleDictExpectValue
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleDictExpectValue(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    if newObjState := getNewObjState(p, b); newObjState != nil {
        p.handle = handleDictExpectEntryDelimOrEnd
        pushState(p, newObjState)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleDictExpectEntryDelimOrEnd(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    switch b {
    case ',':
        p.handle = handleDictExpectKey
        return SIG_USE_NEXT_BYTE
    case '}':
        popState(p)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleDictExpectKey(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    if b == '"' {
        p.handle = handleDictExpectKeyValueDelim
        pushState(p, handleString)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    if b == ']' {
        popState(p)
        return SIG_USE_NEXT_BYTE
    }
    if newObjState := getNewObjState(p, b); newObjState != nil {
        p.handle = handleArrayExpectDelimOrEnd
        pushState(p, newObjState)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleArrayExpectDelimOrEnd(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    switch b {
    case ',':
        p.handle = handleArrayExpectEntry
        return SIG_USE_NEXT_BYTE
    case ']':
        popState(p)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleArrayExpectEntry(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return SIG_USE_NEXT_BYTE
        }
    }
    if newObjState := getNewObjState(p, b); newObjState != nil {
        p.handle = handleArrayExpectDelimOrEnd
        pushState(p, newObjState)
        return SIG_USE_NEXT_BYTE
    }
    return signalUnspecifiedParseError(p)
}

func handleEnd(p *Parser, b byte) uint8 {
    if p.allowFreeContextWhitespace && isCharWhitespace(b) {
        return SIG_EOF
    }
    return signalUnspecifiedParseError(p)
}

func (p *Parser) Parse(byteReader io.ByteReader) error {
    singleByte, err := byteReader.ReadByte()
    if err != nil {
        return err
    }

    PARSE_LOOP:
    //fmt.Printf("%s", string(singleByte))  // DEBUG
    sig := p.handle(p, singleByte)
    if sig == SIG_USE_NEXT_BYTE {
        singleByte, err = byteReader.ReadByte()
        if err != nil {
            if err == io.EOF && len(p.handleStack) == 0 {
                return nil
            }
            return err
        }
        goto PARSE_LOOP
    } else if sig == SIG_REUSE_LAST_BYTE {
        goto PARSE_LOOP
    } else if sig == SIG_EOF {
        // only in this block if allowFreeContextWhitespace is on
        // and trailing whitespace does exist, so just make sure there
        // is truly no more data before EOF
        END_STATE_LOOP:
        singleByte, err = byteReader.ReadByte()
        if err == io.EOF {
            return nil
        }
        if !isCharWhitespace(singleByte) {
            return unspecifiedParseError
        }
        goto END_STATE_LOOP
    } else if sig == SIG_ERR {
        return p.err
    }

    // NOTE: not possible to reach this point
    return nil
}

type Parser struct {
    handle                     func(p *Parser, b byte) uint8
    literalStateIndex          uint8
    stringHexDigitIndex        uint8
    reverseSolidusParity       bool
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
        reverseSolidusParity: false,
        stringHexDigitIndex:  0,
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
