package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
)

var Magic = [4]byte{'L', 'U', 'M', 'E'}

const (
	MajorVersion = 0
	MinorVersion = 3
	FormatByte   = 0
)

type Opcode byte

const (
	OpPushConst   Opcode = 0x01
	OpPushNil     Opcode = 0x02
	OpPop         Opcode = 0x03
	OpDup         Opcode = 0x04

	OpLoadGlobal  Opcode = 0x10
	OpStoreGlobal Opcode = 0x11

	OpAdd Opcode = 0x20
	OpSub Opcode = 0x21
	OpMul Opcode = 0x22
	OpDiv Opcode = 0x23
	OpMod Opcode = 0x24
	OpNeg Opcode = 0x25

	OpEq  Opcode = 0x30
	OpNeq Opcode = 0x31
	OpGt  Opcode = 0x32
	OpLt  Opcode = 0x33
	OpGte Opcode = 0x34
	OpLte Opcode = 0x35
	OpNot Opcode = 0x36
	OpAnd Opcode = 0x37
	OpOr  Opcode = 0x38

	OpJump        Opcode = 0x40
	OpJumpIfFalse Opcode = 0x41
	OpJumpIfTrue  Opcode = 0x42

	OpCall        Opcode = 0x50
	OpRet         Opcode = 0x51
	OpCallBuiltin Opcode = 0x52

	OpPrint Opcode = 0x60
	OpInput Opcode = 0x61
	OpDelay Opcode = 0x63

	OpNewArray Opcode = 0x70
	OpIndexGet Opcode = 0x71
	OpIndexSet Opcode = 0x72
	OpArrayLen Opcode = 0x73

	OpHalt Opcode = 0x80
)

const (
	BuiltinRand   = 1
	BuiltinLen    = 2
	BuiltinPush   = 3
	BuiltinPop    = 4
	BuiltinInsert = 5
	BuiltinRemove = 6
	BuiltinSlice  = 7
)

var BuiltinNames = map[string]byte{
	"rand": BuiltinRand, "len": BuiltinLen, "push": BuiltinPush,
	"pop": BuiltinPop, "insert": BuiltinInsert, "remove": BuiltinRemove, "slice": BuiltinSlice,
}

type ConstTag byte

const (
	ConstNil    ConstTag = 0
	ConstInt    ConstTag = 1
	ConstFloat  ConstTag = 2
	ConstBool   ConstTag = 3
	ConstString ConstTag = 4
)

type Constant struct {
	Tag  ConstTag
	Int  int64
	Flt  float64
	Bool bool
	Str  string
}

type FuncInfo struct {
	NameIdx    uint32
	ParamCount uint8
	EntryPC    uint32
	ParamIdxs  []uint32
}

type ArrayLimit struct {
	GlobalIdx uint32
	Min       int64
	Max       int64
}

type InputBinding struct {
	InputID   int64
	GlobalIdx uint32
}

type HttpRouteInfo struct {
	Method  string
	Path    string
	BodyIdx uint32
	File    string
	Save    string
	Type    string
	Status  int64
	MaxBody int64
}

type HttpStaticInfo struct {
	Root  string
	Index string
}

type HttpErrorInfo struct {
	Status  int64
	BodyIdx uint32
	Type    string
}

type HttpServeInfo struct {
	Port    int64
	Log     bool
	MaxConn int64
}

type Program struct {
	Consts        []Constant
	GlobalNames   []string
	Funcs         []FuncInfo
	Code          []byte
	Limits        []ArrayLimit
	InputBindings []InputBinding
	Imports       []string

	HttpRoutes []HttpRouteInfo
	HttpStatic *HttpStaticInfo
	HttpErrors []HttpErrorInfo
	HttpServe  *HttpServeInfo
}

func (p *Program) Encode() ([]byte, error) {
	var body bytes.Buffer

	writeU32(&body, uint32(len(p.Consts)))
	for _, c := range p.Consts {
		body.WriteByte(byte(c.Tag))
		switch c.Tag {
		case ConstInt:
			writeI64(&body, c.Int)
		case ConstFloat:
			writeF64(&body, c.Flt)
		case ConstBool:
			if c.Bool {
				body.WriteByte(1)
			} else {
				body.WriteByte(0)
			}
		case ConstString:
			writeU32(&body, uint32(len(c.Str)))
			body.WriteString(c.Str)
		}
	}
	writeU32(&body, uint32(len(p.GlobalNames)))
	for _, n := range p.GlobalNames {
		writeU32(&body, uint32(len(n)))
		body.WriteString(n)
	}
	writeU32(&body, uint32(len(p.Funcs)))
	for _, f := range p.Funcs {
		writeU32(&body, f.NameIdx)
		body.WriteByte(f.ParamCount)
		writeU32(&body, f.EntryPC)
		writeU32(&body, uint32(len(f.ParamIdxs)))
		for _, pi := range f.ParamIdxs {
			writeU32(&body, pi)
		}
	}
	writeU32(&body, uint32(len(p.Limits)))
	for _, l := range p.Limits {
		writeU32(&body, l.GlobalIdx)
		writeI64(&body, l.Min)
		writeI64(&body, l.Max)
	}
	writeU32(&body, uint32(len(p.InputBindings)))
	for _, b := range p.InputBindings {
		writeI64(&body, b.InputID)
		writeU32(&body, b.GlobalIdx)
	}
	writeU32(&body, uint32(len(p.Imports)))
	for _, s := range p.Imports {
		writeU32(&body, uint32(len(s)))
		body.WriteString(s)
	}

	writeU32(&body, uint32(len(p.HttpRoutes)))
	for _, r := range p.HttpRoutes {
		writeStr(&body, r.Method)
		writeStr(&body, r.Path)
		writeU32(&body, r.BodyIdx)
		writeStr(&body, r.File)
		writeStr(&body, r.Save)
		writeStr(&body, r.Type)
		writeI64(&body, r.Status)
		writeI64(&body, r.MaxBody)
	}
	if p.HttpStatic != nil {
		body.WriteByte(1)
		writeStr(&body, p.HttpStatic.Root)
		writeStr(&body, p.HttpStatic.Index)
	} else {
		body.WriteByte(0)
	}
	writeU32(&body, uint32(len(p.HttpErrors)))
	for _, e := range p.HttpErrors {
		writeI64(&body, e.Status)
		writeU32(&body, e.BodyIdx)
		writeStr(&body, e.Type)
	}
	if p.HttpServe != nil {
		body.WriteByte(1)
		writeI64(&body, p.HttpServe.Port)
		if p.HttpServe.Log {
			body.WriteByte(1)
		} else {
			body.WriteByte(0)
		}
		writeI64(&body, p.HttpServe.MaxConn)
	} else {
		body.WriteByte(0)
	}

	writeU32(&body, uint32(len(p.Code)))
	body.Write(p.Code)

	var out bytes.Buffer
	out.Write(Magic[:])
	writeU16(&out, MajorVersion)
	writeU16(&out, MinorVersion)
	out.WriteByte(FormatByte)
	out.WriteByte(0)
	writeU16(&out, 0)
	crc := crc32.ChecksumIEEE(body.Bytes())
	writeU32(&out, crc)
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func Decode(data []byte) (*Program, error) {
	if len(data) < 16 {
		return nil, errors.New("文件太短")
	}
	if !bytes.Equal(data[0:4], Magic[:]) {
		return nil, errors.New("魔数不匹配")
	}
	major := binary.LittleEndian.Uint16(data[4:6])
	minor := binary.LittleEndian.Uint16(data[6:8])
	if major != MajorVersion || minor != MinorVersion {
		return nil, fmt.Errorf("版本不兼容：%d.%d", major, minor)
	}
	crc := binary.LittleEndian.Uint32(data[12:16])
	body := data[16:]
	if crc != 0 && crc32.ChecksumIEEE(body) != crc {
		return nil, errors.New("CRC 校验失败")
	}

	r := &reader{data: body}
	p := &Program{}

	cn := r.u32()
	p.Consts = make([]Constant, cn)
	for i := range p.Consts {
		tag := ConstTag(r.byte())
		c := Constant{Tag: tag}
		switch tag {
		case ConstNil:
		case ConstInt:
			c.Int = r.i64()
		case ConstFloat:
			c.Flt = r.f64()
		case ConstBool:
			c.Bool = r.byte() == 1
		case ConstString:
			c.Str = r.str(r.u32())
		}
		p.Consts[i] = c
	}

	gn := r.u32()
	p.GlobalNames = make([]string, gn)
	for i := range p.GlobalNames {
		p.GlobalNames[i] = r.str(r.u32())
	}

	fn := r.u32()
	p.Funcs = make([]FuncInfo, fn)
	for i := range p.Funcs {
		f := FuncInfo{NameIdx: r.u32(), ParamCount: r.byte(), EntryPC: r.u32()}
		pn := r.u32()
		f.ParamIdxs = make([]uint32, pn)
		for j := range f.ParamIdxs {
			f.ParamIdxs[j] = r.u32()
		}
		p.Funcs[i] = f
	}

	ln := r.u32()
	p.Limits = make([]ArrayLimit, ln)
	for i := range p.Limits {
		p.Limits[i] = ArrayLimit{GlobalIdx: r.u32(), Min: r.i64(), Max: r.i64()}
	}

	ibn := r.u32()
	p.InputBindings = make([]InputBinding, ibn)
	for i := range p.InputBindings {
		p.InputBindings[i] = InputBinding{InputID: r.i64(), GlobalIdx: r.u32()}
	}

	imn := r.u32()
	p.Imports = make([]string, imn)
	for i := range p.Imports {
		p.Imports[i] = r.str(r.u32())
	}

	rn := r.u32()
	p.HttpRoutes = make([]HttpRouteInfo, rn)
	for i := range p.HttpRoutes {
		p.HttpRoutes[i] = HttpRouteInfo{
			Method: r.str(r.u32()), Path: r.str(r.u32()), BodyIdx: r.u32(),
			File: r.str(r.u32()), Save: r.str(r.u32()), Type: r.str(r.u32()),
			Status: r.i64(), MaxBody: r.i64(),
		}
	}
	if r.byte() == 1 {
		p.HttpStatic = &HttpStaticInfo{Root: r.str(r.u32()), Index: r.str(r.u32())}
	}
	en := r.u32()
	p.HttpErrors = make([]HttpErrorInfo, en)
	for i := range p.HttpErrors {
		p.HttpErrors[i] = HttpErrorInfo{Status: r.i64(), BodyIdx: r.u32(), Type: r.str(r.u32())}
	}
	if r.byte() == 1 {
		p.HttpServe = &HttpServeInfo{Port: r.i64(), Log: r.byte() == 1, MaxConn: r.i64()}
	}

	codeLen := r.u32()
	p.Code = r.bytes(codeLen)

	if r.err != nil {
		return nil, r.err
	}
	return p, nil
}

func writeU16(w *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.Write(b[:])
}
func writeU32(w *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}
func writeI64(w *bytes.Buffer, v int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	w.Write(b[:])
}
func writeF64(w *bytes.Buffer, v float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	w.Write(b[:])
}
func writeStr(w *bytes.Buffer, s string) {
	writeU32(w, uint32(len(s)))
	w.WriteString(s)
}

type reader struct {
	data []byte
	pos  int
	err  error
}

func (r *reader) byte() byte {
	if r.err != nil || r.pos >= len(r.data) {
		r.err = errors.New("读取越界")
		return 0
	}
	v := r.data[r.pos]
	r.pos++
	return v
}
func (r *reader) u32() uint32 {
	if r.err != nil || r.pos+4 > len(r.data) {
		r.err = errors.New("读取越界")
		return 0
	}
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v
}
func (r *reader) i64() int64 {
	if r.err != nil || r.pos+8 > len(r.data) {
		r.err = errors.New("读取越界")
		return 0
	}
	v := int64(binary.LittleEndian.Uint64(r.data[r.pos:]))
	r.pos += 8
	return v
}
func (r *reader) f64() float64 {
	if r.err != nil || r.pos+8 > len(r.data) {
		r.err = errors.New("读取越界")
		return 0
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(r.data[r.pos:]))
	r.pos += 8
	return v
}
func (r *reader) str(n uint32) string {
	if r.err != nil || r.pos+int(n) > len(r.data) {
		r.err = errors.New("读取越界")
		return ""
	}
	s := string(r.data[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s
}
func (r *reader) bytes(n uint32) []byte {
	if r.err != nil || r.pos+int(n) > len(r.data) {
		r.err = errors.New("读取越界")
		return nil
	}
	b := make([]byte, n)
	copy(b, r.data[r.pos:r.pos+int(n)])
	r.pos += int(n)
	return b
}