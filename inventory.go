package lutron

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Raw parsed json from the caseta app
// via https://mholt.github.io/json-to-go/
type CasetaInventory struct {
	LIPIDList struct {
		Devices []struct {
			ID      int    `json:"ID"`
			Name    string `json:"Name"`
			Buttons []struct {
				Name   string `json:"Name"`
				Number int    `json:"Number"`
			} `json:"Buttons"`
		} `json:"Devices"`
		Zones []struct {
			ID   int    `json:"ID"`
			Name string `json:"Name"`
		} `json:"Zones"`
	} `json:"LIPIdList"`
}

func NewCasetaInventory(path string) *CasetaInventory {
	jsonfile, err := os.Open(path)
	if err != nil {
		// TODO return error - or fail more aggresively
		fmt.Println("opening file", err.Error())
	}
	inventory := &CasetaInventory{}
	jsonParser := json.NewDecoder(jsonfile)
	if err = jsonParser.Decode(inventory); err != nil {
		fmt.Println("parsing config file", err.Error())
	}
	return inventory
}

func (i *CasetaInventory) NameFromId(id int) (name string, err error) {
	for _, e := range i.LIPIDList.Devices {
		if e.ID == id {
			return e.Name, nil
		}
	}
	for _, e := range i.LIPIDList.Zones {
		if e.ID == id {
			return e.Name, nil
		}
	}
	return "", errors.New("no item with expected id")
}

func (i *CasetaInventory) IdFromName(n string) (id int, err error) {
	for _, e := range i.LIPIDList.Devices {
		if e.Name == n {
			return e.ID, nil
		}
	}
	for _, e := range i.LIPIDList.Zones {
		if e.Name == n {
			return e.ID, nil
		}
	}
	return 0, errors.New("no item with expected name")
}

type Inventory interface {
	NameFromId(int) (string, error)
	IdFromName(string) (int, error)
}
