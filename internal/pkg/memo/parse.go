package memo

type Parse struct {
	memoStr string
}

func NewParse(memoStr string) *Parse {
	return &Parse{
		memoStr: memoStr,
	}
}
