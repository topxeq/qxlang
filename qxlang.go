package qxlang

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/topxeq/tk"
)

var VersionG string = "0.0.1"
var DebugG = false

// OpCodes starts
const (
	OpConst int = iota

	OpAssignLocal
	OpGetLocalVarValue

	OpAddInt

	OpEqual
)

// instructions start
var InstrNameSet map[string]int = map[string]int{

	// internal & debug related

	"invalidInstr": 12, // invalid instruction, use internally to indicate invalid instr(s) while parsing commands

	"version": 100, // get current Qxlang version, return a string type value, if the result parameter not designated, it will be put to the global variable $tmp(and it's same for other instructions has result and not variable parameters)

	"pass": 101, // do nothing, useful for placeholder

	"testByText": 122, // for test purpose, check if 2 string values are equal

	// run code related

	"goto": 180, // jump to the instruction line (often indicated by labels)

	"exit": 199, // terminate the program, can with a return value(same as assign the global value $outG)

	// push/peek/pop stack related

	"push": 220, // push any value to stack

	"peek": 222, // peek the value on the top of the stack

	"pop": 224, // pop the value on the top of the stack

	// assign related

	"=": 401, // assignment, from local variable to global, assign value to local if not found

	// if/else, switch related
	"if": 610, // usage: if $boolValue1 :labelForTrue :labelForElse

	// compare related

	"<": 703, // compare two value if the 1st < 2nd

	// func related

	"call": 1010, // call a normal function, usage: call $result :func1 $arg1 $arg2...
	// result value could not be omitted, use $drop if not neccessary
	// all arguments/parameters will be put into the local variable "inputL" in the function
	// and the function should return result in local variable "outL"
	// use "ret $result" is a covenient way to set value of $outL and return from the function

	"ret": 1020, // return from a normal function or a fast call function, while for normal function call, can with a paramter for set $outL

	// array/slice related

	"getArrayItem": 1123,
	"[]":           1123,

	// time related
	"now": 1910, // get the current time

	// print related

	"pln": 10410, // same as println function in other languages
	"plo": 10411, // print a value with its type

	// system related

	"sleep": 20501, // sleep for n seconds(float, 0.001 means 1 millisecond)

	"getClipText": 20511, // get clipboard content as text

	"setClipText": 20512, // set clipboard content as text

	"getEnv":    20521, // get os environment variable by key
	"setEnv":    20522, // set os environment variable by key and value
	"removeEnv": 20523, // remove os environment variable by key

	"systemCmd": 20601, // run an os shell command, usage： systemCmd "cmd" "/k" "copy a.txt b.txt"

	// operator related extra

	"++i": 9999900011,
	"--i": 9999900015,

	"+i": 9999900101, // add 2 integer values

	"-t": 9999900701, // sub 2 time values, return float seconds

}

type VarRef struct {
	Ref   int // -99 - invalid, -56 - integer label, -31 - clipboard(text), -23 - slice of array/slice, -22 - map item, -21 - array/slice item, -18 - local reg, -17 - reg, -16 - label, -15 - ref, -12 - unref, -11 - seq, -10 - quickEval, -9 - flexEval, -8 - pop, -7 - peek, -6 - push, -5 - tmp, -4 - pln, -3 - value only, -2 - drop, -1 - debug, 3 normal vars
	Value interface{}
}

type Instr struct {
	Code     int
	ParamLen int
	Params   []VarRef
}

type OpCode struct {
	Code     int
	ParamLen int
	Params   []int
}

type FuncContext struct {
	Vars []interface{}

	Tmp interface{}

	DeferStack *tk.SimpleStack
}

type ByteCode struct {
	Labels map[string]int

	Consts []interface{}

	InstrList  []Instr
	OpCodeList []OpCode
}

type VM struct {
	Code *ByteCode

	Regs  []interface{} // the first(index 0) is always global vars map, the second is input param, the third is the return/output value
	Stack *tk.SimpleStack
	Vars  []interface{}

	Seq *tk.Seq

	FuncStack *tk.SimpleStack

	CodePointer int
	// CurrentFunc *FuncContext

	PointerStack *tk.SimpleStack

	ErrorHandler int
}

type CallStruct struct {
	ReturnPointer int
	ReturnRef     VarRef
	Value         interface{}
}

type RangeStruct struct {
	Iterator   tk.Iterator
	LoopIndex  int
	BreakIndex int
}

type LoopStruct struct {
	Cond       interface{}
	LoopIndex  int
	BreakIndex int
	LoopInstr  *Instr
}

func Test() {
	tk.Pl("Test")
}

func NewFuncContext() *FuncContext {
	rs := &FuncContext{}

	rs.Vars = make([]interface{}, 10)

	rs.DeferStack = tk.NewSimpleStack(10, tk.Undefined)

	return rs
}

func NewVM(codeA *ByteCode, inputA ...interface{}) *VM {
	var inputT interface{} = nil

	if len(inputA) > 0 {
		inputT = inputA[0]
	}

	p := &VM{}

	p.Code = codeA

	p.Seq = tk.NewSeq()

	p.Regs = make([]interface{}, 10)
	p.Vars = make([]interface{}, 10)
	p.Stack = tk.NewSimpleStack(10, tk.Undefined)

	p.FuncStack = tk.NewSimpleStack(10, tk.Undefined)
	funcContextT := NewFuncContext()
	p.FuncStack.Push(funcContextT)
	// p.CurrentFunc = funcContextT

	p.CodePointer = 0

	p.PointerStack = tk.NewSimpleStack(10, tk.Undefined)

	p.ErrorHandler = -1

	p.Regs[0] = map[string]interface{}{"undefined": tk.Undefined, "argsG": os.Args}
	p.Regs[1] = inputT

	return p
}

func ParseLine(commandA string) ([]string, error) {
	var args []string

	firstT := true

	// state: 1 - start, quotes - 2, arg - 3
	state := 1
	current := ""
	quote := "`"
	escapeNext := false

	command := []rune(commandA)

	for i := 0; i < len(command); i++ {
		c := command[i]

		if escapeNext {
			// if c == 'n' {
			// 	current += string('\n')
			// } else if c == 'r' {
			// 	current += string('\r')
			// } else if c == 't' {
			// 	current += string('\t')
			// } else {
			current += string(c)
			// }
			escapeNext = false
			continue
		}

		if c == '\\' && state == 2 && quote == "\"" {
			current += string(c)
			escapeNext = true
			continue
		}

		if state == 2 {
			if string(c) != quote {
				current += string(c)
			} else {
				current += string(c) // add it

				args = append(args, current)
				if firstT {
					firstT = false
				}
				current = ""
				state = 1
			}
			continue
		}

		// tk.Pln(string(c), c, c == '`', '`')
		// if state == 1 && (c == '"' || c == '\'' || c == '`') {
		if c == '"' || c == '\'' || c == '`' {
			state = 2
			quote = string(c)

			current += string(c) // add it

			continue
		}

		if state == 3 {
			if c == ' ' || c == '\t' {
				args = append(args, current)
				if firstT {
					firstT = false
				}
				current = ""
				state = 1
			} else {
				current += string(c)
			}
			// Pl("state: %v, current: %v, args: %v", state, current, args)
			continue
		}

		if c != ' ' && c != '\t' {
			state = 3
			current += string(c)
		}
	}

	if state == 2 {
		return []string{}, fmt.Errorf("unclosed quotes: %v", string(command))
	}

	if current != "" {
		args = append(args, current)
		if firstT {
			firstT = false
		}
	}

	return args, nil
}

func (p *ByteCode) ParseVar(strA string, optsA ...interface{}) VarRef {
	// tk.Pl("parseVar: %#v", strA)
	s1T := strings.TrimSpace(strA)

	if strings.HasPrefix(s1T, "`") && strings.HasSuffix(s1T, "`") {
		s1T = s1T[1 : len(s1T)-1]

		return VarRef{-3, s1T} // value(string)
	} else if strings.HasPrefix(s1T, `"`) && strings.HasSuffix(s1T, `"`) { // quoted string
		tmps, errT := strconv.Unquote(s1T)

		if errT != nil {
			return VarRef{-3, s1T}
		}

		return VarRef{-3, tmps} // value(string)
	} else {
		if strings.HasPrefix(s1T, "$") {
			numT, errT := tk.StrToIntQuick(s1T[1:])

			if errT == nil {
				return VarRef{3, numT}
			}

			if s1T == "$drop" || s1T == "_" {
				return VarRef{-2, nil}
			} else if s1T == "$debug" {
				return VarRef{-1, nil}
			} else if s1T == "$pln" {
				return VarRef{-4, nil}
			} else if s1T == "$pop" {
				return VarRef{-8, nil}
			} else if s1T == "$peek" {
				return VarRef{-7, nil}
			} else if s1T == "$push" {
				return VarRef{-6, nil}
			} else if s1T == "$tmp" {
				return VarRef{-5, nil}
			} else if s1T == "$seq" {
				return VarRef{-11, nil}
			} else if s1T == "$clip" {
				return VarRef{-31, nil}
			}

			// } else if strings.HasPrefix(s1T, "&") { // ref
			// 	vNameT := s1T[1:]

			// 	if len(vNameT) < 1 {
			// 		return VarRef{-3, s1T}
			// 	}

			// 	return VarRef{-15, ParseVar(vNameT)}
			// } else if strings.HasPrefix(s1T, "*") { // unref
			// 	vNameT := s1T[1:]

			// 	if len(vNameT) < 1 {
			// 		return VarRef{-3, s1T}
			// 	}

			// 	return VarRef{-12, ParseVar(vNameT)}
		} else if strings.HasPrefix(s1T, ":") { // labels
			vNameT := s1T[1:]

			if len(vNameT) < 1 {
				return VarRef{-3, s1T}
			}

			c1, ok := p.Labels[vNameT]

			if ok {
				return VarRef{-56, c1}
			}

			return VarRef{-16, vNameT}
		} else if strings.HasPrefix(s1T, "#") { // values
			if len(s1T) < 2 {
				return VarRef{-3, s1T}
			}

			// remainsT := s1T[2:]

			typeT := s1T[1]

			if typeT == 'i' { // int
				c1T, errT := tk.StrToIntQuick(s1T[2:])

				if errT != nil {
					return VarRef{-3, s1T}
				}

				return VarRef{-3, c1T}
			} else if typeT == 'f' { // float
				c1T, errT := tk.StrToFloat64E(s1T[2:])

				if errT != nil {
					return VarRef{-3, s1T}
				}

				return VarRef{-3, c1T}
			} else if typeT == 'b' { // bool
				return VarRef{-3, tk.ToBool(s1T[2:])}
			} else if typeT == 'y' { // byte
				return VarRef{-3, tk.ToByte(s1T[2:])}
			} else if typeT == 'x' { // byte in hex from
				return VarRef{-3, byte(tk.HexToInt(s1T[2:]))}
			} else if typeT == 'B' { // single rune (same as in Golang, like 'a'), only first character in string is used
				runesT := []rune(s1T[2:])

				if len(runesT) < 1 {
					return VarRef{-3, s1T[2:]}
				}

				return VarRef{-3, runesT[0]}
			} else if typeT == 'r' { // rune
				return VarRef{-3, tk.ToRune(s1T[2:])}
			} else if typeT == 's' { // string
				s1DT := s1T[2:]

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				return VarRef{-3, s1DT}
			} else if typeT == 'e' { // error
				s1DT := s1T[2:]

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				return VarRef{-3, fmt.Errorf("%v", s1DT)}
			} else if typeT == 't' { // time
				s1DT := s1T[2:]

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				tmps := strings.TrimSpace(s1DT)

				if tmps == "" || tmps == "now" {
					return VarRef{-3, time.Now()}
				}

				rsT := tk.ToTime(tmps)

				if tk.IsError(rsT) {
					return VarRef{-3, s1T}
				}

				return VarRef{-3, rsT}
			} else if typeT == 'J' { // value from JSON
				var objT interface{}

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &objT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, objT}
			} else if typeT == 'L' { // list/array
				var listT []interface{}

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &listT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, listT}
			} else if typeT == 'Y' { // byteList
				var listT []byte

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &listT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, listT}
			} else if typeT == 'R' { // runeList
				var listT []rune

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &listT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, listT}
			} else if typeT == 'S' { // strList/stringList
				var listT []string

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &listT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, listT}
			} else if typeT == 'M' { // map
				var mapT map[string]interface{}

				s1DT := s1T[2:] // tk.UrlDecode(s1T[2:])

				if strings.HasPrefix(s1DT, "`") && strings.HasSuffix(s1DT, "`") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, "'") && strings.HasSuffix(s1DT, "'") {
					s1DT = s1DT[1 : len(s1DT)-1]
				} else if strings.HasPrefix(s1DT, `"`) && strings.HasSuffix(s1DT, `"`) {
					tmps, errT := strconv.Unquote(s1DT)

					if errT != nil {
						return VarRef{-3, s1DT}
					}

					s1DT = tmps

				}

				// tk.Plv(s1T[2:])
				// tk.Plv(s1DT)

				errT := json.Unmarshal([]byte(s1DT), &mapT)
				// tk.Plv(errT)
				if errT != nil {
					return VarRef{-3, s1T}
				}

				// tk.Plv(listT)
				return VarRef{-3, mapT}
			}

			return VarRef{-3, s1T}
		} else if strings.HasPrefix(s1T, "@") { // quickEval
			if len(s1T) < 2 {
				return VarRef{-3, s1T}
			}

			if s1T[1] == '@' { // flexEval
				if len(s1T) < 3 {
					return VarRef{-3, s1T}
				}

				s1T = strings.TrimSpace(s1T[2:])

				if strings.HasPrefix(s1T, "`") && strings.HasSuffix(s1T, "`") {
					s1T = s1T[1 : len(s1T)-1]

					return VarRef{-9, s1T} // flex eval value
				} else if strings.HasPrefix(s1T, "'") && strings.HasSuffix(s1T, "'") {
					s1T = s1T[1 : len(s1T)-1]

					return VarRef{-9, s1T} // flex eval value
				} else if strings.HasPrefix(s1T, `"`) && strings.HasSuffix(s1T, `"`) {
					tmps, errT := strconv.Unquote(s1T)

					if errT != nil {
						return VarRef{-9, s1T}
					}

					return VarRef{-9, tmps}
				}

				return VarRef{-9, s1T}
			}

			s1T = strings.TrimSpace(s1T[1:])

			if strings.HasPrefix(s1T, "`") && strings.HasSuffix(s1T, "`") {
				s1T = s1T[1 : len(s1T)-1]

				return VarRef{-10, s1T} // quick eval value
			} else if strings.HasPrefix(s1T, "'") && strings.HasSuffix(s1T, "'") {
				s1T = s1T[1 : len(s1T)-1]

				return VarRef{-10, s1T} // quick eval value
			} else if strings.HasPrefix(s1T, `"`) && strings.HasSuffix(s1T, `"`) {
				tmps, errT := strconv.Unquote(s1T)

				if errT != nil {
					return VarRef{-10, s1T}
				}

				return VarRef{-10, tmps}
			}

			return VarRef{-10, s1T}
			// } else if strings.HasPrefix(s1T, "^") { // regs
			// 	if len(s1T) < 2 {
			// 		return VarRef{-3, s1T}
			// 	}

			// 	s1T = strings.TrimSpace(s1T[1:])

			// 	return VarRef{-17, tk.ToInt(s1T, 0)}
			// } else if strings.HasPrefix(s1T, "%") { // compiled
			// 	if len(s1T) < 2 {
			// 		return VarRef{-3, s1T}
			// 	}

			// 	s1T = strings.TrimSpace(s1T[1:])

			// 	if strings.HasPrefix(s1T, "`") && strings.HasSuffix(s1T, "`") {
			// 		s1T = s1T[1 : len(s1T)-1]

			// 		nc := Compile(s1T)

			// 		// if tk.IsError(nc) {
			// 		// 	return VarRef{-3, nc}
			// 		// }

			// 		return VarRef{-3, nc} // compiled
			// 	} else if strings.HasPrefix(s1T, "'") && strings.HasSuffix(s1T, "'") {
			// 		s1T = s1T[1 : len(s1T)-1]

			// 		nc := Compile(s1T)

			// 		return VarRef{-3, nc} // compiled
			// 	} else if strings.HasPrefix(s1T, `"`) && strings.HasSuffix(s1T, `"`) {
			// 		tmps, errT := strconv.Unquote(s1T)

			// 		if errT != nil {
			// 			return VarRef{-3, errT}
			// 		}

			// 		nc := Compile(tmps)

			// 		return VarRef{-3, nc} // compiled
			// 	}

			// 	return VarRef{-3, Compile(s1T)}
		} else if strings.HasPrefix(s1T, "[") && strings.HasSuffix(s1T, "]") { // array/slice item
			if len(s1T) < 3 {
				return VarRef{-3, s1T}
			}

			s1aT := strings.TrimSpace(s1T[1 : len(s1T)-1])

			listT := strings.Split(s1aT, ",")

			len2T := len(listT)

			if len2T >= 3 { // slice of array/slice/string
				vT := p.ParseVar(listT[0])

				itemKeyT := listT[1]

				if strings.HasPrefix(itemKeyT, "`") && strings.HasSuffix(itemKeyT, "`") {
					itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
				} else if strings.HasPrefix(itemKeyT, "'") && strings.HasSuffix(itemKeyT, "'") {
					itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
				} else if strings.HasPrefix(itemKeyT, `"`) && strings.HasSuffix(itemKeyT, `"`) {
					tmps, errT := strconv.Unquote(itemKeyT)

					if errT != nil {
						itemKeyT = tmps
					}
				}

				itemKeyEndT := listT[2]

				if strings.HasPrefix(itemKeyEndT, "`") && strings.HasSuffix(itemKeyEndT, "`") {
					itemKeyEndT = itemKeyEndT[1 : len(itemKeyEndT)-1]
				} else if strings.HasPrefix(itemKeyEndT, "'") && strings.HasSuffix(itemKeyEndT, "'") {
					itemKeyEndT = itemKeyEndT[1 : len(itemKeyEndT)-1]
				} else if strings.HasPrefix(itemKeyEndT, `"`) && strings.HasSuffix(itemKeyEndT, `"`) {
					tmps, errT := strconv.Unquote(itemKeyEndT)

					if errT != nil {
						itemKeyEndT = tmps
					}
				}

				return VarRef{-23, []interface{}{vT, p.ParseVar(itemKeyT), p.ParseVar(itemKeyEndT)}}

			}

			if len2T < 2 {
				listT = strings.SplitN(s1aT, "|", 2)

				if len(listT) < 2 {
					return VarRef{-3, s1T}
				}
			}

			vT := p.ParseVar(listT[0])

			itemKeyT := listT[1]

			if strings.HasPrefix(itemKeyT, "`") && strings.HasSuffix(itemKeyT, "`") {
				itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
			} else if strings.HasPrefix(itemKeyT, "'") && strings.HasSuffix(itemKeyT, "'") {
				itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
			} else if strings.HasPrefix(itemKeyT, `"`) && strings.HasSuffix(itemKeyT, `"`) {
				tmps, errT := strconv.Unquote(itemKeyT)

				if errT != nil {
					itemKeyT = tmps
				}
			}

			return VarRef{-21, []interface{}{vT, p.ParseVar(itemKeyT)}}
		} else if strings.HasPrefix(s1T, "{") && strings.HasSuffix(s1T, "}") { // map item
			if len(s1T) < 3 {
				return VarRef{-3, s1T}
			}

			s1aT := strings.TrimSpace(s1T[1 : len(s1T)-1])

			listT := strings.SplitN(s1aT, ",", 2)

			if len(listT) < 2 {
				listT = strings.SplitN(s1aT, "|", 2)

				if len(listT) < 2 {
					return VarRef{-3, s1T}
				}
			}

			vT := p.ParseVar(listT[0])

			itemKeyT := listT[1]

			if strings.HasPrefix(itemKeyT, "`") && strings.HasSuffix(itemKeyT, "`") {
				itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
			} else if strings.HasPrefix(itemKeyT, "'") && strings.HasSuffix(itemKeyT, "'") {
				itemKeyT = itemKeyT[1 : len(itemKeyT)-1]
			} else if strings.HasPrefix(itemKeyT, `"`) && strings.HasSuffix(itemKeyT, `"`) {
				tmps, errT := strconv.Unquote(itemKeyT)

				if errT != nil {
					itemKeyT = tmps
				}
			}

			// tk.Pl("itemKeyT: %v", itemKeyT)

			return VarRef{-22, []interface{}{vT, p.ParseVar(itemKeyT)}}
		}
	}

	return VarRef{-3, s1T}
}

func Compile(scriptA string) (*ByteCode, error) {
	if DebugG {
		tk.Pl("compiling: %#v", scriptA)
	}

	p := &ByteCode{}

	p.Consts = make([]interface{}, 0)
	p.Labels = make(map[string]int)
	p.InstrList = make([]Instr, 0)
	p.OpCodeList = make([]OpCode, 0)

	originCodeLenT := 0

	sourceT := tk.SplitLines(scriptA)

	pointerT := originCodeLenT

	codeListT := make([]string, 0, 10)

	for i := 0; i < len(sourceT); i++ {
		v := strings.TrimSpace(sourceT[i])

		if tk.StartsWith(v, "//") || tk.StartsWith(v, "#") {
			continue
		}

		if tk.StartsWith(v, ":") {
			labelT := strings.TrimSpace(v[1:])

			_, ok := p.Labels[labelT]

			if !ok {
				p.Labels[labelT] = pointerT
			} else {
				return nil, fmt.Errorf("failed to compile(line %v %v): duplicated label", i+1, tk.LimitString(sourceT[i], 50))
			}

			continue
		}

		if tk.Contains(v, "`") {
			if strings.Count(v, "`")%2 != 0 {
				foundT := false
				var j int
				for j = i + 1; j < len(sourceT); j++ {
					if tk.Contains(sourceT[j], "`") {
						v = tk.JoinLines(sourceT[i : j+1])
						foundT = true
						break
					}
				}

				if !foundT {
					return nil, fmt.Errorf("failed to analyze the code: ` not paired(%v)", i)
				}

				i = j
			}
		}

		v = strings.TrimSpace(v)

		if v == "" {
			continue
		}

		codeListT = append(codeListT, v)
		pointerT++
	}

	if DebugG {
		tk.Pl("codeLisT: %#v", codeListT)
	}

	for i := originCodeLenT; i < len(codeListT); i++ {
		// listT := strings.SplitN(v, " ", 3)
		v := codeListT[i]

		listT, errT := ParseLine(v)
		if errT != nil {
			return nil, fmt.Errorf("failed to parse parmaters: %v", errT)
		}

		if DebugG {
			tk.Plv(listT)
		}

		lenT := len(listT)

		instrNameT := strings.TrimSpace(listT[0])

		codeT, ok := InstrNameSet[instrNameT]

		if !ok {
			instrT := Instr{Code: codeT, ParamLen: 1, Params: []VarRef{VarRef{Ref: -3, Value: v}}}
			p.InstrList = append(p.InstrList, instrT)

			return nil, fmt.Errorf("compile error(line %v %v): unknown instr", i, tk.LimitString(v, 50))
		}

		instrT := Instr{Code: codeT, Params: make([]VarRef, 0, lenT-1)}

		list3T := []VarRef{}

		for j, jv := range listT {
			if j == 0 {
				continue
			}

			list3T = append(list3T, p.ParseVar(jv, i))
		}

		instrT.Params = append(instrT.Params, list3T...)
		instrT.ParamLen = lenT - 1

		p.InstrList = append(p.InstrList, instrT)
	}

	// tk.Plv(p.SourceM)
	// tk.Plv(p.CodeListM)
	// tk.Plv(p.CodeSourceMapM)

	return p, nil
}

func (p *VM) GetCurrentFuncContext() *FuncContext {
	tk.Pl("GetCurrentFuncContext: %#v", p.FuncStack)
	if p.FuncStack.Size() < 1 {
		return nil
	}

	return p.FuncStack.Peek().(*FuncContext)
}

func (p *VM) SetVar(refA VarRef, setValueA interface{}) error {
	// tk.Pln(refA, "->", setValueA)

	refIntT := refA.Ref

	if refIntT == -2 { // $drop
		return nil
	}

	if refIntT == -4 { // $pln
		fmt.Println(setValueA)
		return nil
	}

	if refIntT == -5 { // $tmp
		p.GetCurrentFuncContext().Tmp = setValueA
		return nil
	}

	if refIntT == -6 { // $push
		p.Stack.Push(setValueA)
		return nil
	}

	if refIntT == -11 { // $seq
		p.Seq.Reset(tk.ToInt(setValueA, 0))
		return nil
	}

	if refIntT == -31 { // $clip
		tk.SetClipText(tk.ToStr(setValueA))
		return nil
	}

	if refIntT == -17 { // regs
		p.Regs[refA.Value.(int)] = setValueA
		return nil
	}

	// if refIntT == -18 { // local regs
	// 	lenT := runA.FuncStack.Size()

	// 	if lenT > 0 {
	// 		funcT := runA.FuncStack.PeekLayer(lenT - 1).(*FuncContext)
	// 		funcT.Regs[refT.Value.(int)] = setValueA
	// 		return nil
	// 	}

	// 	p.RootFunc.Regs[refT.Value.(int)] = setValueA
	// 	return nil
	// }

	// if refIntT == -12 { // unref
	// 	return nil
	// }

	// if refIntT == -15 { // ref
	// 	errT := tk.SetByRef(p.GetVarValue(runA, refT.Value.(VarRef)), setValueA)

	// 	if errT != nil {
	// 		return errT
	// 	}

	// 	return nil
	// }

	if refIntT != 3 {
		return fmt.Errorf("unsupported var reference")
	}

	lenT := p.FuncStack.Size()

	keyT := refA.Value.(int)

	for idxT := lenT - 1; idxT >= 0; idxT-- {
		loopFunc := p.FuncStack.PeekLayer(idxT).(*FuncContext)

		loopFunc.Vars[keyT] = setValueA
		return nil
	}

	return nil
}

func (p *VM) GetVarValue(vA VarRef) interface{} {
	idxT := vA.Ref

	if idxT == -2 {
		return tk.Undefined
	}

	if idxT == -3 {
		return vA.Value
	}

	if idxT == -5 {
		return p.GetCurrentFuncContext().Tmp
	}

	if idxT == -11 {
		return p.Seq.Get()
	}

	if idxT == -31 {
		return tk.GetClipboardTextDefaultEmpty()
	}

	if idxT == -8 {
		return p.Stack.Pop()
	}

	if idxT == -7 {
		return p.Stack.Peek()
	}

	if idxT == -1 { // $debug
		return tk.ToJSONX(p, "-indent", "-sort")
	}

	if idxT == -6 {
		return tk.Undefined
	}

	// if idxT == -9 {
	// 	return tk.FlexEvalMap(tk.ToStr(vA.Value), p.GetVarValue(runA, ParseVar("$flexEvalEnvG")))
	// }

	// if idxT == -10 {
	// 	// tk.Pln("getvarvalue", vA.Value)
	// 	return QuickEval(tk.ToStr(vA.Value), p, runA)
	// }

	if idxT == -16 { // labels
		return p.GetLabelIndex(vA.Value)
	}

	if idxT == -17 { // regs
		return p.Regs[vA.Value.(int)]
	}

	// if idxT == -18 { // local regs
	// 	lenT := runA.FuncStack.Size()

	// 	if lenT > 0 {
	// 		funcT := runA.FuncStack.PeekLayer(lenT - 1).(*FuncContext)
	// 		return funcT.Regs[vA.Value.(int)]
	// 	}

	// 	return p.RootFunc.Regs[vA.Value.(int)]
	// }

	if idxT == -21 { // array/slice item
		nv := vA.Value.([]interface{})
		return tk.GetArrayItem(p.GetVarValue(nv[0].(VarRef)), tk.ToInt(p.GetVarValue(nv[1].(VarRef)), 0))
	}

	if idxT == -22 { // map item
		nv := vA.Value.([]interface{})
		return tk.GetMapItem(p.GetVarValue(nv[0].(VarRef)), p.GetVarValue(nv[1].(VarRef)))
	}

	if idxT == -23 { // slice of array/slice
		nv := vA.Value.([]interface{})
		return tk.GetArraySlice(p.GetVarValue(nv[0].(VarRef)), tk.ToInt(p.GetVarValue(nv[1].(VarRef)), 0), tk.ToInt(p.GetVarValue(nv[2].(VarRef)), 0))
	}

	// if idxT == -12 { // unref
	// 	rs, errT := tk.GetRefValue(p.GetVarValue(runA, vA.Value.(VarRef)))

	// 	if errT != nil {
	// 		return tk.Undefined
	// 	}

	// 	return rs
	// }

	// if idxT == -15 { // ref
	// 	return tk.Undefined
	// }

	if idxT == -56 { // integer labels
		return vA.Value
	}

	if idxT == 3 { // normal variables
		lenT := p.FuncStack.Size()

		for idxT := lenT - 1; idxT >= 0; idxT-- {
			loopFunc := p.FuncStack.PeekLayer(idxT).(*FuncContext)
			nv := loopFunc.Vars[vA.Value.(int)]

			return nv
		}

		return tk.Undefined

	}

	return tk.Undefined

}

func (p *VM) ParamsToStrs(v *Instr, fromA int) []string {
	lenT := len(v.Params)

	sl := make([]string, 0, lenT)

	for i := fromA; i < lenT; i++ {
		sl = append(sl, tk.ToStr(p.GetVarValue(v.Params[i])))
	}

	return sl
}

func (p *VM) ParamsToInts(v *Instr, fromA int) []int {
	lenT := len(v.Params)

	sl := make([]int, 0, lenT)

	for i := fromA; i < lenT; i++ {
		sl = append(sl, tk.ToInt(p.GetVarValue(v.Params[i]), 0))
	}

	return sl
}

func (p *VM) ParamsToList(v *Instr, fromA int) []interface{} {
	lenT := len(v.Params)

	sl := make([]interface{}, 0, lenT)

	for i := fromA; i < lenT; i++ {
		sl = append(sl, p.GetVarValue(v.Params[i]))
	}

	return sl
}

func (p *VM) GetLabelIndex(inputA interface{}) int {
	// tk.Pl("GetLabelIndex: %#v", inputA)
	c, ok := inputA.(int)

	if ok {
		return c
	}

	s2 := tk.ToStr(inputA)

	if strings.HasPrefix(s2, ":") {
		s2 = s2[1:]
	}

	if len(s2) > 1 {
		if strings.HasPrefix(s2, "+") {
			return p.CodePointer + tk.ToInt(s2[1:])
		} else if strings.HasPrefix(s2, "-") {
			return p.CodePointer - tk.ToInt(s2[1:])
		} else {
			labelPointerT, ok := p.Code.Labels[s2]

			if ok {
				return labelPointerT
			}
		}
	}

	return -1
}

func RunInstr(p *VM, instrA *Instr) (resultR interface{}) {
	// startT := time.Now()

	defer func() {
		// endT := time.Now()

		// tk.Pl("instr(%#v) dur: %d", instrA.Code, endT.Sub(startT))

		if r1 := recover(); r1 != nil {
			// tk.Printfln("exception: %v", r)
			// if p.ErrorHandler > -1 {
			// 	p.SetVarGlobal("lastLineG", r.CodeSourceMap[r.CodePointer]+1)
			// 	p.SetVarGlobal("errorMessageG", tk.ToStr(r))
			// 	p.SetVarGlobal("errorDetailG", fmt.Errorf("runtime error: %v\n%v", r, string(debug.Stack())))

			// 	// p.Stack.Push(fmt.Errorf("runtime error: %v\n%v", r, string(debug.Stack())))
			// 	// p.Stack.Push(tk.ToStr(r))
			// 	// p.Stack.Push(r.CodeSourceMap[r.CodePointer] + 1)
			// 	resultR = r.ErrorHandler
			// 	return
			// }

			resultR = fmt.Errorf("runtime exception: %v\n%v", r1, string(debug.Stack()))

			return
		}
	}()

	var instrT *Instr = instrA

	// if p.VerbosePlusM {
	// tk.Plv(instrT)
	// }

	if instrT == nil {
		return fmt.Errorf("nil instr: %v", instrT)
	}

	cmdT := instrT.Code

	switch cmdT {
	case 12: // invalidInstr
		return fmt.Errorf("invalid instr: %v", instrT.Params[0].Value)
	case 100: // version
		pr := instrT.Params[0]

		p.SetVar(pr, VersionG)

		return ""
	case 101: // pass
		return ""
	case 122: // testByText
		if instrT.ParamLen < 2 {
			return p.Errf("not enough parameters(参数不够)")
		}

		v1 := tk.ToStr(p.GetVarValue(instrT.Params[0]))
		v2 := tk.ToStr(p.GetVarValue(instrT.Params[1]))

		// tk.Plo("--->", v2)

		var v3 string
		var v4 string

		if instrT.ParamLen > 3 {
			v3 = tk.ToStr(p.GetVarValue(instrT.Params[2]))
			v4 = "(" + tk.ToStr(p.GetVarValue(instrT.Params[3])) + ")"
		} else if instrT.ParamLen > 2 {
			v3 = tk.ToStr(p.GetVarValue(instrT.Params[2]))
		} else {
			v3 = tk.ToStr(tk.AutoSeq.Get())
		}

		if v1 == v2 {
			tk.Pl("test %v%v passed", v3, v4)
		} else {
			return p.Errf("test %v%v failed: (pos: %v) %#v <-> %#v\n-----\n%v\n-----\n%v", v3, v4, tk.FindFirstDiffIndex(v1, v2), v1, v2, v1, v2)
		}

		return ""

	case 180: // goto
		if instrT.ParamLen < 1 {
			return p.Errf("not enough parameters")
		}

		v1 := p.GetVarValue(instrT.Params[0]).(int)

		// c1 := p.GetLabelIndex(v1)

		if v1 >= 0 {
			return v1
		}

		return p.Errf("invalid label: %v", v1)

	case 199: // exit
		if instrT.ParamLen < 1 {
			return "exit"
		}

		valueT := p.GetVarValue(instrT.Params[0])

		p.Regs[2] = valueT

		return "exit"

	case 220: // push
		if instrT.ParamLen < 1 {
			return p.Errf("not enough parameters")
		}

		v1 := p.GetVarValue(instrT.Params[0])

		p.Stack.Push(v1)

		return ""
	case 222: // peek
		if instrT.ParamLen < 1 {
			return p.Errf("not enough parameters")
		}

		pr := instrT.Params[0]

		errT := p.SetVar(pr, p.Stack.Peek())

		if errT != nil {
			return p.Errf("%v", errT)
		}

		return ""

	case 224: // pop
		pr := instrT.Params[0]

		p.SetVar(pr, p.Stack.Pop())

		return ""
	case 401: // assign/=
		if instrT.ParamLen < 2 {
			return fmt.Errorf("not enough parameters")
		}

		pr := instrT.Params[0]

		var valueT interface{}

		valueT = p.GetVarValue(instrT.Params[1])

		p.SetVar(pr, valueT)

		return ""

	case 610: // if
		// tk.Plv(instrT)
		if instrT.ParamLen < 2 {
			return p.Errf("not enough parameters")
		}

		var condT bool
		var v2 interface{}
		var v2o interface{}
		var ok0 bool

		var elseLabelIntT int = -1

		if instrT.ParamLen > 2 {
			elseLabelT := p.GetLabelIndex(p.GetVarValue(instrT.Params[2]))

			if elseLabelT < 0 {
				return p.Errf("invalid label: %v", elseLabelT)
			}

			elseLabelIntT = elseLabelT
		}

		v2o = instrT.Params[1]

		v2 = p.GetVarValue(instrT.Params[1])

		// tk.Plv(instrT)
		tmpv := p.GetVarValue(instrT.Params[0])
		if DebugG {
			tk.Pl("if %v -> %v", instrT.Params[0], tmpv)
		}

		condT, ok0 = tmpv.(bool)

		// if !ok0 {
		// 	var tmps string
		// 	tmps, ok0 = tmpv.(string)

		// 	if ok0 {
		// 		tmprs := QuickEval(tmps, p, r)

		// 		condT, ok0 = tmprs.(bool)
		// 	}
		// }

		if !ok0 {
			return p.Errf("invalid condition parameter: %#v", tmpv)
		}

		if condT {
			c2 := p.GetLabelIndex(v2)

			if c2 < 0 {
				return p.Errf("invalid label: %v", v2o)
			}

			return c2
		}

		if elseLabelIntT >= 0 {
			return elseLabelIntT
		}

		return ""

	case 703: // <
		pr := instrT.Params[0]
		v1 := p.GetVarValue(instrT.Params[1])
		v2 := p.GetVarValue(instrT.Params[2])

		v3 := tk.GetLTResult(v1, v2)

		p.SetVar(pr, v3)
		return ""

	case 1010: // call
		if instrT.ParamLen < 2 {
			return p.Errf("not enough paramters")
		}

		pr := instrT.Params[0]

		v1p := 1

		v1 := p.GetVarValue(instrT.Params[v1p])

		v1c := p.GetLabelIndex(v1)

		if v1c < 0 {
			return p.Errf("invalid label format: %v", v1)
		}

		p.PointerStack.Push(CallStruct{ReturnPointer: p.CodePointer, ReturnRef: pr})

		funcContextT := NewFuncContext()

		if instrT.ParamLen > 2 {
			vs := p.ParamsToList(instrT, 2)

			funcContextT.Vars[1] = vs
		}

		p.FuncStack.Push(funcContextT)

		return v1c

	case 1020: // ret
		rs := p.PointerStack.Pop()

		if tk.IsUndefined(rs) {
			return p.Errf("pointer stack empty")
		}

		nv, ok := rs.(CallStruct)

		if !ok {
			return p.Errf("not in a call, not a call struct in running stack: %v", rs)
		}

		currentFuncT := p.GetCurrentFuncContext()

		rsi := currentFuncT.RunDefer(p)

		if tk.IsError(rsi) {
			return p.Errf("[%v](qxlang) runtime error: %v", tk.GetNowTimeStringFormal(), rsi)
		}

		if instrT.ParamLen > 0 {
			// tk.Pl("outL <-: %#v", p.GetVarValue(instrT.Params[0]))
			currentFuncT.Vars[2] = p.GetVarValue(instrT.Params[0])
		}

		rs2 := currentFuncT.Vars[2]

		funcContextItemT := p.FuncStack.Pop()

		if tk.IsUndefined(funcContextItemT) {
			return p.Errf("failed to return from function call while pop func: %v", "no function in func stack")
		}

		pr := nv.ReturnRef

		if rs2 != nil && rs2 != tk.Undefined {
			p.SetVar(pr, rs2)
		} else {
			p.SetVar(pr, tk.Undefined)
		}

		return nv.ReturnPointer + 1

	case 1123: // getArrayItem/[]
		if instrT.ParamLen < 3 {
			return p.Errf("not enough parameters")
		}

		var pr = instrT.Params[0]

		v1 := p.GetVarValue(instrT.Params[1])

		if v1 == nil {
			if instrT.ParamLen > 3 {
				p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
				return ""
			} else {
				return p.Errf("object is nil: (%T)%v", v1, v1)
			}

		}

		v2 := tk.ToInt(p.GetVarValue(instrT.Params[2]))

		// tk.Pl("v1: %#v, v2: %#v, instr: %#v", v1, v2, instrT)

		switch nv := v1.(type) {
		case []interface{}:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			// tk.Pl("r: %v", nv[v2])
			p.SetVar(pr, nv[v2])
		case []bool:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []int:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []byte:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []rune:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []int64:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []float64:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []string:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}
			p.SetVar(pr, nv[v2])
		case []map[string]string:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}

			p.SetVar(pr, nv[v2])
		case []map[string]interface{}:
			if (v2 < 0) || (v2 >= len(nv)) {
				if instrT.ParamLen > 3 {
					p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
					return ""
				} else {
					return p.Errf("index out of range: %v/%v", v2, len(nv))
				}
			}

			p.SetVar(pr, nv[v2])
		default:
			valueT := reflect.ValueOf(v1)

			kindT := valueT.Kind()

			if kindT == reflect.Array || kindT == reflect.Slice || kindT == reflect.String {
				lenT := valueT.Len()

				if (v2 < 0) || (v2 >= lenT) {
					return p.Errf("index out of range: %v/%v", v2, lenT)
				}

				p.SetVar(pr, valueT.Index(v2).Interface())
				return ""
			}

			if instrT.ParamLen > 3 {
				p.SetVar(pr, p.GetVarValue(instrT.Params[3]))
			} else {
				p.SetVar(pr, tk.Undefined)
			}

			return p.Errf("parameter types not match: %#v", v1)
		}

		return ""

	case 1910: // now

		pr := instrT.Params[0]

		errT := p.SetVar(pr, time.Now())

		if errT != nil {
			return p.Errf("%v", errT)
		}

		return ""

	case 10410: // pln
		list1T := []interface{}{}

		for _, v := range instrT.Params {
			list1T = append(list1T, p.GetVarValue(v))
		}

		fmt.Println(list1T...)

		return ""

	case 10411: // plo
		vs := p.ParamsToList(instrT, 0)

		tk.Plo(vs...)

		return ""

	case 20601: // systemCmd
		if instrT.ParamLen < 2 {
			return p.Errf("not enough parameters")
		}

		pr := instrT.Params[0]
		v1p := 1

		v1 := tk.ToStr(p.GetVarValue(instrT.Params[v1p]))

		optsA := p.ParamsToStrs(instrT, v1p+1)

		// tk.Pln(v1, ",", optsA)

		p.SetVar(pr, tk.SystemCmd(v1, optsA...))

		return ""

	case 9999900015: // --i
		if instrT.ParamLen < 1 {
			return p.Errf("not enough parameters")
		}

		pr := instrT.Params[0]
		v1p := 0

		v1 := p.GetVarValue(instrT.Params[v1p])

		nv, ok := v1.(int)

		if ok {
			p.SetVar(pr, nv-1)
			return ""
		}

		p.SetVar(pr, tk.ToInt(v1)-1)

		return ""

	case 9999900101: // +i
		// tk.Plv(instrT)
		if instrT.ParamLen < 2 {
			return p.Errf("not enough parameters")
		}

		pr := instrT.Params[0]
		v1p := 1

		v1 := p.GetVarValue(instrT.Params[v1p]).(int)
		v2 := p.GetVarValue(instrT.Params[v1p+1]).(int)

		v3 := v1 + v2

		p.SetVar(pr, v3)

		return ""

	case 9999900701: // -t
		// tk.Plv(instrT)
		if instrT.ParamLen < 2 {
			return p.Errf("not enough parameters")
		}

		pr := instrT.Params[0]
		v1p := 1

		v1 := p.GetVarValue(instrT.Params[v1p]).(time.Time)
		v2 := p.GetVarValue(instrT.Params[v1p+1]).(time.Time)

		v3 := v1.Sub(v2)

		p.SetVar(pr, v3.Seconds())

		return ""

	}

	return fmt.Errorf("unknown instr: %v", instrT)
}

func (p *FuncContext) RunDefer(vmA *VM) error {
	for {
		instrT := p.DeferStack.Pop()

		// tk.Pl("\nDeferStack.Pop: %#v\n", instrT)

		if instrT == nil || tk.IsUndefined(instrT) {
			break
		}

		nv, ok := instrT.(*Instr)

		if !ok {
			nvv, ok := instrT.(Instr)

			if ok {
				nv = &nvv
			} else {
				return fmt.Errorf("invalid instruction: %#v", instrT)
			}
		}

		// if GlobalsG.VerboseLevel > 1 {
		// 	tk.Pl("defer run: %v", nv)
		// }

		rs := RunInstr(vmA, nv)

		if tk.IsError(rs) {
			return fmt.Errorf("[%v](xie) runtime error: %v", tk.GetNowTimeStringFormal(), tk.GetErrStrX(rs))
		}
	}

	return nil
}

func (p *VM) Errf(formatA string, argsA ...interface{}) error {
	return fmt.Errorf(formatA, argsA...)
}

func (p *VM) RunDeferUpToRoot() error {
	// if p.Parent == nil {
	// 	return fmt.Errorf("no parent VM: %v", p.Parent)
	// }

	lenT := p.FuncStack.Size()

	if lenT < 1 {
		return nil
	}

	for i := lenT - 1; i >= 0; i-- {
		contextT := p.FuncStack.PeekLayer(i).(*FuncContext)

		rs := contextT.RunDefer(p)

		if tk.IsError(rs) {
			return rs
		}
	}

	return nil

}

func (p *VM) Run(posA ...int) interface{} {
	// tk.Pl("%#v", p)
	p.CodePointer = 0
	if len(posA) > 0 {
		p.CodePointer = posA[0]
	}

	if len(p.Code.InstrList) < 1 {
		return tk.Undefined
	}

	for {
		// if GlobalsG.VerboseLevel > 1 {
		// 	tk.Pl("-- RunInstr [%v] %v", p.Running.CodePointer, tk.LimitString(p.Running.Source[p.Running.CodeSourceMap[p.Running.CodePointer]], 50))
		// }

		resultT := RunInstr(p, &p.Code.InstrList[p.CodePointer])

		c1T, ok := resultT.(int)

		if ok {
			p.CodePointer = c1T
		} else {
			if tk.IsError(resultT) {
				if p.ErrorHandler > -1 {
					// p.SetVarGlobal("lastLineG", p.Running.CodeSourceMap[p.Running.CodePointer]+1)
					// p.SetVarGlobal("errorMessageG", "runtime error")
					// p.SetVarGlobal("errorDetailG", tk.GetErrStrX(resultT))
					// p.Stack.Push(tk.GetErrStrX(resultT))
					// p.Stack.Push("runtime error")
					// p.Stack.Push(p.Running.CodeSourceMap[p.Running.CodePointer] + 1)

					p.CodePointer = p.ErrorHandler

					continue
				}
				// tk.Plo(1.2, p.Running, p.RootFunc)
				p.RunDeferUpToRoot()
				return fmt.Errorf("[%v](qxlang) runtime error: %v", tk.GetNowTimeStringFormal(), tk.GetErrStrX(resultT))
				// tk.Pl("[%v](xie) runtime error: %v", tk.GetNowTimeStringFormal(), p.CodeSourceMapM[p.CodePointerM]+1, tk.GetErrStr(rs))
				// break
			}

			rs, ok := resultT.(string)

			if !ok {
				p.RunDeferUpToRoot()
				return fmt.Errorf("return result error: (%T)%v", resultT, resultT)
			}

			if tk.IsErrStrX(rs) {
				p.RunDeferUpToRoot()
				return fmt.Errorf("[%v](qxlang) runtime error: %v", tk.GetNowTimeStringFormal(), tk.GetErrStr(rs))
				// tk.Pl("[%v](xie) runtime error: %v", tk.GetNowTimeStringFormal(), p.CodeSourceMapM[p.CodePointerM]+1, tk.GetErrStr(rs))
				// break
			}

			if rs == "" {
				p.CodePointer++

				if p.CodePointer >= len(p.Code.InstrList) {
					break
				}
			} else if rs == "exit" {
				break
				// } else if rs == "cont" {
				// 	return p.Errf("无效指令: %v", rs)
				// } else if rs == "brk" {
				// 	return p.Errf("无效指令: %v", rs)
			} else {
				tmpI := tk.StrToInt(rs)

				if tmpI < 0 {
					p.RunDeferUpToRoot()

					return fmt.Errorf("invalid instr: %v", rs)
				}

				if tmpI >= len(p.Code.InstrList) {
					p.RunDeferUpToRoot()
					return fmt.Errorf("instr index out of range: %v(%v)/%v", tmpI, rs, len(p.Code.InstrList))
				}

				p.CodePointer = tmpI
			}

		}

	}

	rsi := p.RunDeferUpToRoot()

	if tk.IsErrX(rsi) {
		return tk.ErrStrf("[%v](qxlang) runtime error: %v", tk.GetNowTimeStringFormal(), tk.GetErrStrX(rsi))
	}

	// tk.Pl(tk.ToJSONX(p, "-indent", "-sort"))

	outT := p.Regs[2]
	if outT == nil {
		return tk.Undefined
	}

	return outT

}

func (p *ByteCode) DealInputParams(instrA *Instr, startA int) error {
	var jvn VarRef

	for i := instrA.ParamLen - 1; i >= startA; i-- {
		jvn = instrA.Params[i]
		switch jvn.Ref {
		case -3:
			p.Consts = append(p.Consts, jvn.Value)

			p.OpCodeList = append(p.OpCodeList, OpCode{Code: OpConst, ParamLen: 1, Params: []int{len(p.Consts) - 1}})
		case 3:
			p.OpCodeList = append(p.OpCodeList, OpCode{Code: OpGetLocalVarValue, ParamLen: 1, Params: []int{jvn.Value.(int)}})
		}

	}

	return nil
}

func (p *ByteCode) DealOutputParams(instrA *Instr, startA int) error {
	var jvn VarRef

	for i := 0; i <= startA; i++ {
		jvn = instrA.Params[i]
		switch jvn.Ref {
		case 3:
			p.OpCodeList = append(p.OpCodeList, OpCode{Code: OpAssignLocal, ParamLen: 1, Params: []int{jvn.Value.(int)}})
		}

	}

	return nil
}

func plDebug(formatA string, argsA ...interface{}) {
	if DebugG {
		tk.Pl(formatA, argsA...)
	}
}

func (p *ByteCode) DeepCompile() error {
	p.OpCodeList = make([]OpCode, 0, len(p.InstrList))
	for _, v := range p.InstrList {
		switch v.Code {
		case 401: // =
			p.DealInputParams(&v, 1)

			p.DealOutputParams(&v, 0)

		case 9999900101: // +i
			p.DealInputParams(&v, 1)

			p.OpCodeList = append(p.OpCodeList, OpCode{Code: OpAddInt})

			p.DealOutputParams(&v, 0)
		}
	}

	tk.Pl("Consts: %#v", p.Consts)
	tk.Pl("OpCodeList: %v", tk.ToJSONX(p.OpCodeList, "-sort", "-indent"))

	return nil
}

func (p *VM) RunOpCodes() (resultR interface{}) {
	p.CodePointer = 0

	var opCodeT OpCode

	for {
		opCodeT = p.Code.OpCodeList[p.CodePointer]

		plDebug("run op: %#v", opCodeT)

		switch opCodeT.Code {
		case OpConst:
			plDebug("start stack: %#v", p.Stack)
			p.Stack.Push(p.Code.Consts[opCodeT.Params[0]])
			plDebug("end stack: %#v", p.Stack)
		case OpAssignLocal:
			plDebug("start stack: %#v", p.Stack)
			p.GetCurrentFuncContext().Vars[opCodeT.Params[0]] = p.Stack.Pop()
			plDebug("end stack: %#v", p.Stack)
		case OpGetLocalVarValue:
			plDebug("start stack: %#v", p.Stack)
			p.Stack.Push(p.GetCurrentFuncContext().Vars[opCodeT.Params[0]])
			plDebug("end stack: %#v", p.Stack)
		case OpAddInt:
			plDebug("start stack: %#v", p.Stack)
			p.Stack.Push(p.Stack.Pop().(int) + p.Stack.Pop().(int))
			plDebug("end stack: %#v", p.Stack)
		}

		plDebug("")

		p.CodePointer++

		if p.CodePointer >= len(p.Code.OpCodeList) {
			break
		}
	}

	resultR = nil
	return
}

func RunCode(scriptA string, optsA ...string) interface{} {
	compiledT, errT := Compile(scriptA)

	if errT != nil {
		return errT
	}

	errT = compiledT.DeepCompile()

	if errT != nil {
		return errT
	}

	if DebugG {
		tk.Pl("compiled: %v", tk.ToJSONX(compiledT, "-sort", "-indent"))
	}

	// rsT := compiledT.Run()

	vmT := NewVM(compiledT)

	rsT := vmT.RunOpCodes()

	if DebugG {
		tk.Pl("VM: %v", tk.ToJSONX(vmT, "-sort", "-indent"))
	}

	return rsT
}
