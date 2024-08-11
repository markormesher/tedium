package utils

import "path/filepath"

func AddYamlJsonExtensions(root string) []string {
	return []string{
		root + ".yml",
		root + ".yaml",
		root + ".json",
	}
}

func IsYamlOrJsonFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yml" || ext == ".yaml" || ext == ".json"

}
