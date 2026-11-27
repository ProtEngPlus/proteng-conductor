package controllers

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/protengplus/proteng-conductor/models"
)

const standardAminoAcids = "ACDEFGHIKLMNPQRSTVWY"

// lowercase letters are accepted, the frontend allows a sequence in either case
func validateProteinSequence(name string, sequence string) error {
	if sequence == "" {
		return fmt.Errorf("error: %s is empty", name)
	}
	for i, r := range sequence {
		if !strings.ContainsRune(standardAminoAcids, unicode.ToUpper(r)) {
			return fmt.Errorf("error: %s has %q at position %d, only the 20 standard amino acids are allowed", name, string(r), i+1)
		}
	}
	return nil
}

func validateSequences(inputProtein string, labResult models.LabResult) error {
	if err := validateProteinSequence("input protein", inputProtein); err != nil {
		return err
	}
	for i, sequence := range labResult.Sequences {
		if err := validateProteinSequence(fmt.Sprintf("lab result sequence %d", i+1), sequence); err != nil {
			return err
		}
	}
	return nil
}
