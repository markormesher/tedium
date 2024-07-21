package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import "fmt"

type ChoreSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Steps       []ChoreStep `json:"steps"`

	SkipCloneStep    bool `json:"skipCloneStep"`
	SkipFinaliseStep bool `json:"skipFinaliseStep"`
}

type ChoreStep struct {
	Image       string `json:"image"`
	Command     string `json:"command"`
	Environment map[string]string
}

func (choreSpec *ChoreSpec) PrTitle() string {
	return fmt.Sprintf("[Tedium] %s", choreSpec.Name)
}

func (choreSpec *ChoreSpec) PrBody() string {
	if len(choreSpec.Description) > 0 {
		return choreSpec.Description
	} else {
		return "_No description provided by chore_"
	}
}
