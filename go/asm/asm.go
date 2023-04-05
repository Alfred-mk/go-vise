package asm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
//	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"git.defalsify.org/festive/vm"
)


type Asm struct {
	Instructions []*Instruction `@@*`
}

type Display struct {
	Sym string `@Sym Whitespace`
	//Val string `Quote (@Desc @Whitespace?)+ Quote Whitespace`
	Val string `Quote (@Sym @Whitespace?)+ Quote Whitespace? EOL`
}

func(d Display) String() string {
	return fmt.Sprintf("Display: %v %v", d.Sym, d.Val)
}

type Sig struct {
	Sym string `@Sym Whitespace`
	Size uint32 `@Size Whitespace`
	Val uint8 `@Size Whitespace? EOL`
}
//
//func(s Sig) Capture(v []string) error {
//	log.Printf("considering capture %v %v", v[0], len(v))
//	if len(v) < 3 {
//		return nil
//	}
//	s.Sym = v[0]
//	r, err := strconv.Atoi(v[1])
//	if err != nil {
//		return err
//	}
//	s.Size = uint32(r)
//	r, err = strconv.Atoi(v[2])
//	if err != nil {
//		return err
//	}
//	if r != 0 {
//		r = 1
//	}
//	s.Val = uint8(r)
//	log.Printf("after considering capture: %v", s)
//	return nil
//}

func(s Sig) String() string {
	return fmt.Sprintf("Sig: %v %v %v", s.Sym, s.Size, s.Val)
}

type Single struct {
	One string `@Sym Whitespace? EOL`
}

func(s Single) String() string {
	return fmt.Sprintf("Single: %v", s.One)
}

type Double struct {
	One string `@Sym Whitespace`
	Two string `@Sym Whitespace? EOL`
}

func(d Double) String() string {
	return fmt.Sprintf("Double: %v %v", d.One, d.Two)
}

type Sized struct {
	Sym string `@Sym Whitespace`
	Size uint32 `@Size Whitespace? EOL`
}

func(s Sized) Capture(v []Sized) error {
	log.Printf("foofofofofo")
	return fmt.Errorf("foo foo foo")
}

func(s Sized) String() string {
	return fmt.Sprintf("Sized: %v %v", s.Sym, s.Size)
}

type Arg struct {
	ArgNone string "Discard?"
	ArgDisplay *Display `@@?`
	ArgSized *Sized `@@?`
	ArgSig *Sig `@@?`
	ArgDouble *Double `@@?`
	ArgSingle *Single `@@?`
}

func (a Arg) String() string {
	if a.ArgDisplay != nil {
		return fmt.Sprintf("%s", a.ArgDisplay)
	}
	if a.ArgSig != nil {
		return fmt.Sprintf("%s", a.ArgSig)
	}
	if a.ArgSized != nil {
		return fmt.Sprintf("%s", a.ArgSized)
	}
	if a.ArgSingle != nil {
		return fmt.Sprintf("%s", a.ArgSingle)
	}
	if a.ArgDouble != nil {
		return fmt.Sprintf("%s", a.ArgDouble)
	}
	return ""
}

type Instruction struct {
	OpCode string `@Ident`
	OpArg Arg `@@`
	Comment string `Comment?`
}

func (i Instruction) String() string {
	return fmt.Sprintf("%s %s", i.OpCode, i.OpArg)
}

var (
	asmLexer = lexer.MustSimple([]lexer.SimpleRule{
		{"Comment", `(?:#)[^\n]*\n?`},
		{"Ident", `^[A-Z]+`},
		{"SizeSig", `[0-9]+\s+{?:[0-9]}`},
		{"Size", `[0-9]+`},
		{"Sym", `[a-zA-Z_][a-zA-Z0-9_]+`},
		{"Whitespace", `[ \t]+`},
		{"Discard", `^\s+[\n\r]+$`},
		{"EOL", `[\n\r]+`},
		{"Quote", `["']`},
	})
	asmParser = participle.MustBuild[Asm](
		participle.Lexer(asmLexer),
		participle.Elide("Comment", "Whitespace"),
	)
)

func numSize(n uint32) int {
	v := math.Log2(float64(n))
	return int(((v - 1) / 8) + 1)
}

func writeOpcode(op vm.Opcode, w *bytes.Buffer) (int, error) {
	bn := [2]byte{}
	binary.BigEndian.PutUint16(bn[:], uint16(op))
	n, err := w.Write(bn[:])
	return n, err
}

func writeSym(s string, w *bytes.Buffer) (int, error) {
	sz := len(s)
	if sz > 255 {
		return 0, fmt.Errorf("string size %v too big", sz)
	}
	w.Write([]byte{byte(sz)})
	return w.WriteString(s)
}

func writeDisplay(s string, w *bytes.Buffer) (int, error) {
	s = strings.Trim(s, "\"'")
	sz := len(s)
	if sz > 255 {
		return 0, fmt.Errorf("string size %v too big", sz)
	}
	w.Write([]byte{byte(sz)})
	return w.WriteString(s)
}

func writeSize(n uint32, w *bytes.Buffer) (int, error) {
	bn := [4]byte{}
	sz := numSize(n)
	if sz > 4 {
		return 0, fmt.Errorf("number size %v too big", sz)
	}
	w.Write([]byte{byte(sz)})
	binary.BigEndian.PutUint32(bn[:], n)
	c := 4-sz
	return w.Write(bn[c:])
}

func parseSingle(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	v := arg.ArgSingle
	if v == nil {
		return 0, nil
	}

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	
	n, err = writeSym(v.One, b)
	rn += n
	if err != nil {
		return rn, err
	}
	
	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func parseDisplay(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	v := arg.ArgDisplay
	if v == nil {
		return 0, nil
	}

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	
	n, err = writeSym(v.Sym, b)
	rn += n
	if err != nil {
		return rn, err
	}

	n, err = writeDisplay(v.Val, b)
	rn += n
	if err != nil {
		return rn, err
	}
	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func parseDouble(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	v := arg.ArgDouble
	if v == nil {
		return 0, nil
	}

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	
	n, err = writeSym(v.One, b)
	rn += n
	if err != nil {
		return rn, err
	}

	n, err = writeSym(v.Two, b)
	rn += n
	if err != nil {
		return rn, err
	}

	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func parseSized(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	v := arg.ArgSized
	if v == nil {
		return 0, nil
	}

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	
	n, err = writeSym(v.Sym, b)
	rn += n
	if err != nil {
		return rn, err
	}

	n, err = writeSize(v.Size, b)
	rn += n
	if err != nil {
		return rn, err
	}
	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func parseSig(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	v := arg.ArgSig
	if v == nil {
		return 0, nil
	}

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	
	n, err = writeSym(v.Sym, b)
	rn += n
	if err != nil {
		return rn, err
	}

	n, err = writeSize(v.Size, b)
	rn += n
	if err != nil {
		return rn, err
	}

	if v.Val == 0 {
		n, err = b.Write([]byte{0x00})
	} else {
		n, err = b.Write([]byte{0x01})
	}
	rn += n
	if err != nil {
		return rn, err
	}


	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func parseNoarg(op vm.Opcode, arg Arg, w io.Writer) (int, error) {
	var rn int

	b := bytes.NewBuffer(nil)

	n, err := writeOpcode(op, b)
	rn += n
	if  err != nil {
		return rn, err
	}
	if w != nil {
		rn, err = w.Write(b.Bytes())
	} else {
		rn = 0
	}
	return rn, err
}

func Parse(s string, w io.Writer) (int, error) {
	rd := strings.NewReader(s)
	ast, err := asmParser.Parse("file", rd)
	if err != nil {
		return 0, err
	}

	var rn int
	for _, v := range ast.Instructions {
		log.Printf("parsing line %v: %v", v.OpCode, v.OpArg)
		op := vm.OpcodeIndex[v.OpCode]
		n, err := parseSized(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}
		n, err = parseDisplay(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}
		n, err = parseSig(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}
		n, err = parseDouble(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}
		n, err = parseSingle(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}
		n, err = parseNoarg(op, v.OpArg, w)
		if err != nil {
			return n, err
		}
		if n > 0 {
			rn += n
			continue
		}

	}
	return rn, err
}
