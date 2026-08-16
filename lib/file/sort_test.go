package file

import (
	"reflect"
	"sync"
	"testing"
)

func TestSortClientByKeyUsesWhitelistAndStableFallback(t *testing.T) {
	var clients sync.Map
	clients.Store(2, &Client{Id: 2, Flow: &Flow{InletFlow: 20}})
	clients.Store(1, &Client{Id: 1, Flow: &Flow{InletFlow: 10}})

	if got := sortClientByKey(&clients, "InletFlow", "asc"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("unexpected ascending order: %v", got)
	}
	if got := sortClientByKey(&clients, "InletFlow", "desc"); !reflect.DeepEqual(got, []int{2, 1}) {
		t.Fatalf("unexpected descending order: %v", got)
	}
	if got := sortClientByKey(&clients, "NotAField", "asc"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("invalid sort key must fall back to client id order: %v", got)
	}
}
