package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import "fmt"

type ChoreSpec struct {
	Name             string      `json:"name" yaml:"name"`
	ConventionalType string      `json:"conventionalType" yaml:"conventionalType"`
	Description      string      `json:"description" yaml:"description"`
	Steps            []ChoreStep `json:"steps" yaml:"steps"`

	SkipCloneStep    bool `json:"skipCloneStep" yaml:"skipCloneStep"`
	SkipFinaliseStep bool `json:"skipFinaliseStep" yaml:"skipFinaliseStep"`

	// SourceConfig contains the original user-specified config that was resolved into this chore.
	SourceConfig *RepoChoreConfig `json:"internal_sourceConfig" yaml:"internal_sourceConfig"`
}

type ChoreStep struct {
	Image       string            `json:"image" yaml:"image"`
	Command     string            `json:"command" yaml:"command"`
	Environment map[string]string `json:"environment" yaml:"environment"`
	Internal    bool              `json:"-"`
}

func (choreSpec *ChoreSpec) CommitMessage() string {
	prefix := choreSpec.ConventionalType
	if prefix == "" {
		prefix = "chore"
	}
	return fmt.Sprintf("%s: %s", prefix, choreSpec.Name)
}

func (choreSpec *ChoreSpec) PrTitle() string {
	prefix := choreSpec.ConventionalType
	if prefix == "" {
		prefix = "chore"
	}
	return fmt.Sprintf("%s: %s", prefix, choreSpec.Name)
}

func (choreSpec *ChoreSpec) PrBody() string {
	if len(choreSpec.Description) > 0 {
		return choreSpec.Description
	} else {
		return "_No description provided by chore_"
	}
}
