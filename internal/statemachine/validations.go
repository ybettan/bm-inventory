package statemachine

import (
	"fmt"

	"github.com/filanov/stateswitch"
)

/*
 * Validations is a layer on top of stateswitch.  Its main purpose is to provide means to define and run validations in transition rules.
 * A validation is the pair [condition, category, printer].
 * condition is a statewitch.Condition.  If the output of the execution of the conditionis false, it means that the validation has failed.
 * printer is the way to format a string that can explain to the user what validation has failed and why.
 * failures are grouped by categories
 */

// A getter for an attribute.  Used to extract a value, in order to be used by the formatter.
type PrinterArg func(stateSwitch stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error)

// Used to format output strings.
type validationFailurePrinter struct {
	// As in fmt.Sprintf
	format string

	// Attribute getters
	args []PrinterArg
}

// Return formatted string by extracting all parameters with the PrinterArg getter functions, and then sending them to fmt.Sprintf
func (p *validationFailurePrinter) print(stateSwitch stateswitch.StateSwitch, args stateswitch.TransitionArgs) (string, error) {
	params := make([]interface{}, 0)
	for _, a := range p.args {
		i, err := a(stateSwitch, args)
		if err != nil {
			return "", err
		}
		params = append(params, i)
	}
	return fmt.Sprintf(p.format, params...), nil
}

// Create a validationFailurePrinter object that can be used for string formatting.
func Sprintf(format string, args ...PrinterArg) *validationFailurePrinter {
	return &validationFailurePrinter{
		format: format,
		args:   args,
	}
}

type validation struct {
	condition stateswitch.Condition
	category  string
	printer   *validationFailurePrinter
}

// Helper function for creating a validation
func Validation(condition stateswitch.Condition, categoey string, printer *validationFailurePrinter) *validation {
	return &validation{condition: condition, category: categoey, printer: printer}
}

type Validations []*validation

func (v Validations) Condition() stateswitch.Condition {
	conditions := make([]stateswitch.Condition, 0)
	for _, validation := range v {
		conditions = append(conditions, validation.condition)
	}
	return stateswitch.And(conditions...)
}

// Return formatted strings representing all failed validations in "Validations".
// This is done by iterating over the validations, and evaluating the condition in each validation.
// If the condition returns false (validation failed), then validation.printer is used to format the output string.
// This string is concatenated resulting slice, which is categorized by category
func (v Validations) printValidationFailures(stateSwitch stateswitch.StateSwitch, args stateswitch.TransitionArgs) (map[string][]string, error) {
	ret := make(map[string][]string)
	for _, validation := range v {
		b, err := validation.condition(stateSwitch, args)
		if err != nil {
			return nil, err
		}
		if !b {
			s, err := validation.printer.print(stateSwitch, args)
			if err != nil {
				return nil, err
			}
			ret[validation.category] = append(ret[validation.category], s)
		}
	}
	return ret, nil
}

// A callback to receive the validation failure strings by category and handle them.
type PostValidationFailure func(stateSwitch stateswitch.StateSwitch, args stateswitch.TransitionArgs, failures map[string][]string) error

type postValidationFailureData struct {
	validations           Validations
	postValidationFailure PostValidationFailure
}

// To be used as post transition function
func (p postValidationFailureData) postTransition(stateSwitch stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	printedValidationFailures, err := p.validations.printValidationFailures(stateSwitch, args)
	if err != nil {
		return err
	}
	return p.postValidationFailure(stateSwitch, args, printedValidationFailures)
}

// Helper function to create post transition callback function
func MakePostValidation(validations Validations, postValidationFailure PostValidationFailure) stateswitch.PostTransition {
	return postValidationFailureData{
		validations:           validations,
		postValidationFailure: postValidationFailure,
	}.postTransition
}
