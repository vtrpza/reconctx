//go:build !linux

package workspace

func (*Root) PublishTree(string, map[string][]byte) error { return ErrUnsupported }
func (*Root) ListTree(string) ([]string, error)           { return nil, ErrUnsupported }
