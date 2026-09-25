package builtins

import "github.com/IlyaFeoktistov/Rech/internal/rtmsg"

// FuncThrows - встроенные исключения и паники, которые бросает встроенная
// функция (и метод-хендл, который к ней сводится: "бд.выполнить" -
// "выполнить"). Паника от неверного аргумента - корневая rtmsg.PanicEntity.
// Нет записи - функция ничего не бросает. Потребитель - LSP (hover
// "выбрасывает"); сами backend'ы бросают то же через rtmsg.Error*/Panic*.
var FuncThrows = map[string][]string{
	"прочитать_строку":      {rtmsg.ErrorFile},
	"прочитать_целое":       {rtmsg.ErrorFile, rtmsg.ErrorParse},
	"прочитать_число":       {rtmsg.ErrorFile, rtmsg.ErrorParse},
	"открыть":               {rtmsg.ErrorFile},
	"открыть_для_записи":    {rtmsg.ErrorFile},
	"открыть_для_дозаписи":  {rtmsg.ErrorFile},
	"прочитать":             {rtmsg.ErrorFile, rtmsg.PanicEntity},
	"записать":              {rtmsg.ErrorFile},
	"прочитать_файл":        {rtmsg.ErrorFile},
	"записать_в_файл":       {rtmsg.ErrorFile},
	"дописать_в_файл":       {rtmsg.ErrorFile},
	"текущий_каталог":       {rtmsg.ErrorFile},
	"прочитать_байты":       {rtmsg.ErrorFile, rtmsg.PanicEntity},
	"записать_байты":        {rtmsg.ErrorFile},
	"прочитать_файл_байты":  {rtmsg.ErrorFile},
	"записать_в_файл_байты": {rtmsg.ErrorFile},

	"сетевой_узел":               {rtmsg.ErrorNetwork, rtmsg.PanicEntity},
	"отправить":                  {rtmsg.ErrorNetwork},
	"получить":                   {rtmsg.ErrorNetwork, rtmsg.PanicEntity},
	"принять":                    {rtmsg.ErrorNetwork},
	"получить_от":                {rtmsg.ErrorNetwork, rtmsg.PanicEntity},
	"отправить_в":                {rtmsg.ErrorNetwork},
	"слушать_tcp_защищённо":      {rtmsg.ErrorNetwork},
	"запрос":                     {rtmsg.ErrorNetwork},
	"принять_http":               {rtmsg.ErrorNetwork},
	"отправить_http_ответ":       {rtmsg.ErrorNetwork},
	"принять_http_байты":         {rtmsg.ErrorNetwork},
	"отправить_http_ответ_байты": {rtmsg.ErrorNetwork},
	"http2_принять_сессию":       {rtmsg.ErrorNetwork},
	"новая_http2_сессия":         {rtmsg.ErrorNetwork},
	"http2_принять_поток":        {rtmsg.ErrorNetwork},
	"http2_открыть_поток":        {rtmsg.ErrorNetwork},
	"http2_прочитать_данные":     {rtmsg.ErrorNetwork, rtmsg.PanicEntity},
	"http2_прочитать_всё":        {rtmsg.ErrorNetwork},
	"http2_отправить_заголовки":  {rtmsg.ErrorNetwork},
	"http2_отправить_данные":     {rtmsg.ErrorNetwork},
	"http2_отправить_трейлер":    {rtmsg.ErrorNetwork, rtmsg.PanicEntity},
	"http2_завершить_поток":      {rtmsg.ErrorNetwork},
	"grpc_открыть_поток":         {rtmsg.ErrorNetwork},
	"grpc_начать_ответ":          {rtmsg.ErrorNetwork},
	"grpc_прочитать_сообщение":   {rtmsg.ErrorNetwork},
	"grpc_отправить_сообщение":   {rtmsg.ErrorNetwork},
	"grpc_завершить":             {rtmsg.ErrorNetwork},
	"слушать_grpcs":              {rtmsg.ErrorNetwork},
	"новая_grpcs_сессия":         {rtmsg.ErrorNetwork},

	"подключиться_к_бд": {rtmsg.ErrorDB},
	"запросить":         {rtmsg.ErrorDB, rtmsg.PanicEntity},
	"выполнить":         {rtmsg.ErrorDB, rtmsg.PanicEntity},
	"начать_транзакцию": {rtmsg.ErrorDB},

	"целое_из_hex":             {rtmsg.ErrorParse},
	"целое_из_oct":             {rtmsg.ErrorParse},
	"целое_из_bin":             {rtmsg.ErrorParse},
	"декодировать_base64url":   {rtmsg.ErrorParse},
	"rgbцвет_из_hex":           {rtmsg.ErrorParse},
	"разобрать_multipart":      {rtmsg.ErrorParse},
	"декодировать_изображение": {rtmsg.ErrorParse, rtmsg.PanicEntity},
	"закодировать_изображение": {rtmsg.PanicEntity},

	"зашифровать":                  {rtmsg.ErrorCrypto},
	"расшифровать":                 {rtmsg.ErrorParse, rtmsg.ErrorCrypto},
	"сгенерировать_приватный_ключ": {rtmsg.ErrorCrypto, rtmsg.PanicEntity},
	"получить_публичный_ключ":      {rtmsg.ErrorParse},
	"подписать":                    {rtmsg.ErrorParse, rtmsg.ErrorCrypto},
	"проверить_подпись":            {rtmsg.ErrorParse},
	"хеш":                          {rtmsg.PanicEntity},

	"запустить_процесс":            {rtmsg.ErrorProcess},
	"дождаться_процесса":           {rtmsg.ErrorProcess},
	"выполнить_команду":            {rtmsg.ErrorProcess},
	"подключиться_к_процессу":      {rtmsg.ErrorProcess},
	"отключиться_от_процесса":      {rtmsg.ErrorProcess},
	"прочитать_память":             {rtmsg.ErrorProcess},
	"записать_память":              {rtmsg.ErrorProcess},
	"найти_байты":                  {rtmsg.ErrorProcess, rtmsg.PanicEntity},
	"регистры_процесса":            {rtmsg.ErrorProcess},
	"установить_регистры_процесса": {rtmsg.ErrorProcess},
	"установить_точку_останова":    {rtmsg.ErrorProcess},
	"снять_точку_останова":         {rtmsg.PanicEntity},
	"возобновить_процесс":          {rtmsg.ErrorProcess},
	"отправить_сигнал":             {rtmsg.ErrorProcess},
	"получен_сигнал":               {rtmsg.PanicEntity},

	"подстрока":       {rtmsg.PanicEntity},
	"аргумент":        {rtmsg.PanicEntity},
	"случайное_целое": {rtmsg.PanicEntity},
	"разбить":         {rtmsg.PanicEntity},
	"регулярное":      {rtmsg.PanicEntity},
}

// CollectionMethodThrows - паники методов встроенных generic-коллекций:
// ключ - база коллекции ("Список" из "Список<Целое>"), затем имя метода.
var CollectionMethodThrows = map[string]map[string][]string{
	"Список": {
		"получить":   {rtmsg.PanicOutOfRange},
		"установить": {rtmsg.PanicOutOfRange},
		"удалить":    {rtmsg.PanicOutOfRange},
		"первый":     {rtmsg.PanicOutOfRange},
		"последний":  {rtmsg.PanicOutOfRange},
	},
	"Стопка": {
		"снять": {rtmsg.PanicEmptyCollection},
		"верх":  {rtmsg.PanicEmptyCollection},
	},
	"Очередь": {
		"извлечь": {rtmsg.PanicEmptyCollection},
		"первый":  {rtmsg.PanicEmptyCollection},
	},
	"Словарь": {
		"получить": {rtmsg.PanicKeyNotFound},
	},
}
