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

type FilterChooser struct {
	expr        string
	include     []Chooser
	exclude     []Chooser
	implicitAll bool
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
		part := strings.TrimSpace(expr)
		if strings.HasPrefix(part, "^") {
			return newFilterChooser([]string{part})
		}
		return NewGlobChooser(part)
	}

	hasExclude := false
	rawParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rawParts = append(rawParts, part)
		if strings.HasPrefix(part, "^") {
			hasExclude = true
		}
	}
	if hasExclude {
		return newFilterChooser(rawParts)
	}

	choosers := make([]Chooser, 0, len(parts))
	normalized := make([]string, 0, len(parts))
	for _, part := range rawParts {
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

func newFilterChooser(parts []string) (Chooser, error) {
	filter := &FilterChooser{}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		anti := strings.HasPrefix(part, "^")
		expr := part
		if anti {
			expr = strings.TrimSpace(strings.TrimPrefix(part, "^"))
			if expr == "" {
				return nil, fmt.Errorf("empty file exclude expression")
			}
		}
		ch, err := NewGlobChooser(expr)
		if err != nil {
			return nil, err
		}
		if anti {
			filter.exclude = append(filter.exclude, ch)
		} else {
			filter.include = append(filter.include, ch)
		}
		normalized = append(normalized, part)
	}
	if len(filter.include) == 0 {
		all, err := NewGlobChooser("*")
		if err != nil {
			return nil, err
		}
		filter.include = append(filter.include, all)
		filter.implicitAll = true
	}
	if len(filter.include) == 0 && len(filter.exclude) == 0 {
		return nil, fmt.Errorf("empty file chooser expression")
	}
	filter.expr = strings.Join(normalized, ",")
	return filter, nil
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

func (f *FilterChooser) Choose(name string, dir bool, mode fs.FileMode) (bool, bool) {
	for _, chooser := range f.exclude {
		_, possible := chooser.Choose(name, dir, mode)
		if possible {
			return false, false
		}
	}
	for _, chooser := range f.include {
		direct, possible := chooser.Choose(name, dir, mode)
		if direct || possible {
			return direct && !f.implicitAll, true
		}
	}
	return false, false
}

func (f *FilterChooser) String() string {
	return fmt.Sprintf("`%s`", f.expr)
}
