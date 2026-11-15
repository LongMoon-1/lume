package compiler

import (
	"encoding/binary"
	"fmt"
	"math"

	"top.longmoon/lume/internal/ast"
	"top.longmoon/lume/internal/bytecode"
)

type loopCtx struct {
	startPC       int
	breakJumps    []int
	continueJumps []int
}

type Compiler struct {
	prog       *bytecode.Program
	constIndex map[string]uint32
	globalIdx  map[string]uint32
	funcIdx    map[string]uint32
	code       []byte
	loops      []loopCtx
	imports    []string
}

func New() *Compiler {
	return &Compiler{
		prog:       &bytecode.Program{},
		constIndex: map[string]uint32{},
		globalIdx:  map[string]uint32{},
		funcIdx:    map[string]uint32{},
	}
}

func Compile(program *ast.Program) (*bytecode.Program, error) {
	c := New()
	if err := c.compileProgram(program); err != nil {
		return nil, err
	}
	c.prog.Code = c.code
	c.prog.Imports = c.imports
	return c.prog, nil
}

func (c *Compiler) compileProgram(program *ast.Program) error {
	// 1. 预注册函数名（EntryPC 先留 0，后面 patch）
	for _, stmt := range program.Stmts {
		if fn, ok := stmt.(*ast.FuncDecl); ok {
			c.registerFunc(fn)
		}
	}
	// 2. 预扫描顶层 var 块，登记全局名
	for _, stmt := range program.Stmts {
		if vb, ok := stmt.(*ast.VarBlock); ok {
			for _, d := range vb.Decls {
				c.globalIdxOf(d.Name)
			}
		}
	}
	// 3. 先编译顶层（PC=0 开始）
	for _, stmt := range program.Stmts {
		if _, ok := stmt.(*ast.FuncDecl); ok {
			continue
		}
		if err := c.compileStmt(stmt); err != nil {
			return err
		}
	}
	// 4. 再编译函数体（放在后面）
	for _, stmt := range program.Stmts {
		if fn, ok := stmt.(*ast.FuncDecl); ok {
			c.patchFuncEntry(fn)
			if err := c.compileFuncBody(fn); err != nil {
				return err
			}
		}
	}
	c.emit(bytecode.OpHalt)
	return nil
}

func (c *Compiler) registerFunc(fn *ast.FuncDecl) {
	if _, exists := c.funcIdx[fn.Name]; exists {
		panic(fmt.Sprintf("函数 %s 重复定义", fn.Name))
	}
	nameIdx := c.addConst(bytecode.Constant{Tag: bytecode.ConstString, Str: fn.Name})
	paramIdxs := make([]uint32, len(fn.Params))
	for i, p := range fn.Params {
		paramIdxs[i] = c.globalIdxOf(p)
	}
	idx := uint32(len(c.prog.Funcs))
	c.prog.Funcs = append(c.prog.Funcs, bytecode.FuncInfo{
		NameIdx:    nameIdx,
		ParamCount: uint8(len(fn.Params)),
		ParamIdxs:  paramIdxs,
	})
	c.funcIdx[fn.Name] = idx
}

func (c *Compiler) patchFuncEntry(fn *ast.FuncDecl) {
	idx := c.funcIdx[fn.Name]
	c.prog.Funcs[idx].EntryPC = uint32(len(c.code))
}

func (c *Compiler) compileFuncBody(fn *ast.FuncDecl) error {
	for _, p := range fn.Params {
		c.globalIdxOf(p)
	}
	for _, s := range fn.Body {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	c.emit(bytecode.OpPushNil)
	c.emit(bytecode.OpRet)
	return nil
}

func (c *Compiler) compileStmt(node ast.Node) error {
	switch n := node.(type) {
	case *ast.VarBlock:
		for _, mod := range n.Imports {
			c.imports = append(c.imports, mod)
		}
		for _, d := range n.Decls {
			if d.IsInput {
				idx := c.globalIdxOf(d.Name)
				c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstInt, Int: 0}))
				c.emitU32(bytecode.OpStoreGlobal, idx)
				c.prog.InputBindings = append(c.prog.InputBindings, bytecode.InputBinding{
					InputID: d.InputID, GlobalIdx: idx,
				})
			} else {
				idx := c.globalIdxOf(d.Name)
				if err := c.compileExpr(d.Expr); err != nil {
					return err
				}
				c.emitU32(bytecode.OpStoreGlobal, idx)
				if d.HasLimit {
					c.prog.Limits = append(c.prog.Limits, bytecode.ArrayLimit{
						GlobalIdx: idx, Min: d.LimitMin, Max: d.LimitMax,
					})
				}
			}
		}
	case *ast.MsgBlock:
		for _, e := range n.Exprs {
			if err := c.compileExpr(e); err != nil {
				return err
			}
			c.emit(bytecode.OpPrint)
		}
	case *ast.InputBlock:
		c.compileInput(n)
		c.emit(bytecode.OpPop)
	case *ast.IfBlock:
		return c.compileIf(n)
	case *ast.LoopBlock:
		return c.compileLoop(n.Body)
	case *ast.WhileBlock:
		return c.compileWhile(n)
	case *ast.ForBlock:
		return c.compileFor(n)
	case *ast.SetBlock:
		for _, a := range n.Assigns {
			if err := c.compileAssign(a); err != nil {
				return err
			}
		}
	case *ast.ReturnStmt:
		if err := c.compileExpr(n.Expr); err != nil {
			return err
		}
		c.emit(bytecode.OpRet)
	case *ast.BreakStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("break 只能在循环内使用")
		}
		pos := c.emitJumpPlaceholder(bytecode.OpJump)
		top := len(c.loops) - 1
		c.loops[top].breakJumps = append(c.loops[top].breakJumps, pos)
	case *ast.ContinueStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("continue 只能在循环内使用")
		}
		pos := c.emitJumpPlaceholder(bytecode.OpJump)
		top := len(c.loops) - 1
		c.loops[top].continueJumps = append(c.loops[top].continueJumps, pos)
	case *ast.ExprStmt:
		if err := c.compileExpr(n.Expr); err != nil {
			return err
		}
		c.emit(bytecode.OpPop)
	case *ast.FileBlock:
		return c.compileFileBlock(n)
	case *ast.HttpRouteBlock:
		return c.compileHttpRoute(n)
	case *ast.HttpStaticBlock:
		return c.compileHttpStatic(n)
	case *ast.HttpErrorBlock:
		return c.compileHttpError(n)
	case *ast.HttpServeBlock:
		return c.compileHttpServe(n)
	case *ast.DelayBlock:
		if err := c.compileExpr(n.Ms); err != nil {
			return err
		}
		c.emit(bytecode.OpDelay)
		for _, s := range n.Body {
			if err := c.compileStmt(s); err != nil {
				return err
			}
		}
	case *ast.FuncDecl:
	default:
		return fmt.Errorf("未知语句类型 %T", node)
	}
	return nil
}

func (c *Compiler) compileLoop(body []ast.Node) error {
	start := len(c.code)
	c.loops = append(c.loops, loopCtx{startPC: start})
	for _, s := range body {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	top := len(c.loops) - 1
	for _, j := range c.loops[top].continueJumps {
		c.patchJump(j, uint32(start))
	}
	c.emitJumpBack(bytecode.OpJump, uint32(start))
	end := uint32(len(c.code))
	for _, j := range c.loops[top].breakJumps {
		c.patchJump(j, end)
	}
	c.loops = c.loops[:top]
	return nil
}

func (c *Compiler) compileWhile(n *ast.WhileBlock) error {
	start := len(c.code)
	if err := c.compileExpr(n.Cond); err != nil {
		return err
	}
	jf := c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)
	c.loops = append(c.loops, loopCtx{startPC: start})
	for _, s := range n.Body {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	top := len(c.loops) - 1
	for _, j := range c.loops[top].continueJumps {
		c.patchJump(j, uint32(start))
	}
	c.emitJumpBack(bytecode.OpJump, uint32(start))
	c.patchJump(jf, uint32(len(c.code)))
	end := uint32(len(c.code))
	for _, j := range c.loops[top].breakJumps {
		c.patchJump(j, end)
	}
	c.loops = c.loops[:top]
	return nil
}

func (c *Compiler) compileFor(n *ast.ForBlock) error {
	iIdx := c.globalIdxOf("__for_i_" + n.VarName)
	c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstInt, Int: 0}))
	c.emitU32(bytecode.OpStoreGlobal, iIdx)

	start := len(c.code)
	c.emitU32(bytecode.OpLoadGlobal, iIdx)
	if err := c.compileExpr(n.Array); err != nil {
		return err
	}
	c.emit(bytecode.OpArrayLen)
	c.emit(bytecode.OpLt)
	jf := c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)

	idx := c.globalIdxOf(n.VarName)
	if err := c.compileExpr(n.Array); err != nil {
		return err
	}
	c.emitU32(bytecode.OpLoadGlobal, iIdx)
	c.emit(bytecode.OpIndexGet)
	c.emitU32(bytecode.OpStoreGlobal, idx)

	c.loops = append(c.loops, loopCtx{startPC: start})
	for _, s := range n.Body {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	top := len(c.loops) - 1
	continueTarget := len(c.code)
	for _, j := range c.loops[top].continueJumps {
		c.patchJump(j, uint32(continueTarget))
	}

	c.emitU32(bytecode.OpLoadGlobal, iIdx)
	c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstInt, Int: 1}))
	c.emit(bytecode.OpAdd)
	c.emitU32(bytecode.OpStoreGlobal, iIdx)
	c.emitJumpBack(bytecode.OpJump, uint32(start))

	c.patchJump(jf, uint32(len(c.code)))
	end := uint32(len(c.code))
	for _, j := range c.loops[top].breakJumps {
		c.patchJump(j, end)
	}
	c.loops = c.loops[:top]
	return nil
}

func (c *Compiler) compileIf(n *ast.IfBlock) error {
	if err := c.compileExpr(n.Cond); err != nil {
		return err
	}
	jf := c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)
	for _, s := range n.ThenBody {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	endJumps := []int{c.emitJumpPlaceholder(bytecode.OpJump)}

	for _, ei := range n.ElseIfs {
		c.patchJump(jf, uint32(len(c.code)))
		if err := c.compileExpr(ei.Cond); err != nil {
			return err
		}
		jf = c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)
		for _, s := range ei.Body {
			if err := c.compileStmt(s); err != nil {
				return err
			}
		}
		endJumps = append(endJumps, c.emitJumpPlaceholder(bytecode.OpJump))
	}

	c.patchJump(jf, uint32(len(c.code)))
	for _, s := range n.ElseBody {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	end := uint32(len(c.code))
	for _, j := range endJumps {
		c.patchJump(j, end)
	}
	return nil
}

func (c *Compiler) compileInput(n *ast.InputBlock) {
	c.emit(bytecode.OpInput)
	c.writeU32(uint32(n.InputID))
	var tt byte
	switch n.Type {
	case "int":
		tt = 0
	case "float":
		tt = 1
	default:
		tt = 2
	}
	c.code = append(c.code, tt)
	if n.HasMin {
		c.code = append(c.code, 1)
	} else {
		c.code = append(c.code, 0)
	}
	c.writeF64(n.MinF)
	if n.HasMax {
		c.code = append(c.code, 1)
	} else {
		c.code = append(c.code, 0)
	}
	c.writeF64(n.MaxF)
	c.writeU32(c.constStringIdx(n.Prompt))
	c.writeU32(c.optionalExprConst(n.Over))
	c.writeU32(c.optionalExprConst(n.Under))
	c.writeU32(c.optionalExprConst(n.Invalid))
}

func (c *Compiler) optionalExprConst(expr ast.Node) uint32 {
	if expr == nil {
		return 0xFFFFFFFF
	}
	if lit, ok := expr.(*ast.Literal); ok {
		if s, ok := lit.Value.(string); ok {
			return c.constStringIdx(s)
		}
	}
	if b, ok := expr.(*ast.Binary); ok && b.Op == "+" {
		if ls, ok1 := c.foldString(b.Left); ok1 {
			if rs, ok2 := c.foldString(b.Right); ok2 {
				return c.constStringIdx(ls + rs)
			}
		}
	}
	return 0xFFFFFFFF
}

func (c *Compiler) foldString(node ast.Node) (string, bool) {
	switch n := node.(type) {
	case *ast.Literal:
		if s, ok := n.Value.(string); ok {
			return s, true
		}
		return fmt.Sprintf("%v", n.Value), true
	case *ast.VarRef:
		if n.Name == "max" || n.Name == "min" {
			return "{" + n.Name + "}", true
		}
		return "", false
	}
	return "", false
}

func (c *Compiler) compileAssign(a ast.Assign) error {
	switch t := a.Target.(type) {
	case *ast.VarRef:
		idx, ok := c.globalIdx[t.Name]
		if !ok {
			return fmt.Errorf("set 未定义变量 %s", t.Name)
		}
		if err := c.compileExpr(a.Expr); err != nil {
			return err
		}
		c.emitU32(bytecode.OpStoreGlobal, idx)
	case *ast.Index:
		if err := c.compileExpr(t.Array); err != nil {
			return err
		}
		if err := c.compileExpr(t.Idx); err != nil {
			return err
		}
		if err := c.compileExpr(a.Expr); err != nil {
			return err
		}
		c.emit(bytecode.OpIndexSet)
	default:
		return fmt.Errorf("set 目标不支持 %T", a.Target)
	}
	return nil
}

func (c *Compiler) compileFileBlock(n *ast.FileBlock) error {
	opID := fileOpID(n.Op)
	if opID == 0 {
		return fmt.Errorf("未知文件操作 %s", n.Op)
	}
	args := c.fileArgs(n)
	for _, a := range args {
		if err := c.compileExpr(a); err != nil {
			return err
		}
	}
	c.emit(bytecode.OpCallBuiltin)
	c.code = append(c.code, opID, byte(len(args)))

	if n.Into != "" {
		idx := c.globalIdxOf(n.Into)
		c.emitU32(bytecode.OpStoreGlobal, idx)
	} else {
		c.emit(bytecode.OpPop)
	}
	return nil
}

func (c *Compiler) fileArgs(n *ast.FileBlock) []ast.Node {
	must := func(k string) ast.Node {
		v, ok := n.Args[k]
		if !ok {
			panic(fmt.Sprintf("file_%s 缺少 %s", n.Op, k))
		}
		return v
	}
	switch n.Op {
	case "write", "append":
		return []ast.Node{must("path"), must("content")}
	case "read", "exists", "delete", "mkdir":
		return []ast.Node{must("path")}
	case "rename":
		return []ast.Node{must("path"), must("newpath")}
	}
	return nil
}

func fileOpID(op string) byte {
	switch op {
	case "write":
		return 100
	case "append":
		return 101
	case "read":
		return 102
	case "exists":
		return 103
	case "rename":
		return 104
	case "delete":
		return 105
	case "mkdir":
		return 106
	}
	return 0
}

func (c *Compiler) compileHttpRoute(n *ast.HttpRouteBlock) error {
	var bodyIdx uint32 = 0xFFFFFFFF
	if n.Body != nil {
		if s, ok := c.evalToString(n.Body); ok {
			bodyIdx = c.constStringIdx(s)
		} else {
			return fmt.Errorf("http_route body 目前只支持字符串字面量或常量拼接")
		}
	}
	c.prog.HttpRoutes = append(c.prog.HttpRoutes, bytecode.HttpRouteInfo{
		Method: n.Method, Path: n.Path, BodyIdx: bodyIdx,
		File: n.File, Save: n.Save, Type: n.Type, Status: n.Status, MaxBody: n.MaxBody,
	})
	return nil
}

func (c *Compiler) compileHttpStatic(n *ast.HttpStaticBlock) error {
	c.prog.HttpStatic = &bytecode.HttpStaticInfo{Root: n.Root, Index: n.Index}
	return nil
}

func (c *Compiler) compileHttpError(n *ast.HttpErrorBlock) error {
	var bodyIdx uint32 = 0xFFFFFFFF
	if s, ok := c.evalToString(n.Body); ok {
		bodyIdx = c.constStringIdx(s)
	} else {
		return fmt.Errorf("http_error body 目前只支持字符串字面量")
	}
	c.prog.HttpErrors = append(c.prog.HttpErrors, bytecode.HttpErrorInfo{
		Status: n.Status, BodyIdx: bodyIdx, Type: n.Type,
	})
	return nil
}

func (c *Compiler) compileHttpServe(n *ast.HttpServeBlock) error {
	portIdx := c.globalIdxOf("port")
	c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstInt, Int: n.Port}))
	c.emitU32(bytecode.OpStoreGlobal, portIdx)

	for _, s := range n.Body {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	c.prog.HttpServe = &bytecode.HttpServeInfo{
		Port: n.Port, Log: n.Log || !n.HasLog, MaxConn: n.MaxConn,
	}
	c.emit(bytecode.OpCallBuiltin)
	c.code = append(c.code, 200, 0)
	return nil
}

func (c *Compiler) evalToString(node ast.Node) (string, bool) {
	switch n := node.(type) {
	case *ast.Literal:
		return fmt.Sprintf("%v", n.Value), true
	case *ast.Binary:
		if n.Op == "+" {
			l, ok1 := c.evalToString(n.Left)
			r, ok2 := c.evalToString(n.Right)
			if ok1 && ok2 {
				return l + r, true
			}
		}
	}
	return "", false
}

func (c *Compiler) compileExpr(node ast.Node) error {
	switch n := node.(type) {
	case *ast.Literal:
		switch v := n.Value.(type) {
		case int64:
			c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstInt, Int: v}))
		case float64:
			c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstFloat, Flt: v}))
		case string:
			c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstString, Str: v}))
		case bool:
			c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstBool, Bool: v}))
		case nil:
			c.emit(bytecode.OpPushNil)
		default:
			return fmt.Errorf("未知字面量类型 %T", v)
		}
	case *ast.VarRef:
		idx, ok := c.globalIdx[n.Name]
		if !ok {
			return fmt.Errorf("未定义变量 %s", n.Name)
		}
		c.emitU32(bytecode.OpLoadGlobal, idx)
	case *ast.ArrayLit:
		for _, e := range n.Elems {
			if err := c.compileExpr(e); err != nil {
				return err
			}
		}
		c.emitU32(bytecode.OpNewArray, uint32(len(n.Elems)))
	case *ast.Index:
		if err := c.compileExpr(n.Array); err != nil {
			return err
		}
		if err := c.compileExpr(n.Idx); err != nil {
			return err
		}
		c.emit(bytecode.OpIndexGet)
	case *ast.Unary:
		if err := c.compileExpr(n.Operand); err != nil {
			return err
		}
		switch n.Op {
		case "not":
			c.emit(bytecode.OpNot)
		case "-":
			c.emit(bytecode.OpNeg)
		default:
			return fmt.Errorf("未知一元运算符 %s", n.Op)
		}
	case *ast.Binary:
		if n.Op == "and" || n.Op == "or" {
			return c.compileShortCircuit(n)
		}
		if err := c.compileExpr(n.Left); err != nil {
			return err
		}
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		var op bytecode.Opcode
		switch n.Op {
		case "+":
			op = bytecode.OpAdd
		case "-":
			op = bytecode.OpSub
		case "*":
			op = bytecode.OpMul
		case "/":
			op = bytecode.OpDiv
		case "%":
			op = bytecode.OpMod
		case "==":
			op = bytecode.OpEq
		case "!=":
			op = bytecode.OpNeq
		case ">":
			op = bytecode.OpGt
		case "<":
			op = bytecode.OpLt
		case ">=":
			op = bytecode.OpGte
		case "<=":
			op = bytecode.OpLte
		default:
			return fmt.Errorf("未知运算符 %s", n.Op)
		}
		c.emit(op)
	case *ast.Call:
		if bid, ok := bytecode.BuiltinNames[n.Name]; ok {
			for _, a := range n.Args {
				if err := c.compileExpr(a); err != nil {
					return err
				}
			}
			c.emit(bytecode.OpCallBuiltin)
			c.code = append(c.code, bid, byte(len(n.Args)))
			return nil
		}
		fidx, ok := c.funcIdx[n.Name]
		if !ok {
			return fmt.Errorf("未定义函数 %s", n.Name)
		}
		for _, a := range n.Args {
			if err := c.compileExpr(a); err != nil {
				return err
			}
		}
		c.emit(bytecode.OpCall)
		c.writeU32(fidx)
		c.code = append(c.code, byte(len(n.Args)))
	default:
		return fmt.Errorf("未知表达式类型 %T", node)
	}
	return nil
}

func (c *Compiler) compileShortCircuit(n *ast.Binary) error {
	if err := c.compileExpr(n.Left); err != nil {
		return err
	}
	if n.Op == "and" {
		jf := c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		jf2 := c.emitJumpPlaceholder(bytecode.OpJumpIfFalse)
		c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstBool, Bool: true}))
		end := c.emitJumpPlaceholder(bytecode.OpJump)
		c.patchJump(jf, uint32(len(c.code)))
		c.patchJump(jf2, uint32(len(c.code)))
		c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstBool, Bool: false}))
		c.patchJump(end, uint32(len(c.code)))
	} else {
		jt := c.emitJumpPlaceholder(bytecode.OpJumpIfTrue)
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		jt2 := c.emitJumpPlaceholder(bytecode.OpJumpIfTrue)
		c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstBool, Bool: false}))
		end := c.emitJumpPlaceholder(bytecode.OpJump)
		c.patchJump(jt, uint32(len(c.code)))
		c.patchJump(jt2, uint32(len(c.code)))
		c.emitPushConst(c.addConst(bytecode.Constant{Tag: bytecode.ConstBool, Bool: true}))
		c.patchJump(end, uint32(len(c.code)))
	}
	return nil
}

func (c *Compiler) emit(op bytecode.Opcode) { c.code = append(c.code, byte(op)) }

func (c *Compiler) emitU32(op bytecode.Opcode, v uint32) {
	c.emit(op)
	c.writeU32(v)
}

func (c *Compiler) writeU32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	c.code = append(c.code, b[:]...)
}

func (c *Compiler) writeF64(v float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	c.code = append(c.code, b[:]...)
}

func (c *Compiler) emitPushConst(idx uint32) { c.emitU32(bytecode.OpPushConst, idx) }

func (c *Compiler) emitJumpPlaceholder(op bytecode.Opcode) int {
	c.emit(op)
	pos := len(c.code)
	c.code = append(c.code, 0, 0, 0, 0)
	return pos
}

func (c *Compiler) patchJump(pos int, target uint32) {
	binary.LittleEndian.PutUint32(c.code[pos:pos+4], target)
}

func (c *Compiler) emitJumpBack(op bytecode.Opcode, target uint32) {
	c.emitU32(op, target)
}

func (c *Compiler) addConst(k bytecode.Constant) uint32 {
	key := constKey(k)
	if idx, ok := c.constIndex[key]; ok {
		return idx
	}
	idx := uint32(len(c.prog.Consts))
	c.prog.Consts = append(c.prog.Consts, k)
	c.constIndex[key] = idx
	return idx
}

func (c *Compiler) constStringIdx(s string) uint32 {
	return c.addConst(bytecode.Constant{Tag: bytecode.ConstString, Str: s})
}

func (c *Compiler) globalIdxOf(name string) uint32 {
	if idx, ok := c.globalIdx[name]; ok {
		return idx
	}
	idx := uint32(len(c.prog.GlobalNames))
	c.prog.GlobalNames = append(c.prog.GlobalNames, name)
	c.globalIdx[name] = idx
	return idx
}

func constKey(k bytecode.Constant) string {
	switch k.Tag {
	case bytecode.ConstInt:
		return fmt.Sprintf("i:%d", k.Int)
	case bytecode.ConstFloat:
		return fmt.Sprintf("f:%v", k.Flt)
	case bytecode.ConstBool:
		return fmt.Sprintf("b:%v", k.Bool)
	case bytecode.ConstString:
		return "s:" + k.Str
	case bytecode.ConstNil:
		return "nil"
	}
	return ""
}