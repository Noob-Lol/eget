package install

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
)

type Chooser interface {
	Choose(name string, dir bool, mode fs.FileMode) (direct bool, possible bool)
}

type BinaryChooser struct {
	Tool string
}

type LiteralFileChooser struct {
	File string
}

type GlobChooser struct {
	expr string
	g    glob.Glob
	all  bool
}

type MultiChooser struct {
	expr     string
	choosers []Chooser
}

func NewBinaryChooser(tool string) *BinaryChooser {
	return &BinaryChooser{Tool: tool}
}

func NewGlobChooser(gl string) (*GlobChooser, error) {
	g, err := glob.Compile(gl, '/')
	return &GlobChooser{g: g, expr: gl, all: gl == "*" || gl == "/"}, err
}

func NewFileChooser(expr string) (Chooser, error) {
	parts := strings.Split(expr, ",")
	if len(parts) == 1 {
		return NewGlobChooser(strings.TrimSpace(expr))
	}

	choosers := make([]Chooser, 0, len(parts))
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ch, err := NewGlobChooser(part)
		if err != nil {
			return nil, err
		}
		choosers = append(choosers, ch)
		normalized = append(normalized, part)
	}
	if len(choosers) == 0 {
		return nil, fmt.Errorf("empty file chooser expression")
	}
	if len(choosers) == 1 {
		return choosers[0], nil
	}
	return &MultiChooser{
		expr:     strings.Join(normalized, ","),
		choosers: choosers,
	}, nil
}

func (b *BinaryChooser) Choose(name string, dir bool, mode fs.FileMode) (bool, bool) {
	if dir {
		return false, false
	}
	fmatch := filepath.Base(name) == b.Tool || filepath.Base(name) == b.Tool+".exe" || filepath.Base(name) == b.Tool+".appimage"
	possible := !mode.IsDir() && isExec(name, mode.Perm())
	return fmatch && possible, possible
}

func (b *BinaryChooser) String() string {
	return fmt.Sprintf("exe `%s`", b.Tool)
}

func (l *LiteralFileChooser) Choose(name string, dir bool, mode fs.FileMode) (bool, bool) {
	return false, filepath.Base(name) == filepath.Base(l.File) && strings.HasSuffix(name, l.File)
}

func (l *LiteralFileChooser) String() string {
	return fmt.Sprintf("`%s`", l.File)
}

func (g *GlobChooser) Choose(name string, dir bool, mode fs.FileMode) (bool, bool) {
	if g.all {
		return true, true
	}
	if len(name) > 0 && name[len(name)-1] == '/' {
		name = name[:len(name)-1]
	}
	return false, g.g.Match(filepath.Base(name)) || g.g.Match(name)
}

func (g *GlobChooser) String() string {
	return fmt.Sprintf("`%s`", g.expr)
}

func (m *MultiChooser) Choose(name string, dir bool, mode fs.FileMode) (bool, bool) {
	for _, chooser := range m.choosers {
		direct, possible := chooser.Choose(name, dir, mode)
		if direct || possible {
			return direct, true
		}
	}
	return false, false
}

func (m *MultiChooser) String() string {
	return fmt.Sprintf("`%s`", m.expr)
}
