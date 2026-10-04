package planrun

import "testing"

func TestCloneRowCopiesCategories(t *testing.T) {
	row := TaskRow{Categories: map[string]int{"correctness": 1}}
	cp := cloneRow(row)
	cp.Categories["correctness"] = 9
	if row.Categories["correctness"] != 1 {
		t.Fatalf("cloneRow aliased Categories: original now %v", row.Categories)
	}
}
