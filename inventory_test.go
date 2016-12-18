package lutron

import (
	"fmt"
	"testing"
)

func TestReadJson(t *testing.T) {
	// test stuff here...
	i := NewCasetaInventory("report_test.json")
	fmt.Println(len(i.LIPIDList.Devices))
}

func TestNameFromId(t *testing.T) {
	i := NewCasetaInventory("report_test.json")
	s, err := i.NameFromId(4)
	if err != nil {
		t.Error(err.Error())
	}
	if s != "Kitchen Lights" {
		t.Error("expected name not matched " + s)
	}
}

func TestIdFromName(t *testing.T) {
	i := NewCasetaInventory("report_test.json")
	s, err := i.IdFromName("Kitchen Lights")
	if err != nil {
		t.Error(err.Error())
	}
	if s != 4 {
		t.Error("expected name not matched ", s)
	}
}

func TestInterfaceCompliance(t *testing.T) {
	// i := &CasetaInventory{}
	// x := *i.(Inventory)
	var _ Inventory = (*CasetaInventory)(nil)
}
