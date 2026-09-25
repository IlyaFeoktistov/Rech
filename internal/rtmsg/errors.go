package rtmsg

// Встроенные исключения рантайма - ожидаемые ситуации (файл не открылся,
// соединение оборвалось, ввод не разобрался), в отличие от паник
// (PanicKinds) - ошибок программиста. Все наследуют "Исключение"; одни и те
// же имена у семантики (регистрирует сущности) и обоих backend'ов (бросают
// исключения этих типов).
const (
	ErrorFile    = "ОшибкаФайла"
	ErrorNetwork = "ОшибкаСети"
	ErrorDB      = "ОшибкаБД"
	ErrorParse   = "ОшибкаРазбора"
	ErrorProcess = "ОшибкаПроцесса"
	ErrorCrypto  = "ОшибкаКриптографии"
)

// ErrorKinds - все встроенные исключения; индекс - код вида в C-рантайме
// (rech_error_as, ErrorKindMacros).
var ErrorKinds = []string{
	ErrorFile,
	ErrorNetwork,
	ErrorDB,
	ErrorParse,
	ErrorProcess,
	ErrorCrypto,
}

// ErrorKindMacros - имя C-макроса кода вида для каждого встроенного
// исключения - C-рантайм пишет rech_error_as(RECH_ERR_FILE, сообщение).
var ErrorKindMacros = map[string]string{
	ErrorFile:    "RECH_ERR_FILE",
	ErrorNetwork: "RECH_ERR_NETWORK",
	ErrorDB:      "RECH_ERR_DB",
	ErrorParse:   "RECH_ERR_PARSE",
	ErrorProcess: "RECH_ERR_PROCESS",
	ErrorCrypto:  "RECH_ERR_CRYPTO",
}
