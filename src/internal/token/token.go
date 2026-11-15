package token

type Type string

const (
	ILLEGAL Type = "ILLEGAL"
	EOF     Type = "EOF"
	NEWLINE Type = "NEWLINE"

	IDENT  Type = "IDENT"
	NUMBER Type = "NUMBER"
	STRING Type = "STRING"

	VAR      Type = "VAR"
	MSG      Type = "MSG"
	INPUT    Type = "INPUT"
	IF       Type = "IF"
	ELSE     Type = "ELSE"
	LOOP     Type = "LOOP"
	WHILE    Type = "WHILE"
	FOR      Type = "FOR"
	IN       Type = "IN"
	BREAK    Type = "BREAK"
	CONTINUE Type = "CONTINUE"
	SET      Type = "SET"
	FUNC     Type = "FUNC"
	RETURN   Type = "RETURN"
	AND      Type = "AND"
	OR       Type = "OR"
	NOT      Type = "NOT"
	TRUE     Type = "TRUE"
	FALSE    Type = "FALSE"
	NIL      Type = "NIL"
	LIMIT    Type = "LIMIT"

	PLUS     Type = "+"
	MINUS    Type = "-"
	STAR     Type = "*"
	SLASH    Type = "/"
	PERCENT  Type = "%"
	EQ       Type = "="
	EQEQ     Type = "=="
	NEQ      Type = "!="
	GT       Type = ">"
	LT       Type = "<"
	GTE      Type = ">="
	LTE      Type = "<="

	LBRACE   Type = "{"
	RBRACE   Type = "}"
	LPAREN   Type = "("
	RPAREN   Type = ")"
	LBRACKET Type = "["
	RBRACKET Type = "]"
	COLON    Type = ":"
	COMMA    Type = ","
)

type Token struct {
	Type    Type
	Literal string
	Line    int
	Col     int
}

var Keywords = map[string]Type{
	"var":      VAR,
	"msg":      MSG,
	"input":    INPUT,
	"if":       IF,
	"else":     ELSE,
	"loop":     LOOP,
	"while":    WHILE,
	"for":      FOR,
	"in":       IN,
	"break":    BREAK,
	"continue": CONTINUE,
	"set":      SET,
	"func":     FUNC,
	"return":   RETURN,
	"and":      AND,
	"or":       OR,
	"not":      NOT,
	"true":     TRUE,
	"false":    FALSE,
	"nil":      NIL,
	"limit":    LIMIT,
}

func LookupIdent(ident string) Type {
	if t, ok := Keywords[ident]; ok {
		return t
	}
	return IDENT
}