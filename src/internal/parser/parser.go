package parser

import (
	"fmt"
	"strconv"
	"strings"

	"top.longmoon/lume/internal/ast"
	"top.longmoon/lume/internal/token"
)

type Parser struct {
	tokens []token.Token
	pos    int
}

func New(tokens []token.Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) peek(offset int) token.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[idx]
}

func (p *Parser) advance() token.Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) skipNewlines() {
	for p.peek(0).Type == token.NEWLINE {
		p.advance()
	}
}

func (p *Parser) expect(typ token.Type) token.Token {
	t := p.advance()
	if t.Type != typ {
		panic(fmt.Sprintf("语法错误 L%d:%d 期望 %s，得到 %s(%q)",
			t.Line, t.Col, typ, t.Type, t.Literal))
	}
	return t
}

func (p *Parser) Parse() *ast.Program {
	prog := &ast.Program{}
	p.skipNewlines()
	for p.peek(0).Type != token.EOF {
		prog.Stmts = append(prog.Stmts, p.parseStmt())
		p.skipNewlines()
	}
	return prog
}

func (p *Parser) parseStmt() ast.Node {
	t := p.peek(0)
	switch t.Type {
	case token.VAR:
		return p.parseVarBlock()
	case token.MSG:
		return p.parseMsgBlock()
	case token.INPUT:
		return p.parseInputBlock()
	case token.IF:
		return p.parseIfBlock()
	case token.LOOP:
		return p.parseLoopBlock()
	case token.WHILE:
		return p.parseWhileBlock()
	case token.FOR:
		return p.parseForBlock()
	case token.SET:
		return p.parseSetBlock()
	case token.FUNC:
		return p.parseFuncDecl()
	case token.RETURN:
		p.advance()
		p.expect(token.COLON)
		return &ast.ReturnStmt{Expr: p.parseExpr()}
	case token.BREAK:
		p.advance()
		return &ast.BreakStmt{}
	case token.CONTINUE:
		p.advance()
		return &ast.ContinueStmt{}
	case token.IDENT:
		if p.peek(1).Type == token.LBRACE {
			name := t.Literal
			if strings.HasPrefix(name, "file_") {
				return p.parseFileBlock()
			}
			switch name {
			case "http_route":
				return p.parseHttpRouteBlock()
			case "http_static":
				return p.parseHttpStaticBlock()
			case "http_serve":
				return p.parseHttpServeBlock()
			case "http_error":
				return p.parseHttpErrorBlock()
			case "delay":
				return p.parseDelayBlock()
			}
		}
		return &ast.ExprStmt{Expr: p.parseExpr()}
	default:
		return &ast.ExprStmt{Expr: p.parseExpr()}
	}
}

func (p *Parser) parseVarBlock() ast.Node {
	p.expect(token.VAR)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.VarBlock{}
	for p.peek(0).Type != token.RBRACE {
		nameTok := p.expect(token.IDENT)
		name := nameTok.Literal
		p.expect(token.COLON)

		if name == "import" {
			strTok := p.expect(token.STRING)
			block.Imports = append(block.Imports, strTok.Literal)
			p.skipNewlines()
			continue
		}

		if p.peek(0).Type == token.INPUT {
			p.advance()
			var id int64 = -1
			if p.peek(0).Type == token.EQ {
				p.advance()
				numTok := p.expect(token.NUMBER)
				v, err := strconv.ParseInt(numTok.Literal, 10, 64)
				if err != nil {
					panic(fmt.Sprintf("input id 必须是整数 L%d:%d", numTok.Line, numTok.Col))
				}
				id = v
			}
			if id < 0 {
				panic(fmt.Sprintf("变量 %s 的 input 必须指定 id", name))
			}
			block.Decls = append(block.Decls, ast.VarDecl{
				Name: name, IsInput: true, InputID: id,
			})
		} else {
			expr := p.parseExpr()
			decl := ast.VarDecl{Name: name, Expr: expr}
			// 数组内联限制：limit(min, max)
			if p.peek(0).Type == token.LIMIT {
				p.advance()
				p.expect(token.LPAREN)
				minTok := p.expect(token.NUMBER)
				minV, _ := strconv.ParseInt(minTok.Literal, 10, 64)
				p.expect(token.COMMA)
				maxTok := p.expect(token.NUMBER)
				maxV, _ := strconv.ParseInt(maxTok.Literal, 10, 64)
				p.expect(token.RPAREN)
				decl.HasLimit = true
				decl.LimitMin = minV
				decl.LimitMax = maxV
			}
			block.Decls = append(block.Decls, decl)
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return block
}

func (p *Parser) parseMsgBlock() ast.Node {
	p.expect(token.MSG)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.MsgBlock{}
	for p.peek(0).Type != token.RBRACE {
		block.Exprs = append(block.Exprs, p.parseExpr())
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return block
}

func (p *Parser) parseInputBlock() ast.Node {
	p.expect(token.INPUT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.InputBlock{InputID: -1, Type: "int", Prompt: "请输入"}
	for p.peek(0).Type != token.RBRACE {
		t := p.peek(0)
		switch t.Type {
		case token.IDENT:
			key := t.Literal
			p.advance()
			p.expect(token.COLON)
			switch key {
			case "id":
				numTok := p.expect(token.NUMBER)
				v, err := strconv.ParseInt(numTok.Literal, 10, 64)
				if err != nil {
					panic(fmt.Sprintf("input id 必须是整数 L%d:%d", numTok.Line, numTok.Col))
				}
				block.InputID = v
			case "min", "max":
				numTok := p.expect(token.NUMBER)
				v, _ := strconv.ParseFloat(numTok.Literal, 64)
				if key == "min" {
					block.HasMin = true
					block.MinF = v
				} else {
					block.HasMax = true
					block.MaxF = v
				}
			case "type":
				typ := p.expect(token.STRING).Literal
				if typ != "int" && typ != "float" && typ != "string" {
					panic(fmt.Sprintf("input type 只能是 int/float/string，得到 %q L%d:%d",
						typ, t.Line, t.Col))
				}
				block.Type = typ
			case "over":
				block.Over = p.parseExpr()
			case "under":
				block.Under = p.parseExpr()
			case "invalid":
				block.Invalid = p.parseExpr()
			default:
				panic(fmt.Sprintf("input 块内未知属性 %s L%d:%d", key, t.Line, t.Col))
			}
		case token.STRING:
			block.Prompt = t.Literal
			p.advance()
		default:
			panic(fmt.Sprintf("input 块内意外 token %s L%d:%d", t.Type, t.Line, t.Col))
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.InputID < 0 {
		panic("input 块缺少 id")
	}
	return block
}

func (p *Parser) parseIfBlock() ast.Node {
	p.expect(token.IF)
	p.expect(token.LBRACE)
	p.skipNewlines()
	cond := p.parseExpr()
	p.skipNewlines()
	thenBody := p.parseInnerBlock()
	p.skipNewlines()
	block := &ast.IfBlock{Cond: cond, ThenBody: thenBody}

	for p.peek(0).Type == token.ELSE {
		p.advance()
		// else if
		if p.peek(0).Type == token.IF {
			p.advance()
			p.skipNewlines()
			c := p.parseExpr()
			p.skipNewlines()
			b := p.parseInnerBlock()
			p.skipNewlines()
			block.ElseIfs = append(block.ElseIfs, ast.ElseIfBranch{Cond: c, Body: b})
			continue
		}
		// else
		p.skipNewlines()
		block.ElseBody = p.parseInnerBlock()
		p.skipNewlines()
		break
	}
	p.expect(token.RBRACE)
	return block
}

func (p *Parser) parseLoopBlock() ast.Node {
	p.expect(token.LOOP)
	p.expect(token.LBRACE)
	p.skipNewlines()
	body := []ast.Node{}
	for p.peek(0).Type != token.RBRACE {
		body = append(body, p.parseStmt())
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return &ast.LoopBlock{Body: body}
}

func (p *Parser) parseWhileBlock() ast.Node {
	p.expect(token.WHILE)
	p.expect(token.LBRACE)
	p.skipNewlines()
	cond := p.parseExpr()
	p.skipNewlines()
	body := p.parseInnerBlock()
	p.skipNewlines()
	p.expect(token.RBRACE)
	return &ast.WhileBlock{Cond: cond, Body: body}
}

func (p *Parser) parseForBlock() ast.Node {
	p.expect(token.FOR)
	p.expect(token.LBRACE)
	p.skipNewlines()
	varTok := p.expect(token.IDENT)
	p.expect(token.IN)
	arr := p.parseExpr()
	p.skipNewlines()
	body := p.parseInnerBlock()
	p.skipNewlines()
	p.expect(token.RBRACE)
	return &ast.ForBlock{VarName: varTok.Literal, Array: arr, Body: body}
}

func (p *Parser) parseSetBlock() ast.Node {
	p.expect(token.SET)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.SetBlock{}
	for p.peek(0).Type != token.RBRACE {
		// 目标可能是 var 或 a[i]
		nameTok := p.expect(token.IDENT)
		var target ast.Node = &ast.VarRef{Name: nameTok.Literal}
		for p.peek(0).Type == token.LBRACKET {
			p.advance()
			idx := p.parseExpr()
			p.expect(token.RBRACKET)
			target = &ast.Index{Array: target, Idx: idx}
		}
		p.expect(token.COLON)
		expr := p.parseExpr()
		block.Assigns = append(block.Assigns, ast.Assign{Target: target, Expr: expr})
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return block
}

func (p *Parser) parseFuncDecl() ast.Node {
	p.expect(token.FUNC)
	name := p.expect(token.IDENT).Literal
	p.expect(token.LPAREN)
	params := []string{}
	if p.peek(0).Type != token.RPAREN {
		params = append(params, p.expect(token.IDENT).Literal)
		for p.peek(0).Type == token.COMMA {
			p.advance()
			params = append(params, p.expect(token.IDENT).Literal)
		}
	}
	p.expect(token.RPAREN)
	body := p.parseInnerBlock()
	return &ast.FuncDecl{Name: name, Params: params, Body: body}
}

func (p *Parser) parseInnerBlock() []ast.Node {
	p.expect(token.LBRACE)
	p.skipNewlines()
	body := []ast.Node{}
	for p.peek(0).Type != token.RBRACE {
		body = append(body, p.parseStmt())
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return body
}

func (p *Parser) parseFileBlock() ast.Node {
	nameTok := p.expect(token.IDENT)
	op := strings.TrimPrefix(nameTok.Literal, "file_")
	switch op {
	case "write", "append", "read", "exists", "rename", "delete", "mkdir":
	default:
		panic(fmt.Sprintf("未知文件块 file_%s L%d:%d", op, nameTok.Line, nameTok.Col))
	}
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.FileBlock{Op: op, Args: map[string]ast.Node{}}
	for p.peek(0).Type != token.RBRACE {
		keyTok := p.expect(token.IDENT)
		key := keyTok.Literal
		p.expect(token.COLON)
		if key == "into" {
			intoTok := p.expect(token.IDENT)
			block.Into = intoTok.Literal
		} else {
			block.Args[key] = p.parseExpr()
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return block
}

func (p *Parser) parseHttpRouteBlock() ast.Node {
	p.expect(token.IDENT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.HttpRouteBlock{Method: "GET", Type: "text/plain; charset=utf-8", Status: 200}
	for p.peek(0).Type != token.RBRACE {
		keyTok := p.expect(token.IDENT)
		key := keyTok.Literal
		p.expect(token.COLON)
		switch key {
		case "method":
			block.Method = strings.ToUpper(p.expect(token.STRING).Literal)
		case "path":
			block.Path = p.expect(token.STRING).Literal
		case "body":
			block.Body = p.parseExpr()
		case "file":
			block.File = p.expect(token.STRING).Literal
		case "save":
			block.Save = p.expect(token.STRING).Literal
		case "type":
			block.Type = p.expect(token.STRING).Literal
		case "status":
			numTok := p.expect(token.NUMBER)
			v, _ := strconv.ParseInt(numTok.Literal, 10, 64)
			block.Status = v
		case "maxbody":
			numTok := p.expect(token.NUMBER)
			v, _ := strconv.ParseInt(numTok.Literal, 10, 64)
			block.MaxBody = v
		default:
			panic(fmt.Sprintf("http_route 未知属性 %s L%d:%d", key, keyTok.Line, keyTok.Col))
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.Path == "" {
		panic("http_route 缺少 path")
	}
	if block.Body == nil && block.File == "" {
		panic("http_route 需要 body 或 file")
	}
	return block
}

func (p *Parser) parseHttpStaticBlock() ast.Node {
	p.expect(token.IDENT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.HttpStaticBlock{Index: "index.html"}
	for p.peek(0).Type != token.RBRACE {
		keyTok := p.expect(token.IDENT)
		key := keyTok.Literal
		p.expect(token.COLON)
		switch key {
		case "root":
			block.Root = p.expect(token.STRING).Literal
		case "index":
			block.Index = p.expect(token.STRING).Literal
		default:
			panic(fmt.Sprintf("http_static 未知属性 %s L%d:%d", key, keyTok.Line, keyTok.Col))
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.Root == "" {
		panic("http_static 缺少 root")
	}
	return block
}

func (p *Parser) parseHttpServeBlock() ast.Node {
	p.expect(token.IDENT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.HttpServeBlock{Port: -1, Log: true}
	for p.peek(0).Type != token.RBRACE {
		t := p.peek(0)
		if t.Type == token.IDENT && p.peek(1).Type == token.COLON {
			key := t.Literal
			p.advance()
			p.advance()
			switch key {
			case "port":
				numTok := p.expect(token.NUMBER)
				v, _ := strconv.ParseInt(numTok.Literal, 10, 64)
				block.Port = v
			case "log":
				if p.peek(0).Type == token.TRUE {
					p.advance()
					block.Log = true
					block.HasLog = true
				} else if p.peek(0).Type == token.FALSE {
					p.advance()
					block.Log = false
					block.HasLog = true
				} else {
					panic("http_serve log 必须是 true 或 false")
				}
			case "maxconn":
				numTok := p.expect(token.NUMBER)
				v, _ := strconv.ParseInt(numTok.Literal, 10, 64)
				block.MaxConn = v
			default:
				panic(fmt.Sprintf("http_serve 未知属性 %s L%d:%d", key, t.Line, t.Col))
			}
			p.skipNewlines()
			continue
		}
		block.Body = append(block.Body, p.parseStmt())
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.Port < 0 {
		panic("http_serve 缺少 port")
	}
	return block
}

func (p *Parser) parseHttpErrorBlock() ast.Node {
	p.expect(token.IDENT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.HttpErrorBlock{Status: -1, Type: "text/html; charset=utf-8"}
	for p.peek(0).Type != token.RBRACE {
		keyTok := p.expect(token.IDENT)
		key := keyTok.Literal
		p.expect(token.COLON)
		switch key {
		case "status":
			numTok := p.expect(token.NUMBER)
			v, _ := strconv.ParseInt(numTok.Literal, 10, 64)
			block.Status = v
		case "body":
			block.Body = p.parseExpr()
		case "type":
			block.Type = p.expect(token.STRING).Literal
		default:
			panic(fmt.Sprintf("http_error 未知属性 %s L%d:%d", key, keyTok.Line, keyTok.Col))
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.Status < 0 {
		panic("http_error 缺少 status")
	}
	if block.Body == nil {
		panic("http_error 缺少 body")
	}
	return block
}

func (p *Parser) parseDelayBlock() ast.Node {
	p.expect(token.IDENT)
	p.expect(token.LBRACE)
	p.skipNewlines()
	block := &ast.DelayBlock{}
	for p.peek(0).Type != token.RBRACE {
		t := p.peek(0)
		if t.Type == token.IDENT && t.Literal == "ms" && p.peek(1).Type == token.COLON {
			p.advance()
			p.advance()
			block.Ms = p.parseExpr()
			p.skipNewlines()
			continue
		}
		block.Body = append(block.Body, p.parseStmt())
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	if block.Ms == nil {
		panic("delay 缺少 ms")
	}
	return block
}

// ---------- 表达式 ----------

func (p *Parser) parseExpr() ast.Node {
	return p.parseOr()
}

func (p *Parser) parseOr() ast.Node {
	left := p.parseAnd()
	for p.peek(0).Type == token.OR {
		p.advance()
		right := p.parseAnd()
		left = &ast.Binary{Op: "or", Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAnd() ast.Node {
	left := p.parseCompare()
	for p.peek(0).Type == token.AND {
		p.advance()
		right := p.parseCompare()
		left = &ast.Binary{Op: "and", Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseCompare() ast.Node {
	left := p.parseAdd()
	for {
		t := p.peek(0)
		var op string
		switch t.Type {
		case token.EQEQ, token.EQ:
			op = "=="
		case token.NEQ:
			op = "!="
		case token.GT:
			op = ">"
		case token.LT:
			op = "<"
		case token.GTE:
			op = ">="
		case token.LTE:
			op = "<="
		default:
			return left
		}
		p.advance()
		right := p.parseAdd()
		left = &ast.Binary{Op: op, Left: left, Right: right}
	}
}

func (p *Parser) parseAdd() ast.Node {
	left := p.parseMul()
	for {
		t := p.peek(0)
		if t.Type != token.PLUS && t.Type != token.MINUS {
			return left
		}
		op := t.Literal
		p.advance()
		right := p.parseMul()
		left = &ast.Binary{Op: op, Left: left, Right: right}
	}
}

func (p *Parser) parseMul() ast.Node {
	left := p.parseUnary()
	for {
		t := p.peek(0)
		if t.Type != token.STAR && t.Type != token.SLASH && t.Type != token.PERCENT {
			return left
		}
		op := t.Literal
		p.advance()
		right := p.parseUnary()
		left = &ast.Binary{Op: op, Left: left, Right: right}
	}
}

func (p *Parser) parseUnary() ast.Node {
	t := p.peek(0)
	if t.Type == token.NOT {
		p.advance()
		return &ast.Unary{Op: "not", Operand: p.parseUnary()}
	}
	if t.Type == token.MINUS {
		p.advance()
		return &ast.Unary{Op: "-", Operand: p.parseUnary()}
	}
	return p.parseCall()
}

func (p *Parser) parseCall() ast.Node {
	expr := p.parsePrimary()
	// 索引链：a[0][1]
	for p.peek(0).Type == token.LBRACKET {
		p.advance()
		idx := p.parseExpr()
		p.expect(token.RBRACKET)
		expr = &ast.Index{Array: expr, Idx: idx}
	}
	if p.peek(0).Type == token.LPAREN {
		ref, ok := expr.(*ast.VarRef)
		if !ok {
			return expr
		}
		p.advance()
		args := []ast.Node{}
		if p.peek(0).Type != token.RPAREN {
			args = append(args, p.parseExpr())
			for p.peek(0).Type == token.COMMA {
				p.advance()
				args = append(args, p.parseExpr())
			}
		}
		p.expect(token.RPAREN)
		return &ast.Call{Name: ref.Name, Args: args}
	}
	return expr
}

func (p *Parser) parsePrimary() ast.Node {
	t := p.peek(0)
	switch t.Type {
	case token.NUMBER:
		p.advance()
		if v, err := strconv.ParseInt(t.Literal, 10, 64); err == nil {
			return &ast.Literal{Value: v}
		}
		f, _ := strconv.ParseFloat(t.Literal, 64)
		return &ast.Literal{Value: f}
	case token.STRING:
		p.advance()
		return &ast.Literal{Value: t.Literal}
	case token.TRUE:
		p.advance()
		return &ast.Literal{Value: true}
	case token.FALSE:
		p.advance()
		return &ast.Literal{Value: false}
	case token.NIL:
		p.advance()
		return &ast.Literal{Value: nil}
	case token.IDENT:
		p.advance()
		return &ast.VarRef{Name: t.Literal}
	case token.LPAREN:
		p.advance()
		expr := p.parseExpr()
		p.expect(token.RPAREN)
		return expr
	case token.LBRACKET:
		p.advance()
		elems := []ast.Node{}
		if p.peek(0).Type != token.RBRACKET {
			elems = append(elems, p.parseExpr())
			for p.peek(0).Type == token.COMMA {
				p.advance()
				p.skipNewlines()
				elems = append(elems, p.parseExpr())
			}
		}
		p.expect(token.RBRACKET)
		return &ast.ArrayLit{Elems: elems}
	default:
		panic(fmt.Sprintf("语法错误 L%d:%d 无法解析表达式 token %s(%q)",
			t.Line, t.Col, t.Type, t.Literal))
	}
}