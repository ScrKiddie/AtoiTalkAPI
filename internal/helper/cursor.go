package helper

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func DecodeCursor(cursor string, delimiter string) (string, int, error) {
	decodedBytes, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, fmt.Errorf("invalid cursor encoding")
	}

	decodedString := string(decodedBytes)
	parts := strings.Split(decodedString, delimiter)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid cursor format")
	}

	intPart, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid cursor id")
	}

	return parts[0], intPart, nil
}

func EncodeCursor(strPart string, intPart int, delimiter string) string {
	cursorString := fmt.Sprintf("%s%s%d", strPart, delimiter, intPart)
	return base64.URLEncoding.EncodeToString([]byte(cursorString))
}
