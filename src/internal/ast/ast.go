package ast

type Node interface{}

type Program struct {
	Stmts []Node
}

// ---------- 语句 ----------

type VarBlock struct {
	Decls   []VarDecl
	Imports []string
}

type VarDecl struct {
	Name    string
	IsInput bool
	InputID int64
	Expr    Node
	// 数组内联限制（仅 Expr 是 ArrayLit 时有效）
	HasLimit bool
	LimitMin int64
	LimitMax int64
}

type MsgBlock struct {
	Exprs []Node
}

type InputBlock struct {
	InputID int64
	Type    string // "int" / "float" / "string"
	HasMin  bool
	MinF    float64
	HasMax  bool
	MaxF    float64
	Prompt  string
	Over    Node
	Under   Node
	Invalid Node
}

type IfBlock struct {
	Cond     Node
	ThenBody []Node
	ElseIfs  []ElseIfBranch
	ElseBody []Node
}

type ElseIfBranch struct {
	Cond Node
	Body []Node
}

type LoopBlock struct {
	Body []Node
}

type WhileBlock struct {
	Cond Node
	Body []Node
}

type ForBlock struct {
	VarName string
	Array   Node
	Body    []Node
}

type SetBlock struct {
	Assigns []Assign
}

type Assign struct {
	Target Node // VarRef 或 Index
	Expr   Node
}

type FuncDecl struct {
	Name   string
	Params []string
	Body   []Node
}

type ReturnStmt struct {
	Expr Node
}

type BreakStmt struct{}

type ContinueStmt struct{}

type ExprStmt struct {
	Expr Node
}

type FileBlock struct {
	Op   string
	Args map[string]Node
	Into string
}

type HttpRouteBlock struct {
	Method  string
	Path    string
	Body    Node
	File    string
	Save    string
	Type    string
	Status  int64
	MaxBody int64
}

type HttpStaticBlock struct {
	Root  string
	Index string
}

type HttpServeBlock struct {
	Port    int64
	Log     bool
	HasLog  bool
	MaxConn int64
	Body    []Node
}

type DelayBlock struct {
	Ms   Node
	Body []Node
}

type HttpErrorBlock struct {
	Status int64
	Body   Node
	Type   string
}

// ---------- 表达式 ----------

type Literal struct {
	Value interface{} // int64 / float64 / string / bool / nil
}

type VarRef struct {
	Name string
}

type ArrayLit struct {
	Elems []Node
}

type Index struct {
	Array Node
	Idx   Node
}

type Binary struct {
	Op    string
	Left  Node
	Right Node
}

type Unary struct {
	Op      string
	Operand Node
}

type Call struct {
	Name string
	Args []Node
}