package file

import (
	"sort"
	"sync"
)

// A data structure to hold a key/value pair.
type Pair struct {
	key   string //sort key
	cId   int
	order string
	value int64
}

// A slice of Pairs that implements sort.Interface to sort by Value.
type PairList []*Pair

func (p PairList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p PairList) Len() int      { return len(p) }
func (p PairList) Less(i, j int) bool {
	left := p[i].value
	right := p[j].value
	if p[i].order == "desc" {
		return left > right
	}
	return left < right
}

// A function to turn a map into a PairList, then sort and return it.
func sortClientByKey(m *sync.Map, sortKey, order string) (res []int) {
	if sortKey != "InletFlow" && sortKey != "ExportFlow" && sortKey != "FlowLimit" {
		return GetMapKeys(m, false, "", "")
	}
	p := make(PairList, 0)
	m.Range(func(key, value interface{}) bool {
		client := value.(*Client)
		inlet, export, limit := client.Flow.SnapshotWithLimit()
		var sortValue int64
		switch sortKey {
		case "InletFlow":
			sortValue = inlet
		case "ExportFlow":
			sortValue = export
		case "FlowLimit":
			sortValue = limit
		}
		p = append(p, &Pair{key: sortKey, cId: client.Id, order: order, value: sortValue})
		return true
	})
	sort.Sort(p)
	for _, v := range p {
		res = append(res, v.cId)
	}
	return
}
