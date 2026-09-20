package rtmsg

import "testing"

func TestHTTPReasonPhraseKnownCode(t *testing.T) {
	if got := HTTPReasonPhrase(404); got != "Not Found" {
		t.Fatalf("HTTPReasonPhrase(404) = %q, ожидалось \"Not Found\"", got)
	}
	if got := HTTPReasonPhrase(200); got != "OK" {
		t.Fatalf("HTTPReasonPhrase(200) = %q, ожидалось \"OK\"", got)
	}
}

// TestHTTPReasonPhraseUnknownCodeFallsBackToClass - код вне курируемого
// списка - фраза по классу первой цифры, не "OK" безусловно и не пустая
// строка.
func TestHTTPReasonPhraseUnknownCodeFallsBackToClass(t *testing.T) {
	if got := HTTPReasonPhrase(499); got != "Client Error" {
		t.Fatalf("HTTPReasonPhrase(499) = %q, ожидалось \"Client Error\"", got)
	}
	if got := HTTPReasonPhrase(999); got != "Unknown" {
		t.Fatalf("HTTPReasonPhrase(999) = %q, ожидалось \"Unknown\"", got)
	}
}

// TestHTTPReasonPhrasesForCSortedByCode - cbackend полагается на
// стабильный порядок для детерминированной генерации C-кода.
func TestHTTPReasonPhrasesForCSortedByCode(t *testing.T) {
	entries := HTTPReasonPhrasesForC()
	if len(entries) == 0 {
		t.Fatal("ожидался непустой список")
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Code >= entries[i].Code {
			t.Fatalf("не отсортировано по коду: %d затем %d", entries[i-1].Code, entries[i].Code)
		}
	}
}
