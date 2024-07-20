package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"regexp"
	"strings"
)

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	var result strings.Builder
	for i := 0; i < length; i++ {
		randomIndex := rand.Intn(len(charset))
		result.WriteByte(charset[randomIndex])
	}
	return result.String()
}

func UniqueName(prefix string) string {
	return "tedium-" + prefix + "-" + randomString(8)
}

func Sha256String(input string) string {
	hash := sha256.New()
	hash.Write([]byte(input))
	hashSum := hash.Sum(nil)
	return hex.EncodeToString(hashSum)
}

var illegalBranchCharRegex = regexp.MustCompile("[^a-zA-Z0-9\\-]")

func ConvertToBranchName(value string) string {
	value = strings.ReplaceAll(value, " ", "-")
	value = illegalBranchCharRegex.ReplaceAllString(value, "")
	value = strings.ToLower(value)
	return "tedium/" + value
}
