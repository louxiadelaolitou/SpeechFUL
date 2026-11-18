package channels

import (
	"errors"
	"fmt"
	"testing"
)

func TestCampusCatalogNamesAndStableIDs(t *testing.T) {
	names := []string{"校园吐槽", "避雷指南", "吐血推荐", "搭子寻觅", "情感树洞", "互助问答", "人工智能", "社团活动", "求职经验", "校友评论"}
	items := make([]Channel, len(names))
	for i, entry := range expectedCatalog {
		items[i] = Channel{ID: entry.id, Code: entry.code, Name: entry.name, DisplayOrder: entry.displayOrder, InitiallyVisible: entry.initiallyVisible}
		if entry.name != names[i] || entry.displayOrder != 10*(i+1) ||
			entry.id.String() != fmt.Sprintf("5f6f4f88-8f64-4bb2-9d34-%012d", i+1) {
			t.Fatalf("category %d does not match the requested order", i)
		}
	}
	if err := ValidateCatalog(items); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]Channel{items[:7], items[:8], items[:9]} {
		if !errors.Is(ValidateCatalog(bad), ErrInvalidCatalog) {
			t.Fatal("partial catalog accepted")
		}
	}
	items[9].ID = items[0].ID
	if !errors.Is(ValidateCatalog(items), ErrInvalidCatalog) {
		t.Fatal("duplicate category identity accepted")
	}
}
