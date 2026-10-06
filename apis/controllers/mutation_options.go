package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/xeipuuv/gojsonschema"

	"github.com/protengplus/proteng-conductor/config"
)

func validateServiceOptions(service string, option interface{}, inputProtein string) error {
	schemaLoader := gojsonschema.NewStringLoader(config.GetSchema(service))
	optionLoader := gojsonschema.NewGoLoader(option)
	result, err := gojsonschema.Validate(schemaLoader, optionLoader)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return errors.New(result.Errors()[0].String())
	}
	if service == "mutation" {
		return validateMutationOptions(option, inputProtein)
	}
	return nil
}

type mutationOptions struct {
	NumTrajectories  int      `json:"num_trajectories"`
	MutateRegions    [][2]int `json:"mutate_regions"`
	NumMutationsLow  int      `json:"num_mutations_low"`
	NumMutationsHigh int      `json:"num_mutations_high"`
}

func validateMutationOptions(option interface{}, inputProtein string) error {
	raw, err := json.Marshal(option)
	if err != nil {
		return err
	}
	var options mutationOptions
	if err := json.Unmarshal(raw, &options); err != nil {
		return err
	}

	if options.NumMutationsHigh < options.NumMutationsLow {
		return fmt.Errorf("error: num_mutations_high (%d) must be at least num_mutations_low (%d)", options.NumMutationsHigh, options.NumMutationsLow)
	}

	proteinLength := utf8.RuneCountInString(inputProtein)
	// regions are 1-based and inclusive, none means the whole protein
	regions := append([][2]int(nil), options.MutateRegions...)
	sort.Slice(regions, func(i, j int) bool { return regions[i][0] < regions[j][0] })

	mutablePositions := 0
	for i, region := range regions {
		start, end := region[0], region[1]
		if start < 1 || start > end || end > proteinLength {
			return fmt.Errorf("error: mutate_regions [%d, %d] does not fit a protein of length %d", start, end, proteinLength)
		}
		if i > 0 && start <= regions[i-1][1] {
			return fmt.Errorf("error: mutate_regions [%d, %d] and [%d, %d] overlap", regions[i-1][0], regions[i-1][1], start, end)
		}
		mutablePositions += end - start + 1
	}
	if len(regions) == 0 {
		mutablePositions = proteinLength
	}

	if options.NumMutationsHigh > mutablePositions {
		return fmt.Errorf("error: num_mutations_high (%d) is more than the %d positions that can mutate", options.NumMutationsHigh, mutablePositions)
	}

	if len(regions) >= 2 {
		for _, region := range regions {
			if size := region[1] - region[0] + 1; size < options.NumMutationsLow {
				return fmt.Errorf("error: mutate_regions [%d, %d] has %d positions, fewer than num_mutations_low (%d)", region[0], region[1], size, options.NumMutationsLow)
			}
		}
		// one trajectory per region and one more that combines them
		if options.NumTrajectories < len(regions)+1 {
			return fmt.Errorf("error: num_trajectories (%d) must be at least %d for %d mutate_regions", options.NumTrajectories, len(regions)+1, len(regions))
		}
	}
	return nil
}
