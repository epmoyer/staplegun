package staplegun

import (
	"fmt"
	"testing"
)

func TestStaplegun(t *testing.T) {
	verbose := true

	varMap := VarMap{
		"testvar1": "value1",
		"testvar2": "value2",
		"testvar3": "value3",
		"testvar4": "value4",
	}

	err := MakeTemplates("./data/raw/set_1", "./data/processed/set_1", verbose, varMap)
	if err != nil {
		vPrintLn(0, fmt.Sprintf("ERROR: %s\n", err.Error()))
	}
	// TODO: Validate var substitutions
}
