package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/bodgit/sevenzip"
)

type TarArchive struct {
	r *tar.Reader
}

type ZipArchive struct {
	r   *zip.Reader
	idx int
}

type SevenZipArchive struct {
	r   *sevenzip.Reader
	idx int
}

func tarft(typ byte) FileType {
	switch typ {
	case tar.TypeReg:
		return TypeNormal
	case tar.TypeDir:
		return TypeDir
	case tar.TypeLink:
		return TypeLink
	case tar.TypeSymlink:
		return TypeSymlink
	default:
		return TypeOther
	}
}

func NewTarArchive(data []byte, decompress DecompFn) (Archive, error) {
	r := bytes.NewReader(data)
	dr, err := decompress(r)
	if err != nil {
		return nil, err
	}
	return &TarArchive{r: tar.NewReader(dr)}, nil
}

func (t *TarArchive) Next() (File, error) {
	for {
		hdr, err := t.r.Next()
		if err != nil {
			return File{}, err
		}
		ft := tarft(hdr.Typeflag)
		if ft != TypeOther {
			name, err := safeArchiveRelativePath(hdr.Name)
			if err != nil {
				return File{}, err
			}
			linkName, err := safeArchiveLinkName(hdr.Linkname, ft)
			if err != nil {
				return File{}, err
			}
			return File{Name: name, LinkName: linkName, Mode: fs.FileMode(hdr.Mode), Type: ft}, nil
		}
	}
}

func (t *TarArchive) ReadAll() ([]byte, error) {
	return io.ReadAll(t.r)
}

func (t *TarArchive) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, t.r)
}

func NewZipArchive(data []byte, d DecompFn) (Archive, error) {
	r := bytes.NewReader(data)
	zr, err := zip.NewReader(r, int64(len(data)))
	return &ZipArchive{r: zr, idx: -1}, err
}

func NewSevenZipArchive(data []byte, d DecompFn) (Archive, error) {
	r := bytes.NewReader(data)
	szr, err := sevenzip.NewReader(r, int64(len(data)))
	return &SevenZipArchive{r: szr, idx: -1}, err
}

func (z *ZipArchive) Next() (File, error) {
	z.idx++
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return File{}, io.EOF
	}
	f := z.r.File[z.idx]
	typ := TypeNormal
	if strings.HasSuffix(f.Name, "/") {
		typ = TypeDir
	}
	name, err := safeArchiveRelativePath(f.Name)
	if err != nil {
		return File{}, err
	}
	return File{Name: name, Mode: f.Mode(), Type: typ}, nil
}

func (z *ZipArchive) ReadAll() ([]byte, error) {
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return nil, io.EOF
	}
	f := z.r.File[z.idx]
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("zip extract: %w", err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (z *ZipArchive) WriteTo(w io.Writer) (int64, error) {
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return 0, io.EOF
	}
	f := z.r.File[z.idx]
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("zip extract: %w", err)
	}
	defer rc.Close()
	return io.Copy(w, rc)
}

func (z *SevenZipArchive) Next() (File, error) {
	z.idx++
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return File{}, io.EOF
	}
	f := z.r.File[z.idx]
	mode := f.Mode()
	typ := TypeNormal
	if mode.IsDir() {
		typ = TypeDir
	}
	name, err := safeArchiveRelativePath(f.Name)
	if err != nil {
		return File{}, err
	}
	return File{Name: name, Mode: mode, Type: typ}, nil
}

func (z *SevenZipArchive) ReadAll() ([]byte, error) {
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return nil, io.EOF
	}
	f := z.r.File[z.idx]
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("7z extract: %w", err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (z *SevenZipArchive) WriteTo(w io.Writer) (int64, error) {
	if z.idx < 0 || z.idx >= len(z.r.File) {
		return 0, io.EOF
	}
	f := z.r.File[z.idx]
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("7z extract: %w", err)
	}
	defer rc.Close()
	return io.Copy(w, rc)
}
