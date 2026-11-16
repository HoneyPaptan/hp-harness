package host

type NameLister interface {
	ListNames(path string) ([]Entry, error)
}

func ReadNames(h Host, path string) ([]Entry, error) {
	if l, ok := h.(NameLister); ok {
		return l.ListNames(path)
	}
	return h.ReadDir(path)
}
