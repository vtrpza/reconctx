//go:build !linux

package workspace

type Root struct{}

func Open(string) (*Root, error)                     { return nil, ErrUnsupported }
func (*Root) Close() error                           { return nil }
func (*Root) MkdirAll(string) error                  { return ErrUnsupported }
func (*Root) CreateRunDir(string) error              { return ErrUnsupported }
func (*Root) ReadFile(string) ([]byte, error)        { return nil, ErrUnsupported }
func (*Root) atomicWrite(string, []byte, bool) error { return ErrUnsupported }
