// Восстановление после синтаксической ошибки для LSP (ParseProgramLenient) -
// "Отложено - LSP не должен ГЛОХНУТЬ целиком из-за одной
// ошибки в файле". ParseProgram (настоящая сборка) этот файл не использует -
// там первая ошибка остаётся фатальной, как и всегда.
package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// topLevelStarters - токены, с которых может начинаться верхнеуровневое
// объявление (плюс DOC_COMMENT перед ним) - recoverToNextTopLevel
// останавливается на первом таком токене, встреченном на нулевой глубине
// скобок, то же множество, что разбирает ParseProgram в своём switch, плюс
// модификаторы модульной видимости (parseModuleVisibility) и сам
// doc-комментарий следующего объявления.
var topLevelStarters = map[lexer.TokenType]bool{
	lexer.KwFunction:  true,
	lexer.KwAbstract:  true,
	lexer.KwEntity:    true,
	lexer.KwClass:     true,
	lexer.KwStruct:    true,
	lexer.KwExport:    true,
	lexer.KwUse:       true,
	lexer.KwExternal:  true,
	lexer.KwEnum:      true,
	lexer.KwInterface: true,
	lexer.KwConst:     true,
	lexer.KwSynonym:   true,
	lexer.KwPrivate:   true,
	lexer.KwPublic:    true,
	lexer.KwInternal:  true,
	lexer.KwAsync:     true,
	lexer.DOC_COMMENT: true,
}

// recoverToNextTopLevel продвигает парсер от ТЕКУЩЕЙ позиции (обычно самое
// начало сломанного объявления - вызывающая сторона сама отвечает за откат
// p.pos перед вызовом) до границы следующего верхнеуровневого объявления,
// считая фигурные скобки, тем же приёмом, что и skipFunctionBody: токены
// внутри "{...}" (тело функции/сущности, вложенные структуры) не могут
// ложно сойти за начало следующего объявления, даже если совпадают по
// типу токена. Останавливается, не потребляя сам токен-границу, ровно на
// глубине 0 - вызывающий цикл ParseProgram видит его как обычное начало
// следующей итерации. Продвигается минимум на один токен всегда (иначе
// вызывающая сторона, откатившаяся на самое начало уже провалившегося
// объявления, зациклилась бы на нём же).
func (p *Parser) recoverToNextTopLevel() {
	p.advance()
	// Остаток заголовка ТОГО ЖЕ объявления ("публичная функция",
	// "экспортировать абстрактная сущность", перевод строки после его
	// doc-комментария) - тоже начала объявлений, без пропуска разбор
	// вернулся бы к нему же и повторил ту же ошибку.
	for p.cur().Type == lexer.NEWLINE || (topLevelStarters[p.cur().Type] && p.cur().Type != lexer.DOC_COMMENT && p.cur().Type != lexer.EOF) {
		p.advance()
	}
	depth := 0
	for {
		switch p.cur().Type {
		case lexer.EOF:
			return
		case lexer.LBRACE:
			depth++
		case lexer.RBRACE:
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && topLevelStarters[p.cur().Type] {
				return
			}
		}
		p.advance()
	}
}

// ParseProgramLenient - как ParseProgram, но для LSP: синтаксическая ошибка
// внутри одного верхнеуровневого объявления (функция/сущность/интерфейс/...)
// не обнуляет разбор всего файла - объявление, на котором она произошла,
// отсутствует в результирующем ast.Program (или несёт то, что успело
// накопиться до сбоя, для extern-деклараций и т. п. - см. вызывающие места
// в parser.go), все остальные объявления файла разобраны как обычно.
// Возвращаемый *ast.Program никогда не nil, даже если ни одно объявление не
// разобралось (пустая программа). Возвращаемый []error пуст тогда и только
// тогда, когда результат идентичен ParseProgram (ни одной ошибки не было).
func (p *Parser) ParseProgramLenient() (*ast.Program, []error) {
	p.lenient = true
	prog, err := p.ParseProgram()
	if prog == nil {
		prog = &ast.Program{}
	}
	if err != nil {
		// Защитный случай - в lenient-режиме ParseProgram сама не должна
		// возвращать фатальную ошибку (все ошибки уходят в p.errors), но
		// если какой-то путь всё же это делает (например, полностью пустой
		// токен-поток), она не должна теряться молча.
		p.errors = append(p.errors, err)
	}
	return prog, p.errors
}

// recoverToNextStatement продвигает парсер от start (начало сломанной
// инструкции внутри блока) до конца этой инструкции: первого разделителя
// (перенос строки/";") на нулевой глубине скобок, либо закрывающей скобки
// самого блока - её не потребляет, это граница для parseBlock. Вложенные
// "(...)"/"[...]"/"{...}" (тело "если"/"пока", аргументы на нескольких
// строках) пропускаются целиком. Продвигается минимум на один токен.
func (p *Parser) recoverToNextStatement(start int) {
	p.pos = start
	p.advance()
	depth := 0
	for {
		switch p.cur().Type {
		case lexer.EOF:
			return
		case lexer.LBRACE, lexer.LPAREN, lexer.LBRACKET:
			depth++
		case lexer.RBRACE, lexer.RPAREN, lexer.RBRACKET:
			if depth == 0 {
				return
			}
			depth--
		case lexer.NEWLINE, lexer.SEMICOLON:
			if depth == 0 {
				return
			}
		}
		p.advance()
	}
}
