package jev

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// JoinKey builds a reversible SystemOne question key for rowID and optionID.
func JoinKey(rowID, optionID string) string {
	rowID = strings.TrimSpace(rowID)
	optionID = strings.TrimSpace(optionID)
	var b bytes.Buffer
	writeLenPrefixed(&b, rowID)
	writeLenPrefixed(&b, optionID)
	return b.String()
}

// SplitKey decodes a key produced by JoinKey.
func SplitKey(key string) (rowID, optionID string, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("jev: split key: key is empty")
	}
	r := bytes.NewReader([]byte(key))
	rowID, err = readLenPrefixed(r)
	if err != nil {
		return "", "", err
	}
	optionID, err = readLenPrefixed(r)
	if err != nil {
		return "", "", err
	}
	if rowID == "" || optionID == "" {
		return "", "", fmt.Errorf("jev: split key: empty row or option id")
	}
	if r.Len() != 0 {
		return "", "", fmt.Errorf("jev: split key: trailing data")
	}
	return rowID, optionID, nil
}

func writeLenPrefixed(b *bytes.Buffer, s string) {
	_ = binary.Write(b, binary.BigEndian, uint32(len(s)))
	b.WriteString(s)
}

func readLenPrefixed(r *bytes.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return "", fmt.Errorf("jev: split key: invalid key")
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("jev: split key: invalid key")
	}
	return string(buf), nil
}

// optionSetKey returns a stable group key for a sorted option-id set.
func optionSetKey(optionIDs []string) string {
	var b strings.Builder
	for i, id := range optionIDs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(len(id)))
		b.WriteByte(':')
		b.WriteString(id)
	}
	return b.String()
}
