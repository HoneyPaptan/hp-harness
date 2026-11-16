package host

type HeadReader interface {
	ReadHead(path string, n int) ([]byte, error)
}

func ReadHead(h Host, path string, n int) ([]byte, error) {
	if r, ok := h.(HeadReader); ok {
		return r.ReadHead(path, n)
	}
	data, err := h.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data[:min(len(data), n)], nil
}
