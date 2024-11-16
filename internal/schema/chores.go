package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import "fmt"

type ChoreSpec struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	Steps       []ChoreStep `json:"steps" yaml:"steps"`

	SkipCloneStep    bool `json:"skipCloneStep" yaml:"skipCloneStep"`
	SkipFinaliseStep bool `json:"skipFinaliseStep" yaml:"skipFinaliseStep"`

	UserProvidedEnvironment map[string]string `json:"donotuse_userProvidedEnvironment"`
}

type ChoreStep struct {
	Image       string `json:"image" yaml:"image"`
	Command     string `json:"command" yaml:"command"`
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
