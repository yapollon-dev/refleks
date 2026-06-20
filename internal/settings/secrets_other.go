//go:build !windows

package settings

import (
	"encoding/base64"
	"errors"
	"strings"
)

func protectSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return unsupportedSecretPrefix + base64.StdEncoding.EncodeToString([]byte(value)), nil
}

func unprotectSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, unsupportedSecretPrefix) {
		return "", errors.New("unsupported protected secret format")
	}
	plainBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, unsupportedSecretPrefix))
	if err != nil {
		return "", err
	}
	return string(plainBytes), nil
}
