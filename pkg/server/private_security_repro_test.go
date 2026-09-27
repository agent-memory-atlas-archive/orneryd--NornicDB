package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/orneryd/nornicdb/pkg/auth"
	"github.com/orneryd/nornicdb/pkg/multidb"
	"github.com/stretchr/testify/require"
)

func TestPrivateReproHTTPBareBeginRollbackOtherRequest(t *testing.T) {
	server, authenticator := setupTestServer(t)
	token := "Bearer " + getAuthToken(t, authenticator, "admin")
	request := func(statement string) TransactionResponse {
		recorder := makeRequest(t, server, http.MethodPost, "/db/nornic/tx/commit", map[string]any{
			"statements": []map[string]any{{"statement": statement}},
		}, token)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var response TransactionResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Empty(t, response.Errors, statement)
		return response
	}
	count := func() float64 {
		response := request("MATCH (n:PrivateTxProbe) RETURN count(n)")
		require.Len(t, response.Results, 1)
		return response.Results[0].Data[0].Row[0].(float64)
	}
	require.Equal(t, float64(0), count())
	begin := makeRequest(t, server, http.MethodPost, "/db/nornic/tx/commit", map[string]any{
		"statements": []map[string]any{{"statement": "BEGIN"}},
	}, token)
	var beginResponse TransactionResponse
	require.NoError(t, json.Unmarshal(begin.Body.Bytes(), &beginResponse))
	require.Len(t, beginResponse.Errors, 1)
	require.Equal(t, "Neo.ClientError.Statement.SyntaxError", beginResponse.Errors[0].Code)
	for _, statement := range []string{"begin transaction", "CoMmIt", "ROLLBACK TRANSACTION", "USE nornic BEGIN"} {
		recorder := makeRequest(t, server, http.MethodPost, "/db/nornic/tx/commit", map[string]any{
			"statements": []map[string]any{{"statement": statement}},
		}, token)
		var response TransactionResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Errors, 1, statement)
		require.Equal(t, "Neo.ClientError.Statement.SyntaxError", response.Errors[0].Code, statement)
	}
	script := request("BEGIN CREATE (:PrivateScriptProbe) COMMIT")
	require.Empty(t, script.Errors)
	t.Cleanup(func() {
		executor, err := server.getExecutorForDatabase("nornic")
		if err == nil && executor.HasActiveTransaction() {
			request("ROLLBACK")
		}
	})
	created := request("CREATE (:PrivateTxProbe) RETURN 3")
	require.Equal(t, float64(3), created.Results[0].Data[0].Row[0])
	require.Equal(t, float64(1), count(), "acknowledged write must survive unrelated requests")
}

func TestPrivateReproCompositeConstituentAccess(t *testing.T) {
	server, authenticator := setupTestServer(t)
	require.NoError(t, server.dbManager.CreateDatabase("private_allowed"))
	require.NoError(t, server.dbManager.CreateDatabase("private_denied"))
	require.NoError(t, server.dbManager.CreateCompositeDatabase("private_cmp", []multidb.ConstituentRef{
		{Alias: "a", DatabaseName: "private_allowed", Type: "local", AccessMode: "read_write"},
		{Alias: "b", DatabaseName: "private_denied", Type: "local", AccessMode: "read_write"},
	}))
	_, err := authenticator.CreateUser("private_editor", "password123", []auth.Role{auth.RoleEditor})
	require.NoError(t, err)
	require.NoError(t, server.allowlistStore.SaveRoleDatabases(context.Background(), "editor", []string{"private_allowed", "private_cmp"}))
	for _, database := range []string{"private_allowed", "private_cmp"} {
		require.NoError(t, server.privilegesStore.SavePrivilege(context.Background(), "editor", database, true, true))
	}
	request := func(database, token, statement string) (int, TransactionResponse) {
		recorder := makeRequest(t, server, http.MethodPost, "/db/"+database+"/tx/commit", map[string]any{
			"statements": []map[string]any{{"statement": statement}},
		}, "Bearer "+token)
		var response TransactionResponse
		_ = json.Unmarshal(recorder.Body.Bytes(), &response)
		return recorder.Code, response
	}
	_, seeded := request("private_denied", getAuthToken(t, authenticator, "admin"), "CREATE (:PrivateSecret {v: 'hidden'})")
	require.Empty(t, seeded.Errors)
	_, allowedSeed := request("private_allowed", getAuthToken(t, authenticator, "admin"), "CREATE (:PrivateSecret {v: 'allowed'})")
	require.Empty(t, allowedSeed.Errors)
	userToken := getAuthToken(t, authenticator, "private_editor")
	_, allowed := request("private_cmp", userToken, "CALL { USE private_cmp.a MATCH (n:PrivateSecret) RETURN n.v AS v } RETURN v")
	require.Empty(t, allowed.Errors)
	require.Equal(t, "allowed", allowed.Results[0].Data[0].Row[0])
	code, direct := request("private_denied", userToken, "MATCH (n:PrivateSecret) RETURN n.v")
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, direct.Errors)
	require.Equal(t, "Neo.ClientError.Security.Forbidden", direct.Errors[0].Code)
	require.Empty(t, direct.Results)
	code, exposed := request("private_cmp", userToken, "CALL { USE private_cmp.b MATCH (n:PrivateSecret) RETURN n.v AS v } RETURN v")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, exposed.Errors, 1)
	require.Equal(t, "Neo.ClientError.Security.Forbidden", exposed.Errors[0].Code)
	_, forbiddenWrite := request("private_cmp", userToken, "USE private_cmp.b CREATE (:PrivateSecret {v: 'written'})")
	require.Len(t, forbiddenWrite.Errors, 1)
	require.Equal(t, "Neo.ClientError.Security.Forbidden", forbiddenWrite.Errors[0].Code)
}
