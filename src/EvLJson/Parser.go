package EvLJson

import (
    "io"
    //"fmt"  // DEBUG
    //"log"  // DEBUG
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

const (
    STATE_UNDEFINED = iota
    STATE_IN_NULL
    STATE_IN_TRUE
    STATE_IN_FALSE
    STATE_IN_ZERO_OR_DECIMAL_OR_EXPONENT_START
    STATE_IN_INT
    STATE_IN_INT_EXPECT_FIRST_DIGIT_NON_ZERO
    STATE_IN_DECIMAL_FRACTIONAL_START
    STATE_IN_DECIMAL_FRACTIONAL_END
    STATE_IN_EXPONENT_START
    STATE_IN_EXPONENT_LEADING_ZERO
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

var PARSER_STATE_ACTION_LOOKUP = []func(p *Parser, b byte) bool{
    handleStart,
    handleNull,
    handleTrue,
    handleFalse,
    handleZeroOrDecimalOrExponentStart,
    handleInt,
    handleIntExpectFirstDigitNonZero,
    handleDecimalFractionalStart,
    handleDecimalFractionalEnd,
    handleExponentStart,
    handleExponentLeadingZero,
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

func pushState(p *Parser, newState uint8) {
    p.stateStack = append(p.stateStack, p.state)
    p.state = newState
}

func popState(p *Parser) {
    newMaxIdx := len(p.stateStack) - 1
    p.state, p.stateStack = p.stateStack[newMaxIdx], p.stateStack[:newMaxIdx]
}

func getNewObjState(p *Parser, b byte) uint8 {
    if b >= '1' && b <= '9' {
        return STATE_IN_INT
    }
    switch b {
    case '0':
        return STATE_IN_ZERO_OR_DECIMAL_OR_EXPONENT_START
    case '[':
        return STATE_IN_ARRAY_EXPECT_FIRST_ENTRY_OR_END
    case '{':
        return STATE_IN_DICT_EXPECT_FIRST_KEY_OR_END
    case OBJ_STR_NULL[0]:
        return STATE_IN_NULL
    case OBJ_STR_FALSE[0]:
        return STATE_IN_FALSE
    case OBJ_STR_TRUE[0]:
        return STATE_IN_TRUE
    case '"':
        return STATE_IN_STRING
    case '-':
        return STATE_IN_INT_EXPECT_FIRST_DIGIT_NON_ZERO
    default:
        return STATE_UNDEFINED
    }
}

func handleIfStartOfNewObj(p *Parser, b byte) bool {
    if newObjState := getNewObjState(p, b); newObjState != STATE_UNDEFINED {
        pushState(p, newObjState)
        return true
    }
    return false
}

func handleStart(p *Parser, b byte) bool {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return true
        }
    }
    p.state = STATE_END
    if handleIfStartOfNewObj(p, b) {
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleNull(p *Parser, b byte) bool {
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

func handleTrue(p *Parser, b byte) bool {
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

func handleFalse(p *Parser, b byte) bool {
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

func handleZeroOrDecimalOrExponentStart(p *Parser, b byte) bool {
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

func handleInt(p *Parser, b byte) bool {
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

func handleIntExpectFirstDigitNonZero(p *Parser, b byte) bool {
    if b >= '1' && b <= '9' {
        p.state = STATE_IN_INT
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleDecimalFractionalStart(p *Parser, b byte) bool {
    if b >= '0' && b <= '9' {
        p.state = STATE_IN_DECIMAL_FRACTIONAL_END
        return true
    }
    popState(p)
    return false
}

func handleDecimalFractionalEnd(p *Parser, b byte) bool {
    switch {
    case b >= '0' && b <= '9':
        return true
    case b == 'e':
        // TODO: negotiate type changing features
        p.state = STATE_IN_EXPONENT_START
        return true
    }
    popState(p)
    return false
}

func handleExponentStart(p *Parser, b byte) bool {
    if b >= '1' && b <= '9' {
        p.state = STATE_IN_EXPONENT_END
        return true
    }
    if b == '0' {
        p.state = STATE_IN_EXPONENT_LEADING_ZERO
        return true
    }
    if b == '-' {
        p.state = STATE_IN_EXPONENT_END
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleExponentLeadingZero(p *Parser, b byte) bool {
    if b >= '1' && b <= '9' {
        p.state = STATE_IN_EXPONENT_END
        return true
    }
    if b == '0' {
        return true
    }
    // TODO: exponent only had /0+/ for the exponent
    // signal this if it is important
    popState(p)
    return false
}

func handleExponentEnd(p *Parser, b byte) bool {
    if b >= '0' && b <= '9' {
        return true
    }
    popState(p)
    return false
}

func handleString(p *Parser, b byte) bool {
    if p.stringHexDigitIndex > 0 {
        p.stringHexDigitIndex++
        if b >= '0' && b <= '9' {
            // do nothing
        } else {
            if b > 'F' {
                b -= ('f' - 'F')
            }
            if b < 'A' || b > 'F' {
                p.err = unspecifiedParseError
                return true
            }
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

func handleDictExpectFirstKeyOrEnd(p *Parser, b byte) bool {
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

func handleDictExpectKeyValueDelim(p *Parser, b byte) bool {
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

func handleDictExpectValue(p *Parser, b byte) bool {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return true
        }
    }
    if newObjState := getNewObjState(p, b); newObjState != STATE_UNDEFINED {
        p.state = STATE_IN_DICT_EXPECT_ENTRY_DELIM_OR_END
        pushState(p, newObjState)
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleDictExpectEntryDelimOrEnd(p *Parser, b byte) bool {
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

func handleDictExpectKey(p *Parser, b byte) bool {
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

func handleArrayExpectFirstEntryOrEnd(p *Parser, b byte) bool {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return true
        }
    }
    if b == ']' {
        popState(p)
        return true
    }
    if newObjState := getNewObjState(p, b); newObjState != STATE_UNDEFINED {
        p.state = STATE_IN_ARRAY_EXPECT_DELIM_OR_END
        pushState(p, newObjState)
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleArrayExpectDelimOrEnd(p *Parser, b byte) bool {
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

func handleArrayExpectEntry(p *Parser, b byte) bool {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return true
        }
    }
    if newObjState := getNewObjState(p, b); newObjState != STATE_UNDEFINED {
        p.state = STATE_IN_ARRAY_EXPECT_DELIM_OR_END
        pushState(p, newObjState)
        return true
    }
    p.err = unspecifiedParseError
    return true
}

func handleEnd(p *Parser, b byte) bool {
    if p.allowFreeContextWhitespace {
        if isCharWhitespace(b) {
            return true
        }
    }
    p.err = unspecifiedParseError
    return true
}

func (p *Parser) Parse(byteReader io.ByteReader) error {
    singleByte, err := byteReader.ReadByte()

    // locality of reference using slices increases throughput
    parserStateActionLookup := PARSER_STATE_ACTION_LOOKUP[:]

    /*
    debugOldState := uint8(STATE_UNDEFINED)
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
        for !parserStateActionLookup[p.state](p, singleByte) {
            //debugFunc()
        }
        //debugFunc()
        err = p.err
        if err == nil {
            singleByte, err = byteReader.ReadByte()
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

type Parser struct {
    state                      uint8
    literalStateIndex          uint8
    stringHexDigitIndex        uint8
    stateStack                 []uint8
    err                        error
    reverseSolidusParity       bool
    allowFreeContextWhitespace bool
}

const (
    OPT_IGNORE_EXTRA_KEYS              = 0x01
    OPT_EXPECT_NO_FREE_FORM_WHITESPACE = 0x02
)

// TODO: support config options
func NewParser() Parser {
    return Parser{
        reverseSolidusParity: false,
        stringHexDigitIndex:  0,
        // minimum nominal case will require 3 state levels
        // TODO: allow for configuring this size parameter
        stateStack:        []uint8{0, 0, 0},
        literalStateIndex: 1,
        state:             STATE_UNDEFINED,
        err:               nil,
        allowFreeContextWhitespace: false,
    }
}
