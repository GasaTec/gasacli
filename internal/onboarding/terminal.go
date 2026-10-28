package onboarding

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

type TerminalOpener func(in, out *os.File) (KeyReader, func(), error)

var ErrTerminalUnavailable = errors.New("raw terminal unavailable")

func Interactive(in, out *os.File) bool {
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}
func rawKeys(in, out *os.File) (KeyReader, func(), error) {
	restore, err := EnableRawTerminal(in, out)
	if err != nil {
		return nil, nil, err
	}
	return NewByteKeyReader(in), restore, nil
}

func EnableRawTerminal(in, out *os.File) (func(), error) {
	restoreVT, err := enableVT(outFD(out))
	if err != nil {
		return nil, err
	}
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		restoreVT()
		return nil, err
	}
	return func() { _ = term.Restore(int(in.Fd()), state); restoreVT() }, nil
}

func SelectRaw(items []Item, defaults map[string]bool, in, out *os.File, writer io.Writer, open TerminalOpener) (selected map[string]bool, err error) {
	reader, restore, err := open(in, out)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTerminalUnavailable, err)
	}
	defer restore()
	return (Selector{Reader: reader, Out: writer}).Select(items, defaults)
}
func outFD(f *os.File) uintptr { return uintptr(f.Fd()) }
