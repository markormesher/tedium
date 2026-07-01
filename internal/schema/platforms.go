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
	Type    string      `json:"type" yaml:"type"`
	BaseURL string      `json:"baseURL" yaml:"baseURL"`
	Auth    *AuthConfig `json:"auth" yaml:"auth"`

	// AlternateBaseURLs define other URLs that this platform should be used for (e.g. if you host a mirror of a public platform, or access a platform from multiple URLs).
	AlternateBaseURLs []string `json:"alternateBaseURLs" yaml:"alternateBaseURLs"`

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
	ClientID             string `json:"clientID" yaml:"clientID"`
	PrivateKeyString     string `json:"privateKeyString" yaml:"privateKeyString"`
	PrivateKeyFile       string `json:"privateKeyFile" yaml:"privateKeyFile"`
	InstallationID       string `json:"installationID" yaml:"installationID"`
	AppInstallationToken string `json:"doNotUse_appInstallationToken"`
}

func (ac *AuthConfig) GenerateJwt() (string, error) {
	if ac.ClientID == "" {
		return "", fmt.Errorf("error generating JWT: client ID is missing")
	}

	if ac.PrivateKeyFile == "" && ac.PrivateKeyString == "" {
		return "", fmt.Errorf("error generating JWT: private key is missing")
	}

	var privateKeyBytes []byte
	var err error
	if ac.PrivateKeyString != "" {
		privateKeyBytes = []byte(ac.PrivateKeyString)
	} else {
		privateKeyBytes, err = os.ReadFile(ac.PrivateKeyFile)
		if err != nil {
			return "", fmt.Errorf("error reading private key: %w", err)
		}

		// persiste as a string so it can be passed between containers
		ac.PrivateKeyString = string(privateKeyBytes)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("error parsing private key: %w", err)
	}

	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iat": now,
		"exp": now + 60*10,
		"iss": ac.ClientID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing JWT: %w", err)
	}

	return signedToken, nil
}

func (pc *PlatformConfig) AcceptsRepo(fullName string) bool {
	if pc.RepoFilters == nil {
		return true
	}

	for _, filter := range pc.RepoFilters {
		if filter.MatchString(fullName) {
			return true
		}
	}

	return false
}

type PlatformProfile struct {
	Email string
}
