package file

import (
	"reflect"
	"sort"
	"sync"
)

// A data structure to hold a key/value pair.
type Pair struct {
	key        string //sort key
	cId        int
	order      string
	clientFlow *Flow
}

// A slice of Pairs that implements sort.Interface to sort by Value.
type PairList []*Pair

func (p PairList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p PairList) Len() int      { return len(p) }
func (p PairList) Less(i, j int) bool {
	left := reflect.ValueOf(p[i].clientFlow).Elem().FieldByName(p[i].key).Int()
	right := reflect.ValueOf(p[j].clientFlow).Elem().FieldByName(p[j].key).Int()
	if p[i].order == "desc" {
		return left < right
	}
	return left > right
}

// A function to turn a map into a PairList, then sort and return it.
func sortClientByKey(m *sync.Map, sortKey, order string) (res []int) {
	p := make(PairList, 0)
	m.Range(func(key, value interface{}) bool {
		p = append(p, &Pair{sortKey, value.(*Client).Id, order, value.(*Client).Flow})
		return true
	})
	sort.Sort(p)
	for _, v := range p {
		res = append(res, v.cId)
	}
	return
}
