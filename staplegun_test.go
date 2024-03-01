package staplegun

import (
	"fmt"
	"testing"
)

func TestStaplegun(t *testing.T) {
	verbose := true
	err := MakeTemplates("./data/raw/set_1", "./data/processed/set_1", verbose)
	if err != nil {
		vPrintLn(0, fmt.Sprintf("ERROR: %s\n", err.Error()))
	}
}
