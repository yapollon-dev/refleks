//go:build windows

package settings

import (
	"encoding/base64"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protectSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	inputBytes := []byte(value)
	input := windows.DataBlob{Size: uint32(len(inputBytes))}
	if len(inputBytes) > 0 {
		input.Data = &inputBytes[0]
	}

	var output windows.DataBlob
	description, err := windows.UTF16PtrFromString(recordingPasswordDescriptor)
	if err != nil {
		return "", err
	}
	if err := windows.CryptProtectData(&input, description, nil, 0, nil, 0, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))

	protectedBytes := unsafe.Slice(output.Data, output.Size)
	return protectedSecretPrefix + base64.StdEncoding.EncodeToString(protectedBytes), nil
}

func unprotectSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, protectedSecretPrefix) {
		return "", errors.New("unsupported protected secret format")
	}

	encryptedBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, protectedSecretPrefix))
	if err != nil {
		return "", err
	}
	input := windows.DataBlob{Size: uint32(len(encryptedBytes))}
	if len(encryptedBytes) > 0 {
		input.Data = &encryptedBytes[0]
	}

	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, 0, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))

	plainBytes := unsafe.Slice(output.Data, output.Size)
	return string(plainBytes), nil
}
