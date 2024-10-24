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
	Id       string      `json:"id" yaml:"id"`
	Type     string      `json:"type" yaml:"type"`
	Endpoint string      `json:"endpoint" yaml:"endpoint"`
	Auth     *AuthConfig `json:"auth" yaml:"auth"`

	// RepoFiltersRaw specifies a list of Go regexes; if specified, only repos that match at least one filter will be processed.
	RepoFiltersRaw []string `json:"repoFilters" yaml:"repoFilters"`
	RepoFilters    []*regexp.Regexp

	// populated during execution, not via config
	BotProfile struct {
		Username string
		Email    string
	}
}

// AuthConfig defines how to authenticate with a platform.
type AuthConfig struct {
	// domain pattern to match when searching for auth to clone a repo
	DomainPatternRaw string `json:"domainPattern" yaml:"domainPattern"`
	DomainPattern    *regexp.Regexp

	// tokens for gitea
	Token string `json:"token" yaml:"token"`

	// JWT for github apps (and maybe gitea in the future?)
	ClientId         string `json:"clientId" yaml:"clientId"`
	PrivateKeyString string `json:"privateKeyString" yaml:"privateKeyString"`
	PrivateKeyFile   string `json:"privateKeyFile" yaml:"privateKeyFile"`
	InstallationId   string `json:"installationId" yaml:"installationId"`

	// used when any of the credentials above are "exchanged" for a different token that is used for future requests
	InternalToken string `json:"doNotUse_internalToken"`
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

type PlatformBotProfile struct {
	Username string
	Email    string
}
