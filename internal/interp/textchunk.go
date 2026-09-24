package interp

import (
	"bufio"
	"bytes"
	"io"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/textutf8"
)

// pendingBytes - отложенные байты по дескриптору (файл/сокет): хвост
// символа, оборванный концом куска при "прочитать(файл, n)"/"получить(
// сокет, n)", и то, что построчный перебор уже взял в буфер, но не
// отдал. Следующее чтение того же дескриптора любым способом начинается
// с них - тот же принцип, что у cbackend (rech_pend_*, strutil.TextCode).
type pendingBytes struct {
	mu sync.Mutex
	m  map[any][]byte
}

func (p *pendingBytes) take(h any) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.m[h]
	delete(p.m, h)
	return b
}

func (p *pendingBytes) put(h any, b []byte) {
	if len(b) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[any][]byte{}
	}
	p.m[h] = append(append([]byte(nil), b...), p.m[h]...)
}

// reader - r с отложенными байтами h впереди.
func (p *pendingBytes) reader(h any, r io.Reader) io.Reader {
	if b := p.take(h); len(b) > 0 {
		return io.MultiReader(bytes.NewReader(b), r)
	}
	return r
}

// readTextChunk - до max байт h (r - сам дескриптор) как Строка, кусок
// кончается целым символом: оборванный хвост откладывается; если первый
// же символ длиннее max - дочитывается. full - читать до max или конца
// (файл, как fread), иначе одним чтением (сокет, как recv).
func (in *Interp) readTextChunk(h any, r io.Reader, max int, full bool) (string, error) {
	buf := make([]byte, max, max+3)
	n := copy(buf, in.pending.take(h))
	var err error
	if n < max {
		var k int
		if full {
			k, err = io.ReadFull(r, buf[n:])
			if err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
		} else {
			k, err = r.Read(buf[n:])
			if k == 0 && err == nil {
				err = io.EOF
			}
		}
		n += k
	}
	buf = buf[:n]
	if err != nil && err != io.EOF {
		return "", err
	}
	if tail := textutf8.IncompleteTail(buf); tail > 0 && err != io.EOF {
		if tail == n {
			one := make([]byte, 1)
			for textutf8.IncompleteTail(buf) == len(buf) && len(buf) < 4 {
				if k, rerr := r.Read(one); k == 0 || rerr != nil {
					break
				}
				buf = append(buf, one[0])
			}
		} else {
			in.pending.put(h, buf[n-tail:])
			buf = buf[:n-tail]
		}
	}
	return string(buf), nil
}

// fileLineReader - построчный перебор файла f; release возвращает то, что
// буфер уже прочитал, но цикл не отдал, в отложенные байты f.
func (in *Interp) fileLineReader(f any, r io.Reader) (reader *bufio.Reader, release func()) {
	reader = bufio.NewReader(in.pending.reader(f, r))
	return reader, func() {
		if k := reader.Buffered(); k > 0 {
			rest, _ := reader.Peek(k)
			in.pending.put(f, rest)
		}
	}
}
