package openaibatch

import (
	"fmt"
	"strings"
)

// GatewayRoot normalizes a gateway base URL to the host root (no /v1 suffix).
func GatewayRoot(baseURL string) (string, error) {
	root := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for {
		switch {
		case strings.HasSuffix(root, "/v1/chat/completions"):
			root = strings.TrimSuffix(root, "/v1/chat/completions")
		case strings.HasSuffix(root, "/v1"):
			root = strings.TrimSuffix(root, "/v1")
		default:
			root = strings.TrimRight(root, "/")
			if root == "" {
				return "", fmt.Errorf("openaibatch: gateway root: empty base URL")
			}
			return root, nil
		}
		root = strings.TrimRight(root, "/")
	}
}
