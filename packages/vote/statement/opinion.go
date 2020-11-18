package statement

import (
	"sort"

	"github.com/iotaledger/goshimmer/packages/vote"
)

type Opinion struct {
	Value vote.Opinion
	Round uint8
}

type Opinions []Opinion

func (o Opinions) Len() int           { return len(o) }
func (o Opinions) Less(i, j int) bool { return o[i].Round < o[j].Round }
func (o Opinions) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func (o Opinions) Last() Opinion {
	sort.Sort(o)
	return o[len(o)-1]
}

func (o Opinions) Finalized(l int) bool {
	if len(o) < l {
		return false
	}
	sort.Sort(o)
	target := o[len(o)-1].Value
	for i := len(o) - 2; i >= l; l-- {
		if o[i].Value != target {
			return false
		}
	}
	return true
}