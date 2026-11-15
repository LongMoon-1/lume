package lexer

import (
	"unicode"

	"top.longmoon/lume/internal/token"
)

type Lexer struct {
	input  []rune
	pos    int
	line   int
	col    int
	tokens []token.Token
}

func New(input string) *Lexer {
	return &Lexer{input: []rune(input), line: 1, col: 1}
}

func (l *Lexer) peek(offset int) rune {
	idx := l.pos + offset
	if idx >= len(l.input) {
		return 0
	}
	return l.input[idx]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) add(typ token.Type, lit string, line, col int) {
	l.tokens = append(l.tokens, token.Token{Type: typ, Literal: lit, Line: line, Col: col})
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func (l *Lexer) Tokenize() []token.Token {
	for l.pos < len(l.input) {
		ch := l.peek(0)

		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
			continue
		}

		if ch == '\n' {
			line, col := l.line, l.col
			l.advance()
			l.add(token.NEWLINE, "\n", line, col)
			continue
		}

		// 注释：-- 或 --[[ ]]
		if ch == '-' && l.peek(1) == '-' {
			line, col := l.line, l.col
			l.advance()
			l.advance()
			if l.peek(0) == '[' && l.peek(1) == '[' {
				l.advance()
				l.advance()
				for {
					if l.pos >= len(l.input) {
						l.add(token.ILLEGAL, "未闭合的多行注释", line, col)
						return l.tokens
					}
					if l.peek(0) == ']' && l.peek(1) == ']' {
						l.advance()
						l.advance()
						break
					}
					l.advance()
				}
			} else {
				for l.pos < len(l.input) && l.peek(0) != '\n' {
					l.advance()
				}
			}
			continue
		}

		// 字符串（支持转义）
		if ch == '"' {
			line, col := l.line, l.col
			l.advance()
			var buf []rune
			for {
				if l.pos >= len(l.input) {
					l.add(token.ILLEGAL, "未闭合字符串", line, col)
					return l.tokens
				}
				c := l.peek(0)
				if c == '"' {
					l.advance()
					break
				}
				if c == '\\' {
					l.advance()
					if l.pos >= len(l.input) {
						l.add(token.ILLEGAL, "字符串转义未完成", line, col)
						return l.tokens
					}
					esc := l.advance()
					switch esc {
					case 'n':
						buf = append(buf, '\n')
					case 't':
						buf = append(buf, '\t')
					case 'r':
						buf = append(buf, '\r')
					case '"':
						buf = append(buf, '"')
					case '\\':
						buf = append(buf, '\\')
					default:
						buf = append(buf, '\\', esc)
					}
					continue
				}
				buf = append(buf, l.advance())
			}
			l.add(token.STRING, string(buf), line, col)
			continue
		}

		// 数字
		if unicode.IsDigit(ch) {
			line, col := l.line, l.col
			var buf []rune
			for unicode.IsDigit(l.peek(0)) {
				buf = append(buf, l.advance())
			}
			if l.peek(0) == '.' && unicode.IsDigit(l.peek(1)) {
				buf = append(buf, l.advance())
				for unicode.IsDigit(l.peek(0)) {
					buf = append(buf, l.advance())
				}
			}
			l.add(token.NUMBER, string(buf), line, col)
			continue
		}

		// 标识符 / 关键字
		if isIdentStart(ch) {
			line, col := l.line, l.col
			var buf []rune
			for isIdentPart(l.peek(0)) {
				buf = append(buf, l.advance())
			}
			lit := string(buf)
			l.add(token.LookupIdent(lit), lit, line, col)
			continue
		}

		line, col := l.line, l.col
		switch ch {
		case '+':
			l.advance()
			l.add(token.PLUS, "+", line, col)
		case '-':
			l.advance()
			l.add(token.MINUS, "-", line, col)
		case '*':
			l.advance()
			l.add(token.STAR, "*", line, col)
		case '/':
			l.advance()
			l.add(token.SLASH, "/", line, col)
		case '%':
			l.advance()
			l.add(token.PERCENT, "%", line, col)
		case '=':
			l.advance()
			if l.peek(0) == '=' {
				l.advance()
				l.add(token.EQEQ, "==", line, col)
			} else {
				l.add(token.EQ, "=", line, col)
			}
		case '!':
			l.advance()
			if l.peek(0) == '=' {
				l.advance()
				l.add(token.NEQ, "!=", line, col)
			} else {
				l.add(token.ILLEGAL, "!", line, col)
			}
		case '>':
			l.advance()
			if l.peek(0) == '=' {
				l.advance()
				l.add(token.GTE, ">=", line, col)
			} else {
				l.add(token.GT, ">", line, col)
			}
		case '<':
			l.advance()
			if l.peek(0) == '=' {
				l.advance()
				l.add(token.LTE, "<=", line, col)
			} else {
				l.add(token.LT, "<", line, col)
			}
		case '{':
			l.advance()
			l.add(token.LBRACE, "{", line, col)
		case '}':
			l.advance()
			l.add(token.RBRACE, "}", line, col)
		case '(':
			l.advance()
			l.add(token.LPAREN, "(", line, col)
		case ')':
			l.advance()
			l.add(token.RPAREN, ")", line, col)
		case '[':
			l.advance()
			l.add(token.LBRACKET, "[", line, col)
		case ']':
			l.advance()
			l.add(token.RBRACKET, "]", line, col)
		case ':':
			l.advance()
			l.add(token.COLON, ":", line, col)
		case ',':
			l.advance()
			l.add(token.COMMA, ",", line, col)
		default:
			l.advance()
			l.add(token.ILLEGAL, string(ch), line, col)
		}
	}
	l.add(token.EOF, "", l.line, l.col)
	return l.tokens
}