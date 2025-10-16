package bimap

import (
	"reflect"
	"sort"
	"testing"
)

func TestOwnerMap(t *testing.T) {

	var om OwnerMap[string, int]

	if !om.Add("sam", 123) {
		t.Errorf("fail")
	}
	if om.Add("", 123) {
		t.Errorf("fail")
	}
	if !om.Add("sam", 124) {
		t.Errorf("fail")
	}
	if !om.Add("", 125) {
		t.Errorf("fail")
	}

	if o, s := om.Size(); o != 2 || s != 3 {
		t.Error("bad size")
	}

	if owner, _ := om.Owner(124); owner != "sam" {
		t.Error("fail")
	}
	if owner, ok := om.Release(125); owner != "" || !ok {
		t.Error("couldn't release")
	}

	clear := om.Clear("sam")
	sort.Ints(clear)
	if !reflect.DeepEqual(clear, []int{123, 124}) {
		t.Errorf("bad clear: %+v", clear)
	}

	clear = om.Clear("")
	if len(clear) != 0 {
		t.Errorf("bad clear")
	}
}
