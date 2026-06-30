package utils

import "path/filepath"

func AddConfigFileExtensions(root string) []string {
	return []string{
		root + ".yml",
		root + ".yaml",
		root + ".json",
	}
}

func HasConfigFileExtension(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yml" || ext == ".yaml" || ext == ".json"

}
