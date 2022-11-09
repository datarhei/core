package value

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// must directory

type MustDir string

func NewMustDir(p *string, val string) *MustDir {
	*p = val

	return (*MustDir)(p)
}

func (u *MustDir) Set(val string) error {
	*u = MustDir(val)
	return nil
}

func (u *MustDir) String() string {
	return string(*u)
}

func (u *MustDir) Validate() error {
	val := string(*u)

	if len(strings.TrimSpace(val)) == 0 {
		return fmt.Errorf("path name must not be empty")
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.IsDir() {
		return fmt.Errorf("%s is not a directory", val)
	}

	return nil
}

func (u *MustDir) IsEmpty() bool {
	return len(string(*u)) == 0
}

// directory

type Dir string

func NewDir(p *string, val string) *Dir {
	*p = val

	return (*Dir)(p)
}

func (u *Dir) Set(val string) error {
	*u = Dir(val)
	return nil
}

func (u *Dir) String() string {
	return string(*u)
}

func (u *Dir) Validate() error {
	val := string(*u)

	if len(strings.TrimSpace(val)) == 0 {
		return nil
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.IsDir() {
		return fmt.Errorf("%s is not a directory", val)
	}

	return nil
}

func (u *Dir) IsEmpty() bool {
	return len(string(*u)) == 0
}

// executable

type Exec string

func NewExec(p *string, val string) *Exec {
	*p = val

	return (*Exec)(p)
}

func (u *Exec) Set(val string) error {
	*u = Exec(val)
	return nil
}

func (u *Exec) String() string {
	return string(*u)
}

func (u *Exec) Validate() error {
	val := string(*u)

	_, err := exec.LookPath(val)
	if err != nil {
		return fmt.Errorf("%s not found or is not executable", val)
	}

	return nil
}

func (u *Exec) IsEmpty() bool {
	return len(string(*u)) == 0
}

// regular file

type File string

func NewFile(p *string, val string) *File {
	*p = val

	return (*File)(p)
}

func (u *File) Set(val string) error {
	*u = File(val)
	return nil
}

func (u *File) String() string {
	return string(*u)
}

func (u *File) Validate() error {
	val := string(*u)

	if len(val) == 0 {
		return nil
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", val)
	}

	return nil
}

func (u *File) IsEmpty() bool {
	return len(string(*u)) == 0
}

// absolute path

type AbsolutePath string

func NewAbsolutePath(p *string, val string) *AbsolutePath {
	*p = filepath.Clean(val)

	return (*AbsolutePath)(p)
}

func (s *AbsolutePath) Set(val string) error {
	*s = AbsolutePath(filepath.Clean(val))
	return nil
}

func (s *AbsolutePath) String() string {
	return string(*s)
}

func (s *AbsolutePath) Validate() error {
	path := string(*s)

	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s is not an absolute path", path)
	}

	return nil
}

func (s *AbsolutePath) IsEmpty() bool {
	return len(string(*s)) == 0
}
