package testing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestContainerNoAuthDefaultsEmitWarning(t *testing.T) {
	root := filepath.Clean("..")
	dockerfiles, err := filepath.Glob(filepath.Join(root, "docker", "Dockerfile*"))
	require.NoError(t, err)
	require.NotEmpty(t, dockerfiles)
	authDefaultImages := 0
	for _, path := range dockerfiles {
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		if !strings.Contains(string(content), "NORNICDB_NO_AUTH=") {
			continue
		}
		authDefaultImages++
		require.Contains(t, string(content), "NORNICDB_NO_AUTH=true", path)
	}
	require.Positive(t, authDefaultImages)
	entrypoint, err := os.ReadFile(filepath.Join(root, "docker", "entrypoint.sh"))
	require.NoError(t, err)
	require.Contains(t, string(entrypoint), `"event_id":"security.insecure_no_auth.enabled"`)

	composeFiles, err := filepath.Glob(filepath.Join(root, "docker-compose*.yml"))
	require.NoError(t, err)
	nested, err := filepath.Glob(filepath.Join(root, "docker", "docker-compose*.yml"))
	require.NoError(t, err)
	composeFiles = append(composeFiles, nested...)
	require.NotEmpty(t, composeFiles)

	for _, path := range composeFiles {
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var document yaml.Node
		require.NoError(t, yaml.Unmarshal(content, &document), path)
		require.True(t, hasNoAuthDefault(t, path, &document), "%s must declare a no-auth compatibility default", path)
	}
}

func TestComposeFilesForwardProxyEnvironment(t *testing.T) {
	required := []string{
		"NORNICDB_ADDRESS",
		"NORNICDB_BASE_PATH",
		"NORNICDB_AUTH",
		"NORNICDB_AUTH_JWT_SECRET",
		"NORNICDB_AUTH_PROVIDER",
		"NORNICDB_AUTH_TOKEN_EXPIRY",
		"NORNICDB_MIN_PASSWORD_LENGTH",
		"NORNICDB_NO_AUTH",
		"NORNICDB_OAUTH_ISSUER",
		"NORNICDB_OAUTH_CLIENT_ID",
		"NORNICDB_OAUTH_CLIENT_SECRET",
		"NORNICDB_OAUTH_CALLBACK_URL",
		"NORNICDB_CORS_ENABLED",
		"NORNICDB_CORS_ORIGINS",
		"NORNICDB_HTTP_ENABLED",
		"NORNICDB_HTTP_PORT",
		"NORNICDB_HTTP_ADDRESS",
		"NORNICDB_HTTP_TRUSTED_PROXIES",
		"NORNICDB_HTTPS_ENABLED",
		"NORNICDB_HTTPS_PORT",
		"NORNICDB_HTTP_TLS_CERT",
		"NORNICDB_HTTP_TLS_KEY",
		"NORNICDB_BOLT_ENABLED",
		"NORNICDB_BOLT_PORT",
		"NORNICDB_BOLT_ADDRESS",
		"NORNICDB_BOLT_SERVER_ANNOUNCEMENT",
		"NORNICDB_BOLT_TLS_ENABLED",
		"NORNICDB_BOLT_TLS_REQUIRE",
		"NORNICDB_BOLT_TLS_CERT",
		"NORNICDB_BOLT_TLS_KEY",
		"NORNICDB_BOLT_TLS_CLIENT_CA",
		"NORNICDB_BOLT_TLS_CLIENT_AUTH_MODE",
		"NORNICDB_BOLT_SNIFF_TIMEOUT",
		"NORNICDB_BOLT_AUTH_TIMEOUT",
		"NORNICDB_BOLT_STATEMENT_TIMEOUT",
		"NORNICDB_BOLT_WEBSOCKET_ENABLED",
		"NORNICDB_BOLT_WEBSOCKET_ALLOWED_ORIGINS",
		"NORNICDB_BOLT_WEBSOCKET_MAX_MESSAGE_SIZE",
		"NORNICDB_BOLT_WEBSOCKET_WRITE_BUFFER_SIZE",
		"NORNICDB_BOLT_WEBSOCKET_PING_INTERVAL",
		"NORNICDB_BOLT_WEBSOCKET_PONG_TIMEOUT",
		"NORNICDB_TLS_DIR",
	}

	for _, path := range composeFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			for _, name := range required {
				require.Contains(t, string(content), name+"=${"+name, "%s must forward %s", path, name)
			}
		})
	}
}

func composeFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Clean("..")
	composeFiles, err := filepath.Glob(filepath.Join(root, "docker-compose*.yml"))
	require.NoError(t, err)
	nested, err := filepath.Glob(filepath.Join(root, "docker", "docker-compose*.yml"))
	require.NoError(t, err)
	composeFiles = append(composeFiles, nested...)
	require.NotEmpty(t, composeFiles)
	return composeFiles
}

func hasNoAuthDefault(t *testing.T, path string, node *yaml.Node) bool {
	t.Helper()
	found := false
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "environment" && hasNoAuthEnvironmentDefault(t, path, value) {
				found = true
			}
		}
	}
	for _, child := range node.Content {
		if hasNoAuthDefault(t, path, child) {
			found = true
		}
	}
	return found
}

func hasNoAuthEnvironmentDefault(t *testing.T, path string, node *yaml.Node) bool {
	t.Helper()
	found := false
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if strings.TrimSpace(key.Value) == "NORNICDB_NO_AUTH" {
				require.Contains(t, strings.ToLower(strings.TrimSpace(value.Value)), "true", path)
				found = true
			}
		}
		return found
	}
	for _, child := range node.Content {
		value := strings.TrimSpace(child.Value)
		if strings.Contains(value, "NORNICDB_NO_AUTH") {
			require.Contains(t, strings.ToLower(value), "true", path)
			found = true
		}
	}
	return found
}
