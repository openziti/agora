package automation

import "time"

type FilterOptions struct {
	Filter  string
	Limit   int64
	Offset  int64
	Timeout time.Duration
}

func (fo *FilterOptions) timeout(defaultTimeout time.Duration) time.Duration {
	if fo != nil && fo.Timeout > 0 {
		return fo.Timeout
	}
	return defaultTimeout
}

func (fo *FilterOptions) limit() int64 {
	if fo != nil && fo.Limit > 0 {
		return fo.Limit
	}
	return 100
}

func (fo *FilterOptions) offset() int64 {
	if fo != nil {
		return fo.Offset
	}
	return 0
}

func filterValue(fo *FilterOptions) string {
	if fo != nil {
		return fo.Filter
	}
	return ""
}
