// Package dbscheme классифицирует строку подключения "БД" по префиксу
// схемы ("sqlite:"/"postgres://"/"postgresql://"/"mysql://") - единый
// источник истины для трёх независимых потребителей (internal/semantic -
// статический анализ literal-аргумента "подключиться_к_бд(...)" ради
// условной линковки, internal/interp - выбор драйвера в рантайме,
// internal/backend/cbackend - те же префиксы, зашитые в сгенерированный
// C), тем же принципом, что и internal/builtins/internal/rtmsg (не
// копировать список префиксов в каждый backend по отдельности).
package dbscheme

import "strings"

// Kind - распознанная СУБД по схеме строки подключения.
type Kind int

const (
	Unknown Kind = iota
	SQLite
	Postgres
	MySQL
)

// Префиксы схемы - те же строковые константы, что видит и рантайм (см.
// package doc), и cbackend (генерирует те же литералы в C, см.
// internal/backend/cbackend/db.go, dbConnectFuncCode).
const (
	SQLitePrefix      = "sqlite:"
	PostgresPrefix    = "postgres://"
	PostgresAltPrefix = "postgresql://"
	MySQLPrefix       = "mysql://"
)

// Detect определяет СУБД по префиксу строки подключения conn - Unknown,
// если ни одна из трёх известных схем не совпала (аварийное завершение в
// рантайме - rtmsg.DBUnsupportedScheme, не здесь).
func Detect(conn string) Kind {
	switch {
	case strings.HasPrefix(conn, SQLitePrefix):
		return SQLite
	case strings.HasPrefix(conn, PostgresPrefix), strings.HasPrefix(conn, PostgresAltPrefix):
		return Postgres
	case strings.HasPrefix(conn, MySQLPrefix):
		return MySQL
	default:
		return Unknown
	}
}

// Set - какие из трёх СУБД реально встречены в программе (по literal-
// аргументам "подключиться_к_бд(...)", см. internal/semantic) - Dynamic
// == true, если хотя бы один вызов передаёт НЕ строковый литерал
// (переменную/выражение) - тогда какая именно схема будет выбрана, решится
// только в рантайме, поэтому все три считаются потенциально нужными (тот
// же безопасный откат, что был безусловным поведением ДО этой
// классификации).
type Set struct {
	SQLite, Postgres, MySQL bool
	Dynamic                 bool
}

// Mark взводит соответствующий флаг Set по результату Detect(conn).
func (s *Set) Mark(conn string) {
	switch Detect(conn) {
	case SQLite:
		s.SQLite = true
	case Postgres:
		s.Postgres = true
	case MySQL:
		s.MySQL = true
	}
}

// NeedsSQLite/NeedsPostgres/NeedsMySQL - нужен ли конкретный драйвер С
// УЧЁТОМ Dynamic (см. package doc) - используется и cbackend (какой блок
// C-кода эмитировать), и toolchain (какую библиотеку линковать).
func (s Set) NeedsSQLite() bool   { return s.SQLite || s.Dynamic }
func (s Set) NeedsPostgres() bool { return s.Postgres || s.Dynamic }
func (s Set) NeedsMySQL() bool    { return s.MySQL || s.Dynamic }
