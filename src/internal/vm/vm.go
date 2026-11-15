package vm

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"top.longmoon/lume/internal/bytecode"
)

type Value = interface{}

type Array struct {
	Elems []Value
}

type breakSignal struct{}
type continueSignal struct{}
type returnSignal struct{}

type VM struct {
	prog    *bytecode.Program
	code    []byte
	pc      int
	stack   []Value
	globals []Value
	funcs   map[string]uint32

	fileImported bool
	httpImported bool

	routes     []routeInfo
	static     *staticInfo
	errorPages map[int64]errorPageInfo

	reader *bufio.Reader
}

type routeInfo struct {
	Method  string
	Path    string
	Body    uint32
	File    string
	Save    string
	Type    string
	Status  int64
	MaxBody int64
}

type staticInfo struct {
	Root  string
	Index string
}

type errorPageInfo struct {
	BodyIdx uint32
	Type    string
}

func New(prog *bytecode.Program) *VM {
	vm := &VM{
		prog:       prog,
		code:       prog.Code,
		globals:    make([]Value, len(prog.GlobalNames)),
		funcs:      map[string]uint32{},
		errorPages: map[int64]errorPageInfo{},
		reader:     bufio.NewReader(os.Stdin),
	}
	for i := range vm.globals {
		vm.globals[i] = nil
	}
	for i, fn := range prog.Funcs {
		name := prog.Consts[fn.NameIdx].Str
		vm.funcs[name] = uint32(i)
	}
	for _, r := range prog.HttpRoutes {
		vm.routes = append(vm.routes, routeInfo{
			Method: r.Method, Path: r.Path, Body: r.BodyIdx,
			File: r.File, Save: r.Save, Type: r.Type,
			Status: r.Status, MaxBody: r.MaxBody,
		})
	}
	if prog.HttpStatic != nil {
		vm.static = &staticInfo{Root: prog.HttpStatic.Root, Index: prog.HttpStatic.Index}
	}
	for _, e := range prog.HttpErrors {
		vm.errorPages[e.Status] = errorPageInfo{BodyIdx: e.BodyIdx, Type: e.Type}
	}
	for _, m := range prog.Imports {
		switch m {
		case "file":
			vm.fileImported = true
		case "http":
			vm.httpImported = true
		}
	}
	return vm
}

func (vm *VM) Run() error {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case returnSignal, breakSignal, continueSignal:
			default:
				panic(r)
			}
		}
	}()
	for vm.pc < len(vm.code) {
		if err := vm.step(); err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) step() error {
	op := bytecode.Opcode(vm.code[vm.pc])
	vm.pc++
	switch op {
	case bytecode.OpHalt:
		vm.pc = len(vm.code)
	case bytecode.OpPushConst:
		idx := vm.readU32()
		vm.push(vm.constToValue(vm.prog.Consts[idx]))
	case bytecode.OpPushNil:
		vm.push(nil)
	case bytecode.OpPop:
		vm.pop()
	case bytecode.OpDup:
		v := vm.pop()
		vm.push(v)
		vm.push(v)
	case bytecode.OpLoadGlobal:
		idx := vm.readU32()
		vm.push(vm.globals[idx])
	case bytecode.OpStoreGlobal:
		idx := vm.readU32()
		v := vm.pop()
		if arr, ok := v.(*Array); ok {
			vm.applyLimit(idx, arr)
		}
		vm.globals[idx] = v
	case bytecode.OpAdd:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.add(a, b))
	case bytecode.OpSub:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.sub(a, b))
	case bytecode.OpMul:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.mul(a, b))
	case bytecode.OpDiv:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.div(a, b))
	case bytecode.OpMod:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.mod(a, b))
	case bytecode.OpNeg:
		vm.push(vm.neg(vm.pop()))
	case bytecode.OpEq:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.equal(a, b))
	case bytecode.OpNeq:
		b := vm.pop()
		a := vm.pop()
		vm.push(!vm.equal(a, b))
	case bytecode.OpGt:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.compare(a, b) > 0)
	case bytecode.OpLt:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.compare(a, b) < 0)
	case bytecode.OpGte:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.compare(a, b) >= 0)
	case bytecode.OpLte:
		b := vm.pop()
		a := vm.pop()
		vm.push(vm.compare(a, b) <= 0)
	case bytecode.OpNot:
		vm.push(!vm.truthy(vm.pop()))
	case bytecode.OpJump:
		vm.pc = int(vm.readU32())
	case bytecode.OpJumpIfFalse:
		target := vm.readU32()
		if !vm.truthy(vm.pop()) {
			vm.pc = int(target)
		}
	case bytecode.OpJumpIfTrue:
		target := vm.readU32()
		if vm.truthy(vm.pop()) {
			vm.pc = int(target)
		}
	case bytecode.OpCall:
		fidx := vm.readU32()
		argc := int(vm.readByte())
		vm.callUser(fidx, argc)
	case bytecode.OpRet:
		panic(returnSignal{})
	case bytecode.OpCallBuiltin:
		bid := vm.readByte()
		argc := int(vm.readByte())
		vm.callBuiltin(bid, argc)
	case bytecode.OpPrint:
		fmt.Println(vm.format(vm.pop()))
	case bytecode.OpInput:
		vm.opInput()
	case bytecode.OpDelay:
		d := vm.toInt(vm.pop())
		if d > 0 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
	case bytecode.OpNewArray:
		n := int(vm.readU32())
		arr := &Array{Elems: make([]Value, n)}
		for i := n - 1; i >= 0; i-- {
			arr.Elems[i] = vm.pop()
		}
		vm.push(arr)
	case bytecode.OpIndexGet:
		idx := vm.pop()
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			return fmt.Errorf("索引访问的目标不是数组")
		}
		i := vm.toInt(idx)
		if i < 0 || int(i) >= len(arr.Elems) {
			return fmt.Errorf("数组索引越界: %d（长度 %d）", i, len(arr.Elems))
		}
		vm.push(arr.Elems[i])
	case bytecode.OpIndexSet:
		val := vm.pop()
		idx := vm.pop()
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			return fmt.Errorf("索引赋值的目标不是数组")
		}
		i := vm.toInt(idx)
		if i < 0 || int(i) >= len(arr.Elems) {
			return fmt.Errorf("数组索引越界: %d（长度 %d）", i, len(arr.Elems))
		}
		arr.Elems[i] = val
	case bytecode.OpArrayLen:
		arrV := vm.pop()
		switch x := arrV.(type) {
		case *Array:
			vm.push(int64(len(x.Elems)))
		case string:
			vm.push(int64(len([]rune(x))))
		default:
			return fmt.Errorf("len 不支持的类型")
		}
	default:
		return fmt.Errorf("未知指令 0x%02x", byte(op))
	}
	return nil
}

func (vm *VM) callBuiltin(id byte, argc int) {
	switch id {
	case bytecode.BuiltinRand:
		hi := vm.toInt(vm.pop())
		lo := vm.toInt(vm.pop())
		if hi < lo {
			lo, hi = hi, lo
		}
		vm.push(lo + rand.Int63n(hi-lo+1))
	case bytecode.BuiltinLen:
		v := vm.pop()
		switch x := v.(type) {
		case string:
			vm.push(int64(len([]rune(x))))
		case *Array:
			vm.push(int64(len(x.Elems)))
		default:
			panic("len 只支持字符串和数组")
		}
	case bytecode.BuiltinPush:
		x := vm.pop()
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			panic("push 第一个参数必须是数组")
		}
		arr.Elems = append(arr.Elems, x)
		vm.push(arr)
	case bytecode.BuiltinPop:
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			panic("pop 参数必须是数组")
		}
		if len(arr.Elems) == 0 {
			panic("pop 空数组")
		}
		v := arr.Elems[len(arr.Elems)-1]
		arr.Elems = arr.Elems[:len(arr.Elems)-1]
		vm.push(v)
	case bytecode.BuiltinInsert:
		x := vm.pop()
		iV := vm.pop()
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			panic("insert 第一个参数必须是数组")
		}
		i := vm.toInt(iV)
		if i < 0 || int(i) > len(arr.Elems) {
			panic("insert 索引越界")
		}
		arr.Elems = append(arr.Elems, nil)
		copy(arr.Elems[i+1:], arr.Elems[i:])
		arr.Elems[i] = x
		vm.push(arr)
	case bytecode.BuiltinRemove:
		iV := vm.pop()
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			panic("remove 第一个参数必须是数组")
		}
		i := vm.toInt(iV)
		if i < 0 || int(i) >= len(arr.Elems) {
			panic("remove 索引越界")
		}
		v := arr.Elems[i]
		arr.Elems = append(arr.Elems[:i], arr.Elems[i+1:]...)
		vm.push(v)
	case bytecode.BuiltinSlice:
		end := vm.toInt(vm.pop())
		start := vm.toInt(vm.pop())
		arrV := vm.pop()
		arr, ok := arrV.(*Array)
		if !ok {
			panic("slice 第一个参数必须是数组")
		}
		s, e := start, end
		if s < 0 {
			s = 0
		}
		if e > int64(len(arr.Elems)) {
			e = int64(len(arr.Elems))
		}
		if s > e {
			s = e
		}
		vm.push(&Array{Elems: append([]Value{}, arr.Elems[s:e]...)})
	case 100:
		content := vm.format(vm.pop())
		path := vm.format(vm.pop())
		vm.push(vm.fileWrite(path, content))
	case 101:
		content := vm.format(vm.pop())
		path := vm.format(vm.pop())
		vm.push(vm.fileAppend(path, content))
	case 102:
		path := vm.format(vm.pop())
		vm.push(vm.fileRead(path))
	case 103:
		path := vm.format(vm.pop())
		vm.push(vm.fileExists(path))
	case 104:
		newp := vm.format(vm.pop())
		oldp := vm.format(vm.pop())
		vm.push(vm.fileRename(oldp, newp))
	case 105:
		path := vm.format(vm.pop())
		vm.push(vm.fileDelete(path))
	case 106:
		path := vm.format(vm.pop())
		vm.push(vm.fileMkdir(path))
	case 200:
		if vm.prog.HttpServe != nil {
			vm.serveHTTP(vm.prog.HttpServe.Port, vm.prog.HttpServe.Log, vm.prog.HttpServe.MaxConn)
		}
	default:
		panic(fmt.Sprintf("未知内置函数 id %d", id))
	}
}

func (vm *VM) callUser(fidx uint32, argc int) {
	fn := vm.prog.Funcs[fidx]
	name := vm.prog.Consts[fn.NameIdx].Str
	if argc != int(fn.ParamCount) {
		panic(fmt.Sprintf("函数 %s 需要 %d 个参数，给了 %d", name, fn.ParamCount, argc))
	}
	// 弹参数（最后一个参数在栈顶）
	args := make([]Value, argc)
	for i := argc - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	// 写参数到全局
	for i, gi := range fn.ParamIdxs {
		vm.globals[gi] = args[i]
	}

	savedPC := vm.pc
	var result Value = nil
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(returnSignal); ok {
					if len(vm.stack) > 0 {
						result = vm.stack[len(vm.stack)-1]
						vm.stack = vm.stack[:len(vm.stack)-1]
					}
					return
				}
				panic(r)
			}
		}()
		vm.pc = int(fn.EntryPC)
		for vm.pc < len(vm.code) {
			if err := vm.step(); err != nil {
				panic(err)
			}
		}
	}()
	vm.pc = savedPC
	vm.push(result)
}

func (vm *VM) opInput() {
	inputId := vm.readU32()
	typeTag := vm.readByte()
	hasMin := vm.readByte() == 1
	minF := vm.readF64()
	hasMax := vm.readByte() == 1
	maxF := vm.readF64()
	promptIdx := vm.readU32()
	overIdx := vm.readU32()
	underIdx := vm.readU32()
	invalidIdx := vm.readU32()

	prompt := "请输入"
	if promptIdx != 0xFFFFFFFF {
		prompt = vm.prog.Consts[promptIdx].Str
	}

	for {
		fmt.Printf("%s: ", prompt)
		line, err := vm.reader.ReadString('\n')
		if err != nil && line == "" {
			vm.push(nil)
			return
		}
		line = strings.TrimSpace(line)
		var value Value
		switch typeTag {
		case 0:
			v, err := strconv.ParseInt(line, 10, 64)
			if err != nil {
				fmt.Println(vm.inputErr(invalidIdx, "请输入一个整数"))
				continue
			}
			if hasMin && float64(v) < minF {
				fmt.Println(vm.inputErr(underIdx, fmt.Sprintf("不能小于 %v", trimFloat(minF))))
				continue
			}
			if hasMax && float64(v) > maxF {
				fmt.Println(vm.inputErr(overIdx, fmt.Sprintf("不能大于 %v", trimFloat(maxF))))
				continue
			}
			value = v
		case 1:
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				fmt.Println(vm.inputErr(invalidIdx, "请输入一个小数"))
				continue
			}
			if hasMin && v < minF {
				fmt.Println(vm.inputErr(underIdx, fmt.Sprintf("不能小于 %v", trimFloat(minF))))
				continue
			}
			if hasMax && v > maxF {
				fmt.Println(vm.inputErr(overIdx, fmt.Sprintf("不能大于 %v", trimFloat(maxF))))
				continue
			}
			value = v
		default:
			value = line
		}
		for _, b := range vm.prog.InputBindings {
			if b.InputID == int64(inputId) {
				vm.globals[b.GlobalIdx] = value
			}
		}
		vm.push(value)
		return
	}
}

func (vm *VM) inputErr(idx uint32, def string) string {
	if idx == 0xFFFFFFFF {
		return def
	}
	return vm.prog.Consts[idx].Str
}

func (vm *VM) applyLimit(globalIdx uint32, arr *Array) {
	for _, l := range vm.prog.Limits {
		if l.GlobalIdx == globalIdx {
			if int64(len(arr.Elems)) < l.Min {
				panic(fmt.Sprintf("数组长度 %d 小于下限 %d", len(arr.Elems), l.Min))
			}
			if int64(len(arr.Elems)) > l.Max {
				panic(fmt.Sprintf("数组长度 %d 超过上限 %d", len(arr.Elems), l.Max))
			}
		}
	}
}

func (vm *VM) push(v Value) { vm.stack = append(vm.stack, v) }

func (vm *VM) pop() Value {
	if len(vm.stack) == 0 {
		panic("栈下溢")
	}
	v := vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return v
}

func (vm *VM) readByte() byte {
	if vm.pc >= len(vm.code) {
		panic("指令读取越界")
	}
	b := vm.code[vm.pc]
	vm.pc++
	return b
}

func (vm *VM) readU32() uint32 {
	if vm.pc+4 > len(vm.code) {
		panic("指令读取越界")
	}
	v := binary.LittleEndian.Uint32(vm.code[vm.pc:])
	vm.pc += 4
	return v
}

func (vm *VM) readF64() float64 {
	if vm.pc+8 > len(vm.code) {
		panic("指令读取越界")
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(vm.code[vm.pc:]))
	vm.pc += 8
	return v
}

func (vm *VM) truthy(v Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case *Array:
		return len(x.Elems) > 0
	}
	return true
}

func (vm *VM) constToValue(c bytecode.Constant) Value {
	switch c.Tag {
	case bytecode.ConstInt:
		return c.Int
	case bytecode.ConstFloat:
		return c.Flt
	case bytecode.ConstBool:
		return c.Bool
	case bytecode.ConstString:
		return c.Str
	}
	return nil
}

func (vm *VM) format(v Value) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if math.Abs(x-math.Round(x)) < 1e-9 {
			return strconv.FormatInt(int64(math.Round(x)), 10)
		}
		return strconv.FormatFloat(x, 'g', 15, 64)
	case string:
		return x
	case *Array:
		parts := make([]string, len(x.Elems))
		for i, e := range x.Elems {
			parts[i] = vm.format(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return fmt.Sprintf("%v", v)
}

func (vm *VM) toInt(v Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	}
	panic("期望数值")
}

func (vm *VM) toFloat(v Value) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	}
	panic("期望数值")
}

func (vm *VM) add(a, b Value) Value {
	if as, ok := a.(string); ok {
		return as + vm.format(b)
	}
	if bs, ok := b.(string); ok {
		return vm.format(a) + bs
	}
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok {
			return ai + bi
		}
	}
	return vm.toFloat(a) + vm.toFloat(b)
}

func (vm *VM) sub(a, b Value) Value {
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok {
			return ai - bi
		}
	}
	return vm.toFloat(a) - vm.toFloat(b)
}

func (vm *VM) mul(a, b Value) Value {
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok {
			return ai * bi
		}
	}
	return vm.toFloat(a) * vm.toFloat(b)
}

func (vm *VM) div(a, b Value) Value {
	ai, aok := a.(int64)
	bi, bok := b.(int64)
	if aok && bok {
		if bi == 0 {
			panic("除以零")
		}
		return ai / bi
	}
	return vm.toFloat(a) / vm.toFloat(b)
}

func (vm *VM) mod(a, b Value) Value {
	ai := vm.toInt(a)
	bi := vm.toInt(b)
	if bi == 0 {
		panic("取模零")
	}
	return ai % bi
}

func (vm *VM) neg(a Value) Value {
	switch x := a.(type) {
	case int64:
		return -x
	case float64:
		return -x
	}
	panic("一元负号只适用于数值")
}

func (vm *VM) equal(a, b Value) bool {
	if isNum(a) && isNum(b) {
		return vm.toFloat(a) == vm.toFloat(b)
	}
	return a == b
}

func (vm *VM) compare(a, b Value) int {
	if isNum(a) && isNum(b) {
		af, bf := vm.toFloat(a), vm.toFloat(b)
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs)
	}
	panic("无法比较的类型")
}

func isNum(v Value) bool {
	switch v.(type) {
	case int64, float64:
		return true
	}
	return false
}

func trimFloat(f float64) string {
	if math.Abs(f-math.Round(f)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(f)), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// ---------- file ----------

func (vm *VM) fileWrite(path, content string) int64 {
	if !vm.fileImported {
		panic("未导入 file 模块")
	}
	if err := os.WriteFile(vm.resolvePath(path), []byte(content), 0644); err != nil {
		panic(err)
	}
	return int64(len(content))
}

func (vm *VM) fileAppend(path, content string) int64 {
	if !vm.fileImported {
		panic("未导入 file 模块")
	}
	f, err := os.OpenFile(vm.resolvePath(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	n, _ := f.WriteString(content)
	return int64(n)
}

func (vm *VM) fileRead(path string) string {
	if !vm.fileImported {
		panic("未导入 file 模块")
	}
	data, err := os.ReadFile(vm.resolvePath(path))
	if err != nil {
		panic(err)
	}
	return string(data)
}

func (vm *VM) fileExists(path string) bool {
	_, err := os.Stat(vm.resolvePath(path))
	return err == nil
}

func (vm *VM) fileRename(oldp, newp string) bool {
	if err := os.Rename(vm.resolvePath(oldp), vm.resolvePath(newp)); err != nil {
		panic(err)
	}
	return true
}

func (vm *VM) fileDelete(path string) bool {
	if err := os.Remove(vm.resolvePath(path)); err != nil {
		panic(err)
	}
	return true
}

func (vm *VM) fileMkdir(path string) bool {
	if err := os.MkdirAll(vm.resolvePath(path), 0755); err != nil {
		panic(err)
	}
	return true
}

func (vm *VM) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, p)
}

// ---------- HTTP ----------

func (vm *VM) serveHTTP(port int64, log bool, maxConn int64) {
	if !vm.httpImported {
		panic("未导入 http 模块")
	}

	var staticRoot string
	if vm.static != nil {
		staticRoot = vm.resolvePath(vm.static.Root)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		for _, rt := range vm.routes {
			if strings.EqualFold(r.Method, rt.Method) && r.URL.Path == rt.Path {
				if rt.MaxBody > 0 {
					r.Body = http.MaxBytesReader(w, r.Body, rt.MaxBody)
				}
				if rt.Save != "" {
					buf := make([]byte, 32*1024)
					f, err := os.Create(vm.resolvePath(rt.Save))
					if err == nil {
						for {
							n, err := r.Body.Read(buf)
							if n > 0 {
								f.Write(buf[:n])
							}
							if err != nil {
								break
							}
						}
						f.Close()
					}
				}
				var body string
				if rt.File != "" {
					data, err := os.ReadFile(vm.resolvePath(rt.File))
					if err != nil {
						http.Error(w, "500 Internal Server Error", 500)
						return
					}
					body = string(data)
				} else {
					body = vm.prog.Consts[rt.Body].Str
				}
				w.Header().Set("Content-Type", rt.Type)
				w.WriteHeader(int(rt.Status))
				fmt.Fprint(w, body)
				if log {
					logRequest(r, int(rt.Status), time.Since(start))
				}
				return
			}
		}

		if vm.static != nil {
			serveStatic(w, r, staticRoot, vm.static.Index, vm.errorPages, vm, log, start)
			return
		}

		writeError(w, r, 404, vm.errorPages, vm, log, start)
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Lume HTTP 服务已启动: http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, http.HandlerFunc(handler)); err != nil {
		panic(err)
	}
}

func serveStatic(w http.ResponseWriter, r *http.Request, root, index string, errorPages map[int64]errorPageInfo, vm *VM, log bool, start time.Time) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, r, 405, errorPages, vm, log, start)
		return
	}
	urlPath := r.URL.Path
	if urlPath == "/" || strings.HasSuffix(urlPath, "/") {
		urlPath += index
	}
	full := filepath.Join(root, filepath.Clean("/"+urlPath))
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		writeError(w, r, 403, errorPages, vm, log, start)
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		writeError(w, r, 404, errorPages, vm, log, start)
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(full))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(200)
	w.Write(data)
	if log {
		logRequest(r, 200, time.Since(start))
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, errorPages map[int64]errorPageInfo, vm *VM, log bool, start time.Time) {
	if ep, ok := errorPages[int64(status)]; ok {
		w.Header().Set("Content-Type", ep.Type)
		w.WriteHeader(status)
		fmt.Fprint(w, vm.prog.Consts[ep.BodyIdx].Str)
	} else {
		http.Error(w, http.StatusText(status), status)
	}
	if log {
		logRequest(r, status, time.Since(start))
	}
}

func logRequest(r *http.Request, status int, d time.Duration) {
	fmt.Printf("[%s] %s %s %s %d %v\n",
		time.Now().Format("2006-01-02 15:04:05"),
		clientIP(r), r.Method, r.URL.Path, status, d.Round(time.Millisecond))
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}