package onboarding

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
)

type Key int

const (
	KeyUnknown Key = iota
	KeyUp
	KeyDown
	KeySpace
	KeyEnter
	KeyEscape
	KeyAll
	KeyNone
	KeyQuit
	KeyCancel
)

type KeyReader interface{ ReadKey() (Key, error) }
type Selector struct {
	Reader KeyReader
	Out    io.Writer
}

// Select keeps catalog order, mutates only the in-memory selection, and never installs.
func (s Selector) Select(items []Item, defaults map[string]bool) (map[string]bool, error) {
	selected := map[string]bool{}
	for id, v := range defaults {
		selected[id] = v
	}
	if _, err := fmt.Fprintln(s.Out, i18n.T("onboarding.select.prompt")); err != nil {
		return nil, err
	}
	pos := 0
	for {
		if err := s.render(items, selected, pos); err != nil {
			return nil, err
		}
		key, err := s.Reader.ReadKey()
		if err != nil {
			return nil, err
		}
		switch key {
		case KeyUp:
			if pos > 0 {
				pos--
			}
		case KeyDown:
			if pos < len(items)-1 {
				pos++
			}
		case KeySpace:
			if len(items) > 0 {
				selected[items[pos].ID] = !selected[items[pos].ID]
			}
		case KeyAll:
			for _, it := range items {
				selected[it.ID] = true
			}
		case KeyNone:
			// Clear the selection but stay in the list, like "a" selects all and stays.
			for id := range selected {
				selected[id] = false
			}
		case KeyEscape, KeyQuit:
			for id := range selected {
				selected[id] = false
			}
			return selected, nil
		case KeyEnter:
			return selected, nil
		case KeyCancel:
			return selected, errors.New(i18n.T("onboarding.select.cancelled"))
		}
	}
}
func (s Selector) render(items []Item, selected map[string]bool, pos int) error {
	if _, err := fmt.Fprint(s.Out, "\x1b[2J\x1b[H"+i18n.T("onboarding.select.prompt")+"\n"); err != nil {
		return err
	}
	for i, it := range items {
		mark := " "
		if selected[it.ID] {
			mark = "x"
		}
		cursor := " "
		if i == pos {
			cursor = ">"
		}
		if _, err := fmt.Fprintf(s.Out, "%s [%s] %-16s (%s)\n", cursor, mark, it.Name, methodLabel(it)); err != nil {
			return err
		}
	}
	return nil
}
func methodLabel(i Item) string {
	m := i.Methods[currentOS()]
	if len(m) == 0 {
		return i18n.T("onboarding.select.manual")
	}
	for _, x := range m {
		if x.Auto {
			return x.Label
		}
	}
	return m[0].Label
}

type byteKeyReader struct{ r *bufio.Reader }

func NewByteKeyReader(r io.Reader) KeyReader { return &byteKeyReader{bufio.NewReader(r)} }
func (r *byteKeyReader) ReadKey() (Key, error) {
	b, e := r.r.ReadByte()
	if e != nil {
		return KeyUnknown, e
	}
	switch b {
	case ' ':
		return KeySpace, nil
	case '\r', '\n':
		return KeyEnter, nil
	case 'k', 'K':
		return KeyUp, nil
	case 'j', 'J':
		return KeyDown, nil
	case 'a', 'A':
		return KeyAll, nil
	case 'n', 'N':
		return KeyNone, nil
	case 'q', 'Q':
		return KeyQuit, nil
	case 3:
		return KeyCancel, nil
	case 27:
		if r.r.Buffered() == 0 {
			return KeyEscape, nil
		}
		next, e := r.r.ReadByte()
		if e != nil {
			return KeyEscape, nil
		}
		if next != '[' {
			return KeyEscape, nil
		}
		arrow, e := r.r.ReadByte()
		if e != nil {
			return KeyEscape, nil
		}
		if arrow == 'A' {
			return KeyUp, nil
		}
		if arrow == 'B' {
			return KeyDown, nil
		}
		return KeyUnknown, nil
	}
	return KeyUnknown, nil
}
func SelectNumbers(in io.Reader, out io.Writer, items []Item, defaults map[string]bool) (map[string]bool, error) {
	return selectNumbers(bufio.NewReader(in), out, items, defaults)
}

func selectNumbers(reader *bufio.Reader, out io.Writer, items []Item, defaults map[string]bool) (map[string]bool, error) {
	selected := map[string]bool{}
	for id, v := range defaults {
		selected[id] = v
	}
	for i, it := range items {
		mark := " "
		if selected[it.ID] {
			mark = "*"
		}
		fmt.Fprintf(out, "%d) [%s] %s (%s)\n", i+1, mark, it.Name, methodLabel(it))
	}
	fmt.Fprint(out, i18n.T("onboarding.select.numbered_prompt"))
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return selected, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return selected, nil
	}
	if strings.EqualFold(line, "n") {
		for id := range selected {
			selected[id] = false
		}
		return selected, nil
	}
	for id := range selected {
		selected[id] = false
	}
	for _, part := range strings.Fields(line) {
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(part) {
			return nil, fmt.Errorf("%s", i18n.T("onboarding.select.invalid_number", part))
		}
		var n int
		if _, e := fmt.Sscanf(part, "%d", &n); e != nil || n < 1 || n > len(items) {
			return nil, fmt.Errorf("%s", i18n.T("onboarding.select.out_of_range", part))
		}
		selected[items[n-1].ID] = true
	}
	return selected, nil
}
