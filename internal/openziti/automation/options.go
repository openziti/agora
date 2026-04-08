package automation

import "time"

type BaseOptions struct {
	Name    string
	Tags    *Tags
	Timeout time.Duration
}

func (bo *BaseOptions) timeout(defaultTimeout time.Duration) time.Duration {
	if bo != nil && bo.Timeout > 0 {
		return bo.Timeout
	}
	return defaultTimeout
}

func (bo *BaseOptions) tags() *Tags {
	if bo != nil && bo.Tags != nil {
		return bo.Tags
	}
	return NewTags()
}
