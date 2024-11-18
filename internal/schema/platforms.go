package schema

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PlatformConfig defines a Git platform from which repos can be discovered, such as Gitea or GitHub.
type PlatformConfig struct {
	Type   string      `json:"type" yaml:"type"`
	Domain string      `json:"domain" yaml:"domain"`
	Auth   *AuthConfig `json:"auth" yaml:"auth"`

	// SkipDiscovery specifies that this platform should not be used to discover target repos (i.e. it is only used for reading config).
	SkipDiscovery bool `json:"skipDiscovery" yaml:"skipDiscovery"`

	// RepoFiltersRaw specifies a list of Go regexes; if specified, only repos that match at least one filter will be processed.
	RepoFiltersRaw []string `json:"repoFilters" yaml:"repoFilters"`
	RepoFilters    []*regexp.Regexp
}

var (
	AuthConfigTypeUserToken = "user_token"
	AuthConfigTypeApp       = "app"
)

// AuthConfig defines how to authenticate with a platform.
type AuthConfig struct {
	Type string `json:"type" yaml:"type"`

	// type: user_token
	Token string `json:"token" yaml:"token"`

	// type: app
	ClientId             string `json:"clientId" yaml:"clientId"`
	PrivateKeyString     string `json:"privateKeyString" yaml:"privateKeyString"`
	PrivateKeyFile       string `json:"privateKeyFile" yaml:"privateKeyFile"`
	InstallationId       string `json:"installationId" yaml:"installationId"`
	AppInstallationToken string `json:"doNotUse_appInstallationToken"`
}

func (ac *AuthConfig) GenerateJwt() (string, error) {
	if ac.ClientId == "" {
		return "", fmt.Errorf("Error generating JWT: client ID is missing")
	}

	if ac.PrivateKeyFile == "" && ac.PrivateKeyString == "" {
		return "", fmt.Errorf("Error generating JWT: private key is missing")
	}

	var privateKeyBytes []byte
	var err error
	if ac.PrivateKeyString != "" {
		privateKeyBytes = []byte(ac.PrivateKeyString)
	} else {
		privateKeyBytes, err = os.ReadFile(ac.PrivateKeyFile)
		if err != nil {
			return "", fmt.Errorf("Error reading private key: %w", err)
		}

		// persiste as a string so it can be passed between containers
		ac.PrivateKeyString = string(privateKeyBytes)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("Error parsing private key: %w", err)
	}

	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iat": now,
		"exp": now + 60*10,
		"iss": ac.ClientId,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("Error signing JWT: %w", err)
	}

	return signedToken, nil
}

func (pc PlatformConfig) AcceptsRepo(fullName string) bool {
	if pc.RepoFilters == nil {
		return true
	}

	for i := range pc.RepoFilters {
		// loop by index to avoid copying the regexp object
		if pc.RepoFilters[i].MatchString(fullName) {
			return true
		}
	}

	return false
}

type PlatformProfile struct {
	Email string
}
